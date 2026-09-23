package mcp

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"github.com/blackwell-systems/knowing/internal/store"
	"github.com/blackwell-systems/knowing/internal/types"
	"github.com/mark3labs/mcp-go/mcp"
)

// fakeSession implements mcp-go's SessionWithClientInfo so tests can drive
// handlers as if they arrived from a specific client/connection.
type fakeSession struct {
	id         string
	clientInfo mcp.Implementation
}

func (f *fakeSession) Initialize()      {}
func (f *fakeSession) Initialized() bool { return true }
func (f *fakeSession) NotificationChannel() chan<- mcp.JSONRPCNotification {
	return make(chan mcp.JSONRPCNotification, 1)
}
func (f *fakeSession) SessionID() string                    { return f.id }
func (f *fakeSession) GetClientInfo() mcp.Implementation    { return f.clientInfo }
func (f *fakeSession) SetClientInfo(ci mcp.Implementation)  { f.clientInfo = ci }
func (f *fakeSession) GetClientCapabilities() mcp.ClientCapabilities {
	return mcp.ClientCapabilities{}
}
func (f *fakeSession) SetClientCapabilities(mcp.ClientCapabilities) {}

// sessionCtx builds a context that carries the given client session, as mcp-go
// does for real tool calls.
func (s *Server) sessionCtx(id, clientName string) context.Context {
	sess := &fakeSession{id: id, clientInfo: mcp.Implementation{Name: clientName}}
	return s.mcpServer.WithContext(context.Background(), sess)
}

func TestSessionFor_Isolation(t *testing.T) {
	srv := NewServer(newMockGraphStore())

	a := srv.sessionFor(srv.sessionCtx("conn-a", "claude-code"))
	b := srv.sessionFor(srv.sessionCtx("conn-b", "hermes"))
	if a == b {
		t.Fatal("distinct session ids must yield distinct sessionState")
	}
	if a.clientName != "claude-code" || b.clientName != "hermes" {
		t.Errorf("client name not captured: a=%q b=%q", a.clientName, b.clientName)
	}
	// Same id returns the same state (stable per connection).
	if a2 := srv.sessionFor(srv.sessionCtx("conn-a", "claude-code")); a2 != a {
		t.Error("same session id must return the same sessionState")
	}
	// No session in context -> shared default state (single-client / stdio).
	d1 := srv.sessionFor(context.Background())
	d2 := srv.sessionFor(context.Background())
	if d1 != d2 {
		t.Error("no-session contexts should share one default state")
	}
}

// TestKeywordIsolation is the core regression test for the cross-agent
// contamination bug: before per-connection state, one client's task keywords
// were global and would overwrite another's, so implicit feedback recorded the
// wrong (keyword -> symbol) associations.
func TestKeywordIsolation(t *testing.T) {
	ss := newFileStore(t)
	srv := NewServer(ss)

	ctxA := srv.sessionCtx("conn-a", "agent-a")
	ctxB := srv.sessionCtx("conn-b", "agent-b")

	if _, err := srv.handleContextForTask(ctxA, makeCallToolRequest("context_for_task", map[string]any{
		"task_description": "alphaterm zeta refactor",
	})); err != nil {
		t.Fatalf("A: %v", err)
	}
	if _, err := srv.handleContextForTask(ctxB, makeCallToolRequest("context_for_task", map[string]any{
		"task_description": "betaword omega migrate",
	})); err != nil {
		t.Fatalf("B: %v", err)
	}

	sa := srv.sessionFor(ctxA)
	sb := srv.sessionFor(ctxB)
	if !sliceHas(sa.lastTaskKeywords, "alphaterm") {
		t.Errorf("session A lost its own keywords: %v", sa.lastTaskKeywords)
	}
	if sliceHas(sa.lastTaskKeywords, "betaword") {
		t.Errorf("session A contaminated with B's keywords: %v", sa.lastTaskKeywords)
	}
	if !sliceHas(sb.lastTaskKeywords, "betaword") {
		t.Errorf("session B lost its own keywords: %v", sb.lastTaskKeywords)
	}
}

// TestConcurrentSessionsNoRace drives many distinct clients through the context
// handler concurrently. Run with -race, it proves the per-session registry,
// lastPacks maps, and GCF dedup session are free of data races and that the
// previously-global unsynchronized maps no longer panic under concurrency.
func TestConcurrentSessionsNoRace(t *testing.T) {
	ss := newFileStore(t)
	srv := NewServer(ss)

	const n = 24
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ctx := srv.sessionCtx(fmt.Sprintf("conn-%d", i), fmt.Sprintf("agent-%d", i))
			req := makeCallToolRequest("context_for_task", map[string]any{
				"task_description": fmt.Sprintf("task number %d refactor", i),
			})
			for j := 0; j < 3; j++ {
				if _, err := srv.handleContextForTask(ctx, req); err != nil {
					t.Errorf("goroutine %d: %v", i, err)
					return
				}
				srv.ObserveToolUse(ctx, "SomeSymbol")
			}
		}(i)
	}
	wg.Wait()

	srv.sessionsMu.Lock()
	got := len(srv.sessions)
	srv.sessionsMu.Unlock()
	if got != n {
		t.Errorf("expected %d isolated sessions, got %d", n, got)
	}
}

func TestDisableImplicitFeedback_PerSession(t *testing.T) {
	srv := NewServer(newMockGraphStore())
	// Existing session before disable.
	before := srv.sessionFor(srv.sessionCtx("early", "a"))
	if before.implicit == nil {
		t.Fatal("feedback should be enabled by default")
	}
	srv.DisableImplicitFeedback()
	if before.implicit != nil {
		t.Error("disable must clear implicit on existing sessions")
	}
	// New session created after disable must also have feedback off.
	after := srv.sessionFor(srv.sessionCtx("late", "b"))
	if after.implicit != nil {
		t.Error("disable must persist to newly-created sessions")
	}
}

func TestEvictSession(t *testing.T) {
	srv := NewServer(newMockGraphStore())
	srv.sessionFor(srv.sessionCtx("gone", "a"))
	srv.evictSession("gone")
	srv.sessionsMu.Lock()
	_, present := srv.sessions["gone"]
	srv.sessionsMu.Unlock()
	if present {
		t.Error("evictSession should drop the state")
	}
}

func newFileStore(t *testing.T) *store.SQLiteStore {
	t.Helper()
	ss, err := store.NewSQLiteStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	t.Cleanup(func() { ss.Close() })
	// Seed one node so the handler has something to rank/return.
	if err := ss.PutNode(context.Background(), types.Node{
		NodeHash:      testHash("seed-node"),
		QualifiedName: "seed.SomeSymbol",
		Kind:          "function",
		Signature:     "func SomeSymbol()",
	}); err != nil {
		t.Fatalf("seed node: %v", err)
	}
	return ss
}

func sliceHas(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

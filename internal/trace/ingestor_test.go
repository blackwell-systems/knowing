package trace

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/blackwell-systems/knowing/internal/types"

	_ "modernc.org/sqlite"
)

// setupIngestDB creates an in-memory SQLite database with the full schema
// needed for ingestor tests (edges, edge_events, route_symbols tables).
func setupIngestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	schema := `
		CREATE TABLE repos (
			repo_hash   BLOB PRIMARY KEY,
			repo_url    TEXT NOT NULL,
			last_commit TEXT,
			last_indexed INTEGER
		);
		CREATE TABLE files (
			file_hash    BLOB PRIMARY KEY,
			repo_hash    BLOB NOT NULL,
			path         TEXT NOT NULL,
			content_hash BLOB NOT NULL
		);
		CREATE TABLE nodes (
			node_hash      BLOB PRIMARY KEY,
			file_hash      BLOB NOT NULL,
			qualified_name TEXT NOT NULL,
			kind           TEXT NOT NULL,
			line           INTEGER,
			signature      TEXT
		);
		CREATE TABLE edges (
			edge_hash    BLOB PRIMARY KEY,
			source_hash  BLOB NOT NULL,
			target_hash  BLOB NOT NULL,
			edge_type    TEXT NOT NULL,
			confidence   REAL NOT NULL DEFAULT 1.0,
			provenance   TEXT NOT NULL DEFAULT 'ast_resolved',
			callsite_line INTEGER NOT NULL DEFAULT 0,
			callsite_col  INTEGER NOT NULL DEFAULT 0,
			callsite_file TEXT NOT NULL DEFAULT '',
			observation_count INTEGER NOT NULL DEFAULT 0,
			last_observed INTEGER NOT NULL DEFAULT 0
		);
		CREATE TABLE edge_events (
			event_id      INTEGER PRIMARY KEY AUTOINCREMENT,
			edge_hash     BLOB NOT NULL,
			event_type    TEXT NOT NULL,
			snapshot_hash BLOB NOT NULL,
			source_commit TEXT NOT NULL,
			indexer_ver   TEXT NOT NULL,
			timestamp     INTEGER NOT NULL
		);
		CREATE TABLE snapshots (
			snapshot_hash BLOB PRIMARY KEY,
			parent_hash   BLOB,
			repo_hash     BLOB NOT NULL,
			commit_hash   TEXT NOT NULL,
			timestamp     INTEGER NOT NULL,
			node_count    INTEGER NOT NULL,
			edge_count    INTEGER NOT NULL
		);
		CREATE TABLE route_symbols (
			service_name  TEXT NOT NULL,
			route_pattern TEXT NOT NULL,
			node_hash     BLOB NOT NULL,
			mapping_type  TEXT NOT NULL,
			created_at    INTEGER NOT NULL,
			PRIMARY KEY (service_name, route_pattern, mapping_type)
		);
		CREATE INDEX idx_edges_provenance ON edges(provenance);
		CREATE INDEX idx_edges_last_observed ON edges(last_observed);
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	return db
}

func TestIngestSpans_NewEdge(t *testing.T) {
	db := setupIngestDB(t)
	ctx := context.Background()
	resolver := NewSymbolResolver(db)
	ing := NewIngestor(db, resolver, TraceIngestConfig{})

	span := TraceSpan{
		TraceID:       "trace-001",
		SpanID:        "span-001",
		ServiceName:   "api-gateway",
		OperationName: "HTTP GET /users",
		PeerService:   "user-service",
		Attributes: map[string]string{
			"http.method": "GET",
			"http.route":  "/users",
		},
		StartTime: time.Now(),
		Duration:  100 * time.Millisecond,
	}

	result, err := ing.IngestSpans(ctx, []TraceSpan{span})
	if err != nil {
		t.Fatalf("IngestSpans: %v", err)
	}
	if result.Created != 1 {
		t.Errorf("Created: got %d, want 1", result.Created)
	}
	if result.Updated != 0 {
		t.Errorf("Updated: got %d, want 0", result.Updated)
	}

	// Verify edge was created with correct fields.
	var obsCount int
	var lastObs int64
	var confidence float64
	var provenance string
	err = db.QueryRowContext(ctx, `SELECT observation_count, last_observed, confidence, provenance FROM edges LIMIT 1`).
		Scan(&obsCount, &lastObs, &confidence, &provenance)
	if err != nil {
		t.Fatalf("query edge: %v", err)
	}
	if obsCount != 1 {
		t.Errorf("observation_count: got %d, want 1", obsCount)
	}
	if lastObs == 0 {
		t.Error("last_observed should be non-zero")
	}
	// Confidence should be 0.3 (unresolved target confidence caps it)
	// or 0.5 (from ConfidenceFromCount(1)). The lower of resolver confidence
	// (0.3 for unresolved) and count confidence (0.5) is used.
	if confidence > 0.5 {
		t.Errorf("confidence: got %f, want <= 0.5", confidence)
	}

	// Verify an edge event was recorded.
	var eventCount int
	err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM edge_events WHERE event_type = 'added'`).Scan(&eventCount)
	if err != nil {
		t.Fatalf("query events: %v", err)
	}
	if eventCount != 1 {
		t.Errorf("edge events: got %d, want 1", eventCount)
	}
}

func TestIngestSpans_ExistingEdge(t *testing.T) {
	db := setupIngestDB(t)
	ctx := context.Background()
	resolver := NewSymbolResolver(db)
	ing := NewIngestor(db, resolver, TraceIngestConfig{})

	span := TraceSpan{
		TraceID:       "trace-001",
		SpanID:        "span-001",
		ServiceName:   "api-gateway",
		OperationName: "HTTP GET /users",
		PeerService:   "user-service",
		Attributes: map[string]string{
			"http.method": "GET",
			"http.route":  "/users",
		},
	}

	// Ingest the same span twice.
	_, err := ing.IngestSpans(ctx, []TraceSpan{span})
	if err != nil {
		t.Fatalf("first IngestSpans: %v", err)
	}

	result, err := ing.IngestSpans(ctx, []TraceSpan{span})
	if err != nil {
		t.Fatalf("second IngestSpans: %v", err)
	}
	if result.Updated != 1 {
		t.Errorf("Updated: got %d, want 1", result.Updated)
	}
	if result.Created != 0 {
		t.Errorf("Created: got %d, want 0", result.Created)
	}

	// Verify observation_count was incremented.
	var obsCount int
	err = db.QueryRowContext(ctx, `SELECT observation_count FROM edges LIMIT 1`).Scan(&obsCount)
	if err != nil {
		t.Fatalf("query edge: %v", err)
	}
	if obsCount != 2 {
		t.Errorf("observation_count: got %d, want 2", obsCount)
	}

	// Verify only one edge exists (same hash).
	var edgeCount int
	err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM edges`).Scan(&edgeCount)
	if err != nil {
		t.Fatalf("count edges: %v", err)
	}
	if edgeCount != 1 {
		t.Errorf("edge count: got %d, want 1", edgeCount)
	}
}

func TestIngestSpans_HTTPRoute(t *testing.T) {
	db := setupIngestDB(t)
	ctx := context.Background()
	resolver := NewSymbolResolver(db)
	ing := NewIngestor(db, resolver, TraceIngestConfig{})

	span := TraceSpan{
		TraceID:     "trace-http",
		ServiceName: "frontend",
		PeerService: "backend",
		Attributes: map[string]string{
			"http.method": "POST",
			"http.route":  "/api/orders",
		},
	}

	result, err := ing.IngestSpans(ctx, []TraceSpan{span})
	if err != nil {
		t.Fatalf("IngestSpans: %v", err)
	}
	if result.Created != 1 {
		t.Errorf("Created: got %d, want 1", result.Created)
	}

	// Verify edge type is runtime_calls for HTTP.
	var edgeType string
	err = db.QueryRowContext(ctx, `SELECT edge_type FROM edges LIMIT 1`).Scan(&edgeType)
	if err != nil {
		t.Fatalf("query edge type: %v", err)
	}
	if edgeType != "runtime_calls" {
		t.Errorf("edge_type: got %q, want %q", edgeType, "runtime_calls")
	}
}

func TestIngestSpans_GRPCMethod(t *testing.T) {
	db := setupIngestDB(t)
	ctx := context.Background()
	resolver := NewSymbolResolver(db)
	ing := NewIngestor(db, resolver, TraceIngestConfig{})

	span := TraceSpan{
		TraceID:     "trace-grpc",
		ServiceName: "api-gateway",
		PeerService: "user-service",
		Attributes: map[string]string{
			"rpc.service": "UserService",
			"rpc.method":  "GetUser",
		},
	}

	result, err := ing.IngestSpans(ctx, []TraceSpan{span})
	if err != nil {
		t.Fatalf("IngestSpans: %v", err)
	}
	if result.Created != 1 {
		t.Errorf("Created: got %d, want 1", result.Created)
	}

	// Verify edge type is runtime_rpc for gRPC.
	var edgeType string
	err = db.QueryRowContext(ctx, `SELECT edge_type FROM edges LIMIT 1`).Scan(&edgeType)
	if err != nil {
		t.Fatalf("query edge type: %v", err)
	}
	if edgeType != "runtime_rpc" {
		t.Errorf("edge_type: got %q, want %q", edgeType, "runtime_rpc")
	}
}

func TestIngestHTTPLogs(t *testing.T) {
	db := setupIngestDB(t)
	ctx := context.Background()
	resolver := NewSymbolResolver(db)
	ing := NewIngestor(db, resolver, TraceIngestConfig{})

	entries := []HTTPLogEntry{
		{
			Timestamp:   time.Now(),
			Method:      "GET",
			Path:        "/api/health",
			StatusCode:  200,
			ServiceName: "api-service",
			Duration:    50 * time.Millisecond,
		},
	}

	result, err := ing.IngestHTTPLogs(ctx, entries)
	if err != nil {
		t.Fatalf("IngestHTTPLogs: %v", err)
	}
	if result.Created != 1 {
		t.Errorf("Created: got %d, want 1", result.Created)
	}

	// Verify the edge was created.
	var edgeCount int
	err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM edges`).Scan(&edgeCount)
	if err != nil {
		t.Fatalf("count edges: %v", err)
	}
	if edgeCount != 1 {
		t.Errorf("edge count: got %d, want 1", edgeCount)
	}
}

func TestRuntimeEdgeStats(t *testing.T) {
	db := setupIngestDB(t)
	ctx := context.Background()

	now := time.Now().Unix()

	// Insert a static edge (non-otel provenance).
	staticSource := types.ComputeNodeHash("repo", "pkg", types.EmptyHash, "Func", "function")
	staticTarget := types.ComputeNodeHash("repo", "pkg", types.EmptyHash, "Other", "function")
	staticHash := types.ComputeEdgeHash(staticSource, staticTarget, "calls", "ast_resolved")
	_, err := db.ExecContext(ctx,
		`INSERT INTO edges (edge_hash, source_hash, target_hash, edge_type, confidence, provenance, callsite_line, callsite_col, callsite_file, observation_count, last_observed)
		 VALUES (?, ?, ?, 'calls', 1.0, 'ast_resolved', 0, 0, '', 0, 0)`,
		staticHash[:], staticSource[:], staticTarget[:],
	)
	if err != nil {
		t.Fatalf("insert static edge: %v", err)
	}

	// Insert an active runtime edge (observed recently).
	activeSource := types.ComputeNodeHash("svc-a", "", types.EmptyHash, "svc-a", "service")
	activeTarget := types.ComputeNodeHash("svc-b", "", types.EmptyHash, "svc-b", "service")
	activeHash := types.ComputeEdgeHash(activeSource, activeTarget, "runtime_calls", "otel_trace")
	_, err = db.ExecContext(ctx,
		`INSERT INTO edges (edge_hash, source_hash, target_hash, edge_type, confidence, provenance, callsite_line, callsite_col, callsite_file, observation_count, last_observed)
		 VALUES (?, ?, ?, 'runtime_calls', 0.85, 'otel_trace', 0, 0, '', 100, ?)`,
		activeHash[:], activeSource[:], activeTarget[:], now,
	)
	if err != nil {
		t.Fatalf("insert active edge: %v", err)
	}

	// Insert a stale runtime edge (not observed in 60 days).
	staleSource := types.ComputeNodeHash("svc-c", "", types.EmptyHash, "svc-c", "service")
	staleTarget := types.ComputeNodeHash("svc-d", "", types.EmptyHash, "svc-d", "service")
	staleHash := types.ComputeEdgeHash(staleSource, staleTarget, "runtime_rpc", "otel_trace")
	_, err = db.ExecContext(ctx,
		`INSERT INTO edges (edge_hash, source_hash, target_hash, edge_type, confidence, provenance, callsite_line, callsite_col, callsite_file, observation_count, last_observed)
		 VALUES (?, ?, ?, 'runtime_rpc', 0.5, 'otel_trace', 0, 0, '', 5, ?)`,
		staleHash[:], staleSource[:], staleTarget[:], now-60*86400,
	)
	if err != nil {
		t.Fatalf("insert stale edge: %v", err)
	}

	resolver := NewSymbolResolver(db)
	ing := NewIngestor(db, resolver, TraceIngestConfig{})

	stats, err := ing.RuntimeEdgeStats(ctx, types.EmptyHash)
	if err != nil {
		t.Fatalf("RuntimeEdgeStats: %v", err)
	}

	if stats.TotalEdges != 2 {
		t.Errorf("TotalEdges: got %d, want 2", stats.TotalEdges)
	}
	if stats.ActiveCount != 1 {
		t.Errorf("ActiveCount: got %d, want 1", stats.ActiveCount)
	}
	if stats.StaleCount != 1 {
		t.Errorf("StaleCount: got %d, want 1", stats.StaleCount)
	}
	if stats.BySourceType["runtime_calls"] != 1 {
		t.Errorf("BySourceType[runtime_calls]: got %d, want 1", stats.BySourceType["runtime_calls"])
	}
	if stats.BySourceType["runtime_rpc"] != 1 {
		t.Errorf("BySourceType[runtime_rpc]: got %d, want 1", stats.BySourceType["runtime_rpc"])
	}
}

func TestDecayConfidence(t *testing.T) {
	db := setupIngestDB(t)
	ctx := context.Background()

	now := time.Now().Unix()

	// Insert a stale runtime edge (not observed in 45 days, confidence 0.85).
	source := types.ComputeNodeHash("svc-a", "", types.EmptyHash, "svc-a", "service")
	target := types.ComputeNodeHash("svc-b", "", types.EmptyHash, "svc-b", "service")
	hash := types.ComputeEdgeHash(source, target, "runtime_calls", "otel_trace")
	_, err := db.ExecContext(ctx,
		`INSERT INTO edges (edge_hash, source_hash, target_hash, edge_type, confidence, provenance, callsite_line, callsite_col, callsite_file, observation_count, last_observed)
		 VALUES (?, ?, ?, 'runtime_calls', 0.85, 'otel_trace', 0, 0, '', 100, ?)`,
		hash[:], source[:], target[:], now-45*86400,
	)
	if err != nil {
		t.Fatalf("insert edge: %v", err)
	}

	resolver := NewSymbolResolver(db)
	ing := NewIngestor(db, resolver, TraceIngestConfig{})

	updated, err := ing.DecayConfidence(ctx)
	if err != nil {
		t.Fatalf("DecayConfidence: %v", err)
	}
	if updated != 1 {
		t.Errorf("updated: got %d, want 1", updated)
	}

	// Verify confidence was reduced to 0.2.
	var confidence float64
	err = db.QueryRowContext(ctx, `SELECT confidence FROM edges WHERE edge_hash = ?`, hash[:]).Scan(&confidence)
	if err != nil {
		t.Fatalf("query confidence: %v", err)
	}
	if confidence != 0.2 {
		t.Errorf("confidence: got %f, want 0.2", confidence)
	}
}

func TestBatchAccumulation(t *testing.T) {
	db := setupIngestDB(t)
	resolver := NewSymbolResolver(db)
	ing := NewIngestor(db, resolver, TraceIngestConfig{BatchSize: 3})
	ctx := context.Background()

	// Add two spans (below batch size, should not auto-flush).
	ing.AddToBatch(TraceSpan{
		TraceID:     "t1",
		ServiceName: "svc-a",
		PeerService: "svc-b",
		Attributes:  map[string]string{"http.method": "GET", "http.route": "/a"},
	})
	ing.AddToBatch(TraceSpan{
		TraceID:     "t2",
		ServiceName: "svc-a",
		PeerService: "svc-c",
		Attributes:  map[string]string{"http.method": "POST", "http.route": "/b"},
	})

	// Verify no edges yet (batch not flushed).
	var count int
	err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM edges`).Scan(&count)
	if err != nil {
		t.Fatalf("count edges: %v", err)
	}
	if count != 0 {
		t.Errorf("edges before flush: got %d, want 0", count)
	}

	// Manually flush.
	if err := ing.FlushBatch(ctx); err != nil {
		t.Fatalf("FlushBatch: %v", err)
	}

	err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM edges`).Scan(&count)
	if err != nil {
		t.Fatalf("count edges after flush: %v", err)
	}
	if count != 2 {
		t.Errorf("edges after flush: got %d, want 2", count)
	}

	// Verify pending batch is cleared.
	ing.mu.Lock()
	pendingLen := len(ing.pending)
	ing.mu.Unlock()
	if pendingLen != 0 {
		t.Errorf("pending after flush: got %d, want 0", pendingLen)
	}
}

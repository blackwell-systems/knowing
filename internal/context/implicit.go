package context

import (
	"strings"
	"sync"
	"time"

	"github.com/blackwell-systems/knowing/internal/types"
)

// ImplicitFeedback tracks symbols returned by context_for_task and detects
// when the agent subsequently uses them (in Edit tool calls, file references,
// etc). When a returned symbol is "used," positive feedback is auto-recorded.
//
// This closes the feedback loop without requiring explicit agent cooperation:
// the agent just uses context naturally, and the system learns which symbols
// were actually useful.
//
// Attribution window: symbols remain attributable for 10 minutes after being
// returned. After that, the association expires (the agent may be working on
// something else). A new context_for_task call resets the window.
type ImplicitFeedback struct {
	mu sync.Mutex

	// pending maps symbol hash -> PendingAttribution for symbols returned
	// by the most recent context_for_task call(s).
	pending map[types.Hash]*PendingAttribution

	// nameIndex maps lowercase symbol short names to their hashes for fast
	// lookup when matching against tool call content.
	nameIndex map[string][]types.Hash

	// attributed tracks which symbols have already been attributed this session
	// to avoid recording duplicate feedback for the same symbol.
	attributed map[types.Hash]bool

	// attributionWindow is how long a symbol remains attributable after being
	// returned by context_for_task.
	attributionWindow time.Duration
}

// PendingAttribution represents a symbol awaiting implicit attribution.
type PendingAttribution struct {
	Hash       types.Hash
	Name       string // short name (last component of qualified name)
	QualName   string // full qualified name
	ReturnedAt time.Time
	Score      float64 // ranking score when returned (higher = more confident attribution)
}

// NewImplicitFeedback creates a new implicit feedback tracker.
func NewImplicitFeedback() *ImplicitFeedback {
	return &ImplicitFeedback{
		pending:           make(map[types.Hash]*PendingAttribution),
		nameIndex:         make(map[string][]types.Hash),
		attributed:        make(map[types.Hash]bool),
		attributionWindow: 10 * time.Minute,
	}
}

// RegisterReturned records symbols that were just returned by context_for_task.
// These become candidates for implicit attribution when the agent subsequently
// references them in tool calls.
func (f *ImplicitFeedback) RegisterReturned(symbols []RankedSymbol) {
	f.mu.Lock()
	defer f.mu.Unlock()

	now := time.Now()

	for _, sym := range symbols {
		hash := sym.Node.NodeHash
		shortName := extractShortName(sym.Node.QualifiedName)

		f.pending[hash] = &PendingAttribution{
			Hash:       hash,
			Name:       shortName,
			QualName:   sym.Node.QualifiedName,
			ReturnedAt: now,
			Score:      sym.Score,
		}

		// Index by lowercase short name for fast content matching.
		key := strings.ToLower(shortName)
		if key != "" && len(key) > 2 {
			f.nameIndex[key] = append(f.nameIndex[key], hash)
		}
	}
}

// DetectUsed scans tool call content (e.g., Edit old_string, file paths) for
// references to pending symbols. Returns the hashes of symbols that appear to
// have been used by the agent.
//
// Detection strategy:
//   - Extract identifiers from the content (CamelCase words, snake_case, dotted paths)
//   - Match against the name index of pending symbols
//   - Only match symbols within the attribution window
//   - Skip symbols already attributed this session
func (f *ImplicitFeedback) DetectUsed(content string) []types.Hash {
	if content == "" {
		return nil
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	now := time.Now()
	var used []types.Hash

	// Extract all identifiers from the content.
	identifiers := extractIdentifiers(content)

	for _, ident := range identifiers {
		key := strings.ToLower(ident)
		hashes, ok := f.nameIndex[key]
		if !ok {
			continue
		}

		for _, h := range hashes {
			// Skip if already attributed.
			if f.attributed[h] {
				continue
			}

			attr, ok := f.pending[h]
			if !ok {
				continue
			}

			// Check attribution window.
			if now.Sub(attr.ReturnedAt) > f.attributionWindow {
				continue
			}

			used = append(used, h)
			f.attributed[h] = true
		}
	}

	return used
}

// PendingCount returns the number of symbols awaiting attribution.
func (f *ImplicitFeedback) PendingCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.pending)
}

// AttributedCount returns the number of symbols that have been implicitly attributed.
func (f *ImplicitFeedback) AttributedCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.attributed)
}

// Expire removes symbols that have exceeded the attribution window and returns
// the hashes of symbols that expired WITHOUT being used. These represent
// "returned but not useful" symbols that should receive negative feedback.
//
// The negative signal is what makes implicit feedback work: it's not enough to
// boost used symbols; we must also penalize unused ones so they rank lower next
// time. Without this asymmetry, positive-only feedback doesn't shift rankings.
func (f *ImplicitFeedback) Expire() []types.Hash {
	f.mu.Lock()
	defer f.mu.Unlock()

	now := time.Now()
	var unused []types.Hash

	for h, attr := range f.pending {
		if now.Sub(attr.ReturnedAt) > f.attributionWindow {
			// If this symbol was never attributed, it's an unused result.
			if !f.attributed[h] {
				unused = append(unused, h)
			}
			delete(f.pending, h)
			// Clean name index.
			key := strings.ToLower(attr.Name)
			if hashes, ok := f.nameIndex[key]; ok {
				filtered := hashes[:0]
				for _, existing := range hashes {
					if existing != h {
						filtered = append(filtered, existing)
					}
				}
				if len(filtered) == 0 {
					delete(f.nameIndex, key)
				} else {
					f.nameIndex[key] = filtered
				}
			}
		}
	}

	return unused
}

// ExpireAndReport is like Expire but also accepts a callback for processing
// unused symbols. This avoids the caller needing to hold onto the returned
// slice when immediate processing is preferred.
func (f *ImplicitFeedback) ExpireAndReport(onUnused func(hash types.Hash)) {
	unused := f.Expire()
	for _, h := range unused {
		onUnused(h)
	}
}

// FlushUnused forces expiration of ALL pending symbols and returns unused ones.
// Use at end of a context_for_task call cycle (new query replaces old context)
// or at session end. This ensures timely negative feedback without waiting for
// the attribution window.
//
// Also clears the attributed set for the flushed batch so the same symbols can
// be re-attributed in the next cycle. Each context_for_task call starts a fresh
// attribution cycle.
func (f *ImplicitFeedback) FlushUnused() []types.Hash {
	f.mu.Lock()
	defer f.mu.Unlock()

	var unused []types.Hash
	for h := range f.pending {
		if !f.attributed[h] {
			unused = append(unused, h)
		}
	}

	// Clear everything for the next cycle.
	f.pending = make(map[types.Hash]*PendingAttribution)
	f.nameIndex = make(map[string][]types.Hash)
	f.attributed = make(map[types.Hash]bool)

	return unused
}

// Reset clears all pending attributions and the attributed set.
// Call when the session context shifts significantly.
func (f *ImplicitFeedback) Reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pending = make(map[types.Hash]*PendingAttribution)
	f.nameIndex = make(map[string][]types.Hash)
	f.attributed = make(map[types.Hash]bool)
}

// extractShortName gets the last component of a qualified name.
// "github.com/org/repo://pkg/sub.FunctionName" -> "FunctionName"
func extractShortName(qn string) string {
	// Find the last dot or slash.
	lastDot := strings.LastIndex(qn, ".")
	if lastDot >= 0 {
		return qn[lastDot+1:]
	}
	lastSlash := strings.LastIndex(qn, "/")
	if lastSlash >= 0 {
		return qn[lastSlash+1:]
	}
	return qn
}

// extractIdentifiers extracts potential symbol identifiers from content.
// Looks for CamelCase words, snake_case words, and dotted paths.
func extractIdentifiers(content string) []string {
	var result []string
	seen := make(map[string]bool)

	// Split on non-identifier characters.
	words := splitOnNonIdent(content)
	for _, w := range words {
		if len(w) <= 2 {
			continue
		}
		lower := strings.ToLower(w)
		if seen[lower] {
			continue
		}
		seen[lower] = true
		result = append(result, w)
	}
	return result
}

// splitOnNonIdent splits content into identifier-like tokens.
// Keeps letters, digits, and underscores together. Dots are split points
// but we also extract the dotted segments.
func splitOnNonIdent(s string) []string {
	var tokens []string
	var current []byte

	for i := 0; i < len(s); i++ {
		c := s[i]
		if isIdentChar(c) {
			current = append(current, c)
		} else {
			if len(current) > 0 {
				tokens = append(tokens, string(current))
				current = current[:0]
			}
		}
	}
	if len(current) > 0 {
		tokens = append(tokens, string(current))
	}
	return tokens
}

func isIdentChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
		(c >= '0' && c <= '9') || c == '_'
}

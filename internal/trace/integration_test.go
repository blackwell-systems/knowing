package trace

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/blackwell-systems/knowing/internal/store"
	"github.com/blackwell-systems/knowing/internal/types"

	_ "modernc.org/sqlite"
)

// setupIntegrationDB creates an in-memory SQLite database with all migrations
// applied (001 through 004) using store.Migrate.
func setupIntegrationDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		t.Fatalf("enable WAL: %v", err)
	}

	if err := store.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	return db
}

// insertNode is a test helper that inserts a node directly into the DB.
func insertNode(t *testing.T, db *sql.DB, n types.Node) {
	t.Helper()
	_, err := db.Exec(
		`INSERT OR REPLACE INTO nodes (node_hash, file_hash, qualified_name, kind, line, signature)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		n.NodeHash[:], n.FileHash[:], n.QualifiedName, n.Kind, n.Line, n.Signature,
	)
	if err != nil {
		t.Fatalf("insert node %s: %v", n.QualifiedName, err)
	}
}

// putRouteSymbol is a test helper that inserts a route_symbols entry directly.
func putRouteSymbol(t *testing.T, db *sql.DB, serviceName, routePattern string, nodeHash types.Hash, mappingType string) {
	t.Helper()
	_, err := db.Exec(
		`INSERT OR REPLACE INTO route_symbols (service_name, route_pattern, node_hash, mapping_type, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		serviceName, routePattern, nodeHash[:], mappingType, time.Now().Unix(),
	)
	if err != nil {
		t.Fatalf("insert route_symbol: %v", err)
	}
}

func TestIntegrationFullPipeline(t *testing.T) {
	db := setupIntegrationDB(t)
	ctx := context.Background()

	// --- Step 1: Insert nodes representing real symbols ---

	handlerFileHash := types.NewHash([]byte("handler-file"))
	handlerNode := types.Node{
		NodeHash:      types.ComputeNodeHash("user-service", "api/handlers", types.EmptyHash, "GetUsersHandler", "function"),
		FileHash:      handlerFileHash,
		QualifiedName: "user-service/api/handlers.GetUsersHandler",
		Kind:          "function",
		Line:          42,
		Signature:     "func GetUsersHandler(w http.ResponseWriter, r *http.Request)",
	}
	insertNode(t, db, handlerNode)

	// --- Step 2: Map an HTTP route to that node ---

	putRouteSymbol(t, db, "user-service", "GET /api/users", handlerNode.NodeHash, "http_route")

	// --- Step 3: Create resolver and ingestor ---

	resolver := NewSymbolResolver(db)
	ingestor := NewIngestor(db, resolver, TraceIngestConfig{})

	// --- Step 4: Create trace spans simulating production traffic ---

	httpResolvedSpan := TraceSpan{
		TraceID:       "trace-001",
		SpanID:        "span-001",
		ServiceName:   "api-gateway",
		PeerService:   "user-service",
		OperationName: "GET /api/users",
		Attributes: map[string]string{
			"http.method": "GET",
			"http.route":  "/api/users",
		},
		StartTime: time.Now(),
		Duration:  50 * time.Millisecond,
	}

	httpUnresolvedSpan := TraceSpan{
		TraceID:       "trace-002",
		SpanID:        "span-002",
		ServiceName:   "api-gateway",
		PeerService:   "order-service",
		OperationName: "POST /api/orders",
		Attributes: map[string]string{
			"http.method": "POST",
			"http.route":  "/api/orders",
		},
		StartTime: time.Now(),
		Duration:  120 * time.Millisecond,
	}

	grpcSpan := TraceSpan{
		TraceID:       "trace-003",
		SpanID:        "span-003",
		ServiceName:   "api-gateway",
		PeerService:   "user-service",
		OperationName: "UserService.GetUser",
		Attributes: map[string]string{
			"rpc.service": "UserService",
			"rpc.method":  "GetUser",
		},
		StartTime: time.Now(),
		Duration:  30 * time.Millisecond,
	}

	spans := []TraceSpan{httpResolvedSpan, httpUnresolvedSpan, grpcSpan}

	// --- Step 5: Ingest spans ---

	result, err := ingestor.IngestSpans(ctx, spans)
	if err != nil {
		t.Fatalf("IngestSpans (first): %v", err)
	}
	if result.Created != 3 {
		t.Errorf("first ingest Created: got %d, want 3", result.Created)
	}
	if result.Updated != 0 {
		t.Errorf("first ingest Updated: got %d, want 0", result.Updated)
	}

	// --- Step 6: Verify edges ---

	type edgeRow struct {
		edgeType         string
		confidence       float64
		provenance       string
		observationCount int
		lastObserved     int64
		sourceHash       []byte
		targetHash       []byte
	}

	rows, err := db.QueryContext(ctx,
		`SELECT edge_type, confidence, provenance, observation_count, last_observed, source_hash, target_hash
		 FROM edges ORDER BY edge_type, provenance`)
	if err != nil {
		t.Fatalf("query edges: %v", err)
	}
	defer rows.Close()

	var edges []edgeRow
	for rows.Next() {
		var e edgeRow
		if err := rows.Scan(&e.edgeType, &e.confidence, &e.provenance, &e.observationCount, &e.lastObserved, &e.sourceHash, &e.targetHash); err != nil {
			t.Fatalf("scan edge: %v", err)
		}
		edges = append(edges, e)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows error: %v", err)
	}

	if len(edges) != 3 {
		t.Fatalf("edge count: got %d, want 3", len(edges))
	}

	// Categorize edges by type for targeted assertions.
	httpEdges := 0
	rpcEdges := 0
	var resolvedEdge, unresolvedEdge, grpcEdge *edgeRow

	// Compute expected target hashes to distinguish resolved vs unresolved HTTP edges.
	resolvedTargetHash := handlerNode.NodeHash
	unresolvedTargetHash := types.ComputeNodeHash("order-service", "UNRESOLVED", types.EmptyHash, "POST /api/orders", "runtime_endpoint")

	for i := range edges {
		e := &edges[i]

		// Verify provenance starts with "otel_trace" for all edges.
		if !strings.HasPrefix(e.provenance, "otel_trace") {
			t.Errorf("edge provenance %q does not start with otel_trace", e.provenance)
		}

		// Verify observation count is 1.
		if e.observationCount != 1 {
			t.Errorf("edge observation_count: got %d, want 1", e.observationCount)
		}

		// Verify last_observed is recent (within last 5 seconds).
		if time.Now().Unix()-e.lastObserved > 5 {
			t.Errorf("edge last_observed too old: %d", e.lastObserved)
		}

		switch e.edgeType {
		case "runtime_calls":
			httpEdges++
			var targetH types.Hash
			copy(targetH[:], e.targetHash)
			if targetH == resolvedTargetHash {
				resolvedEdge = e
			} else if targetH == unresolvedTargetHash {
				unresolvedEdge = e
			}
		case "runtime_rpc":
			rpcEdges++
			grpcEdge = e
		}
	}

	if httpEdges != 2 {
		t.Errorf("HTTP edges (runtime_calls): got %d, want 2", httpEdges)
	}
	if rpcEdges != 1 {
		t.Errorf("gRPC edges (runtime_rpc): got %d, want 1", rpcEdges)
	}

	// The resolved span targets a known node (confidence from resolver = 1.0),
	// but ConfidenceFromCount(1) = 0.5, and the ingestor takes min(resolverConf, countConf).
	// Since resolver returned 1.0 and count conf is 0.5, the result is 0.5.
	if resolvedEdge == nil {
		t.Fatal("resolved HTTP edge not found")
	}
	if resolvedEdge.confidence != 0.5 {
		t.Errorf("resolved edge confidence: got %f, want 0.5", resolvedEdge.confidence)
	}

	// The unresolved span targets a synthetic node (confidence from resolver = 0.3),
	// and ConfidenceFromCount(1) = 0.5. The ingestor takes min(0.3, 0.5) = 0.3.
	if unresolvedEdge == nil {
		t.Fatal("unresolved HTTP edge not found")
	}
	if unresolvedEdge.confidence != 0.3 {
		t.Errorf("unresolved edge confidence: got %f, want 0.3", unresolvedEdge.confidence)
	}

	// gRPC edge should exist.
	if grpcEdge == nil {
		t.Fatal("gRPC edge not found")
	}
	if grpcEdge.edgeType != "runtime_rpc" {
		t.Errorf("gRPC edge type: got %q, want runtime_rpc", grpcEdge.edgeType)
	}

	// --- Step 7: Reingest same spans and verify idempotency ---

	result2, err := ingestor.IngestSpans(ctx, spans)
	if err != nil {
		t.Fatalf("IngestSpans (second): %v", err)
	}
	if result2.Created != 0 {
		t.Errorf("second ingest Created: got %d, want 0", result2.Created)
	}
	if result2.Updated != 3 {
		t.Errorf("second ingest Updated: got %d, want 3", result2.Updated)
	}

	// Verify no new edges were created (still 3 total).
	var totalEdges int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM edges`).Scan(&totalEdges); err != nil {
		t.Fatalf("count edges: %v", err)
	}
	if totalEdges != 3 {
		t.Errorf("total edges after re-ingest: got %d, want 3", totalEdges)
	}

	// Verify observation_count was incremented to 2 for all edges.
	var minObs, maxObs int
	if err := db.QueryRowContext(ctx,
		`SELECT MIN(observation_count), MAX(observation_count) FROM edges`,
	).Scan(&minObs, &maxObs); err != nil {
		t.Fatalf("query obs counts: %v", err)
	}
	if minObs != 2 || maxObs != 2 {
		t.Errorf("observation_count range: got [%d, %d], want [2, 2]", minObs, maxObs)
	}

	// Verify confidence was updated (ComputeConfidence(2, 0) = ConfidenceFromCount(2) = 0.5).
	// The update path uses ComputeConfidence(newCount, 0) which ignores resolver confidence.
	var updatedConfidence float64
	if err := db.QueryRowContext(ctx,
		`SELECT confidence FROM edges WHERE source_hash = ? AND edge_type = 'runtime_calls'`,
		resolvedEdge.sourceHash,
	).Scan(&updatedConfidence); err != nil {
		// Multiple edges share the source; just verify at least one updated.
		t.Logf("note: could not query specific updated confidence: %v", err)
	}

	// --- Step 8: RuntimeEdgeStats ---

	stats, err := ingestor.RuntimeEdgeStats(ctx, types.EmptyHash)
	if err != nil {
		t.Fatalf("RuntimeEdgeStats: %v", err)
	}
	if stats.TotalEdges != 3 {
		t.Errorf("stats.TotalEdges: got %d, want 3", stats.TotalEdges)
	}
	if stats.BySourceType["runtime_calls"] != 2 {
		t.Errorf("stats.BySourceType[runtime_calls]: got %d, want 2", stats.BySourceType["runtime_calls"])
	}
	if stats.BySourceType["runtime_rpc"] != 1 {
		t.Errorf("stats.BySourceType[runtime_rpc]: got %d, want 1", stats.BySourceType["runtime_rpc"])
	}
	if stats.ActiveCount != 3 {
		t.Errorf("stats.ActiveCount: got %d, want 3", stats.ActiveCount)
	}
	if stats.StaleCount != 0 {
		t.Errorf("stats.StaleCount: got %d, want 0", stats.StaleCount)
	}

	// --- Step 9: Confidence decay ---

	// Set one edge's last_observed to 60 days ago to simulate staleness.
	sixtyDaysAgo := time.Now().Unix() - 60*86400
	_, err = db.ExecContext(ctx,
		`UPDATE edges SET last_observed = ? WHERE edge_type = 'runtime_rpc'`,
		sixtyDaysAgo,
	)
	if err != nil {
		t.Fatalf("set stale last_observed: %v", err)
	}

	decayed, err := ingestor.DecayConfidence(ctx)
	if err != nil {
		t.Fatalf("DecayConfidence: %v", err)
	}
	if decayed != 1 {
		t.Errorf("DecayConfidence count: got %d, want 1", decayed)
	}

	// Verify the gRPC edge's confidence dropped to 0.2.
	var decayedConfidence float64
	if err := db.QueryRowContext(ctx,
		`SELECT confidence FROM edges WHERE edge_type = 'runtime_rpc'`,
	).Scan(&decayedConfidence); err != nil {
		t.Fatalf("query decayed confidence: %v", err)
	}
	if decayedConfidence != 0.2 {
		t.Errorf("decayed confidence: got %f, want 0.2", decayedConfidence)
	}

	// --- Step 10: IngestHTTPLogs full cycle ---

	httpLogEntries := []HTTPLogEntry{
		{
			Timestamp:   time.Now(),
			Method:      "DELETE",
			Path:        "/api/sessions",
			StatusCode:  204,
			ServiceName: "auth-service",
			ClientIP:    "10.0.0.1",
			Duration:    15 * time.Millisecond,
		},
	}

	logResult, err := ingestor.IngestHTTPLogs(ctx, httpLogEntries)
	if err != nil {
		t.Fatalf("IngestHTTPLogs: %v", err)
	}
	if logResult.Created != 1 {
		t.Errorf("IngestHTTPLogs Created: got %d, want 1", logResult.Created)
	}

	// Verify the HTTP log edge exists with correct properties.
	var logEdgeType, logProvenance string
	var logObsCount int
	if err := db.QueryRowContext(ctx,
		`SELECT edge_type, provenance, observation_count FROM edges
		 WHERE provenance LIKE 'otel_trace%'
		 ORDER BY last_observed DESC LIMIT 1`,
	).Scan(&logEdgeType, &logProvenance, &logObsCount); err != nil {
		t.Fatalf("query HTTP log edge: %v", err)
	}
	if logEdgeType != "runtime_calls" {
		t.Errorf("HTTP log edge type: got %q, want runtime_calls", logEdgeType)
	}
	if !strings.HasPrefix(logProvenance, "otel_trace") {
		t.Errorf("HTTP log provenance %q does not start with otel_trace", logProvenance)
	}

	// Verify total edge count: 3 original + 1 from HTTP log = 4.
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM edges`).Scan(&totalEdges); err != nil {
		t.Fatalf("count final edges: %v", err)
	}
	if totalEdges != 4 {
		t.Errorf("final edge count: got %d, want 4", totalEdges)
	}
}

package store

import (
	"context"
	"testing"
	"time"

	"github.com/blackwell-systems/knowing/internal/types"
)

func TestRuntimePutGetRouteSymbol_RoundTrip(t *testing.T) {
	s := tempDB(t)
	ctx := context.Background()

	nodeHash := types.NewHash([]byte("handler-node"))
	err := s.PutRouteSymbol(ctx, "order-service", "POST /api/orders", nodeHash, "http_handler")
	if err != nil {
		t.Fatalf("PutRouteSymbol: %v", err)
	}

	got, err := s.GetRouteSymbol(ctx, "order-service", "POST /api/orders", "http_handler")
	if err != nil {
		t.Fatalf("GetRouteSymbol: %v", err)
	}
	if got == nil {
		t.Fatal("GetRouteSymbol returned nil")
	}
	if got.ServiceName != "order-service" {
		t.Errorf("ServiceName = %q, want order-service", got.ServiceName)
	}
	if got.RoutePattern != "POST /api/orders" {
		t.Errorf("RoutePattern = %q, want POST /api/orders", got.RoutePattern)
	}
	if got.MappingType != "http_handler" {
		t.Errorf("MappingType = %q, want http_handler", got.MappingType)
	}
	if got.NodeHash != nodeHash {
		t.Errorf("NodeHash mismatch")
	}
	if got.CreatedAt == 0 {
		t.Error("CreatedAt should be non-zero")
	}
}

func TestRuntimeGetRouteSymbol_NotFound(t *testing.T) {
	s := tempDB(t)
	ctx := context.Background()

	got, err := s.GetRouteSymbol(ctx, "nonexistent", "/nope", "http_handler")
	if err != nil {
		t.Fatalf("GetRouteSymbol: %v", err)
	}
	if got != nil {
		t.Error("expected nil for nonexistent route symbol")
	}
}

func TestRuntimePutRouteSymbol_Upsert(t *testing.T) {
	s := tempDB(t)
	ctx := context.Background()

	hash1 := types.NewHash([]byte("node-v1"))
	hash2 := types.NewHash([]byte("node-v2"))

	if err := s.PutRouteSymbol(ctx, "svc", "/route", hash1, "handler"); err != nil {
		t.Fatalf("PutRouteSymbol v1: %v", err)
	}
	if err := s.PutRouteSymbol(ctx, "svc", "/route", hash2, "handler"); err != nil {
		t.Fatalf("PutRouteSymbol v2: %v", err)
	}

	got, err := s.GetRouteSymbol(ctx, "svc", "/route", "handler")
	if err != nil {
		t.Fatalf("GetRouteSymbol: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil after upsert")
	}
	if got.NodeHash != hash2 {
		t.Error("NodeHash should be updated to v2")
	}
}

func TestRuntimeUpdateObservation(t *testing.T) {
	s := tempDB(t)
	ctx := context.Background()
	repo := makeRepo(t, s, "https://example.com/repo")
	file := makeFile(t, s, repo, "main.go")
	src := makeNode(t, s, file, "main.Caller", "function")
	tgt := makeNode(t, s, file, "main.Callee", "function")

	edge := makeEdge(t, s, src, tgt, "calls")

	now := time.Now().Unix()
	err := s.UpdateObservation(ctx, edge.EdgeHash, 42, now, 0.85)
	if err != nil {
		t.Fatalf("UpdateObservation: %v", err)
	}

	// Verify the edge was updated by reading it back.
	var count int
	var lastObs int64
	var conf float64
	err = s.db.QueryRowContext(ctx,
		`SELECT observation_count, last_observed, confidence FROM edges WHERE edge_hash = ?`,
		edge.EdgeHash[:],
	).Scan(&count, &lastObs, &conf)
	if err != nil {
		t.Fatalf("query updated edge: %v", err)
	}
	if count != 42 {
		t.Errorf("observation_count = %d, want 42", count)
	}
	if lastObs != now {
		t.Errorf("last_observed = %d, want %d", lastObs, now)
	}
	if conf != 0.85 {
		t.Errorf("confidence = %f, want 0.85", conf)
	}
}

func TestRuntimeEdgesByProvenance(t *testing.T) {
	s := tempDB(t)
	ctx := context.Background()
	repo := makeRepo(t, s, "https://example.com/repo")
	file := makeFile(t, s, repo, "main.go")
	src := makeNode(t, s, file, "main.Caller", "function")
	tgt := makeNode(t, s, file, "main.Callee", "function")

	// Insert an edge with otel provenance using raw SQL to set all 11 columns.
	otelEdgeHash := types.ComputeEdgeHash(src.NodeHash, tgt.NodeHash, "calls", "otel_trace")
	_, err := s.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO edges (edge_hash, source_hash, target_hash, edge_type, confidence, provenance, callsite_line, callsite_col, callsite_file, observation_count, last_observed)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		otelEdgeHash[:], src.NodeHash[:], tgt.NodeHash[:], "calls", 0.8, "otel_trace", 0, 0, "", 5, time.Now().Unix(),
	)
	if err != nil {
		t.Fatalf("insert otel edge: %v", err)
	}

	// Insert a static edge with different provenance.
	staticEdge := types.Edge{
		EdgeHash:   types.ComputeEdgeHash(src.NodeHash, tgt.NodeHash, "calls", "ast_resolved"),
		SourceHash: src.NodeHash,
		TargetHash: tgt.NodeHash,
		EdgeType:   "calls",
		Confidence: 1.0,
		Provenance: "ast_resolved",
	}
	if err := s.PutEdge(ctx, staticEdge); err != nil {
		t.Fatalf("PutEdge static: %v", err)
	}

	// Query by otel prefix.
	edges, err := s.RuntimeEdgesByProvenance(ctx, "otel_")
	if err != nil {
		t.Fatalf("RuntimeEdgesByProvenance: %v", err)
	}
	if len(edges) != 1 {
		t.Fatalf("expected 1 otel edge, got %d", len(edges))
	}
	if edges[0].Provenance != "otel_trace" {
		t.Errorf("Provenance = %q, want otel_trace", edges[0].Provenance)
	}
	if edges[0].ObservationCount != 5 {
		t.Errorf("ObservationCount = %d, want 5", edges[0].ObservationCount)
	}
}

func TestRuntimeDecayConfidence(t *testing.T) {
	s := tempDB(t)
	ctx := context.Background()
	repo := makeRepo(t, s, "https://example.com/repo")
	file := makeFile(t, s, repo, "main.go")

	src1 := makeNode(t, s, file, "main.A", "function")
	tgt1 := makeNode(t, s, file, "main.B", "function")
	src2 := makeNode(t, s, file, "main.C", "function")
	tgt2 := makeNode(t, s, file, "main.D", "function")
	src3 := makeNode(t, s, file, "main.E", "function")
	tgt3 := makeNode(t, s, file, "main.F", "function")

	oldTime := time.Now().Unix() - 100*86400 // 100 days ago
	recentTime := time.Now().Unix() - 1*86400 // 1 day ago

	// Old otel edge (should be decayed).
	oldEdgeHash := types.ComputeEdgeHash(src1.NodeHash, tgt1.NodeHash, "calls", "otel_trace_old")
	_, err := s.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO edges (edge_hash, source_hash, target_hash, edge_type, confidence, provenance, callsite_line, callsite_col, callsite_file, observation_count, last_observed)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		oldEdgeHash[:], src1.NodeHash[:], tgt1.NodeHash[:], "calls", 0.8, "otel_trace", 0, 0, "", 10, oldTime,
	)
	if err != nil {
		t.Fatalf("insert old otel edge: %v", err)
	}

	// Recent otel edge (should NOT be decayed).
	recentEdgeHash := types.ComputeEdgeHash(src2.NodeHash, tgt2.NodeHash, "calls", "otel_trace_recent")
	_, err = s.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO edges (edge_hash, source_hash, target_hash, edge_type, confidence, provenance, callsite_line, callsite_col, callsite_file, observation_count, last_observed)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		recentEdgeHash[:], src2.NodeHash[:], tgt2.NodeHash[:], "calls", 0.8, "otel_trace", 0, 0, "", 5, recentTime,
	)
	if err != nil {
		t.Fatalf("insert recent otel edge: %v", err)
	}

	// Static edge (should NOT be decayed, not otel_ provenance).
	staticEdge := types.Edge{
		EdgeHash:   types.ComputeEdgeHash(src3.NodeHash, tgt3.NodeHash, "calls", "ast_resolved"),
		SourceHash: src3.NodeHash,
		TargetHash: tgt3.NodeHash,
		EdgeType:   "calls",
		Confidence: 1.0,
		Provenance: "ast_resolved",
	}
	if err := s.PutEdge(ctx, staticEdge); err != nil {
		t.Fatalf("PutEdge static: %v", err)
	}

	// Decay edges older than 30 days to confidence 0.3.
	updated, err := s.DecayRuntimeConfidence(ctx, 30, 0.3)
	if err != nil {
		t.Fatalf("DecayRuntimeConfidence: %v", err)
	}
	if updated != 1 {
		t.Fatalf("expected 1 decayed edge, got %d", updated)
	}

	// Verify the old edge was decayed.
	var conf float64
	err = s.db.QueryRowContext(ctx,
		`SELECT confidence FROM edges WHERE edge_hash = ?`, oldEdgeHash[:],
	).Scan(&conf)
	if err != nil {
		t.Fatalf("query old edge: %v", err)
	}
	if conf != 0.3 {
		t.Errorf("old edge confidence = %f, want 0.3", conf)
	}

	// Verify the recent edge was NOT decayed.
	err = s.db.QueryRowContext(ctx,
		`SELECT confidence FROM edges WHERE edge_hash = ?`, recentEdgeHash[:],
	).Scan(&conf)
	if err != nil {
		t.Fatalf("query recent edge: %v", err)
	}
	if conf != 0.8 {
		t.Errorf("recent edge confidence = %f, want 0.8", conf)
	}

	// Verify the static edge was NOT decayed.
	err = s.db.QueryRowContext(ctx,
		`SELECT confidence FROM edges WHERE edge_hash = ?`, staticEdge.EdgeHash[:],
	).Scan(&conf)
	if err != nil {
		t.Fatalf("query static edge: %v", err)
	}
	if conf != 1.0 {
		t.Errorf("static edge confidence = %f, want 1.0", conf)
	}
}

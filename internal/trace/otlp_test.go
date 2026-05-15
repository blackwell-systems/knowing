package trace

import (
	"context"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestOTLPReceiver_NewOTLPReceiver(t *testing.T) {
	db := setupIngestDB(t)
	resolver := NewSymbolResolver(db)
	ing := NewIngestor(db, resolver, TraceIngestConfig{})

	recv := NewOTLPReceiver(":4317", ing)
	if recv == nil {
		t.Fatal("NewOTLPReceiver returned nil")
	}
	if recv.Addr() != ":4317" {
		t.Errorf("Addr() = %q, want %q", recv.Addr(), ":4317")
	}
	if recv.Health() != HealthDisabled {
		t.Errorf("initial Health() = %q, want %q", recv.Health(), HealthDisabled)
	}
}

func TestOTLPReceiver_StartStop(t *testing.T) {
	db := setupIngestDB(t)
	resolver := NewSymbolResolver(db)
	ing := NewIngestor(db, resolver, TraceIngestConfig{})
	recv := NewOTLPReceiver(":0", ing)

	ctx := context.Background()

	if err := recv.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if recv.Health() != HealthConnected {
		t.Errorf("after Start, Health() = %q, want %q", recv.Health(), HealthConnected)
	}

	if err := recv.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if recv.Health() != HealthDisabled {
		t.Errorf("after Stop, Health() = %q, want %q", recv.Health(), HealthDisabled)
	}
}

func TestOTLPReceiver_DoubleStartError(t *testing.T) {
	db := setupIngestDB(t)
	resolver := NewSymbolResolver(db)
	ing := NewIngestor(db, resolver, TraceIngestConfig{})
	recv := NewOTLPReceiver(":0", ing)

	ctx := context.Background()

	if err := recv.Start(ctx); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	defer recv.Stop()

	err := recv.Start(ctx)
	if err == nil {
		t.Fatal("expected error on double Start")
	}
}

func TestOTLPReceiver_ExportSpans_NotConnected(t *testing.T) {
	db := setupIngestDB(t)
	resolver := NewSymbolResolver(db)
	ing := NewIngestor(db, resolver, TraceIngestConfig{})
	recv := NewOTLPReceiver(":0", ing)

	ctx := context.Background()
	spans := []TraceSpan{{
		TraceID:     "t1",
		ServiceName: "svc",
		Attributes:  map[string]string{"http.method": "GET", "http.route": "/a"},
	}}

	err := recv.ExportSpans(ctx, spans)
	if err == nil {
		t.Fatal("expected error when not connected")
	}
}

func TestOTLPReceiver_ExportSpans_Connected(t *testing.T) {
	db := setupIngestDB(t)
	resolver := NewSymbolResolver(db)
	ing := NewIngestor(db, resolver, TraceIngestConfig{BatchSize: 100})
	recv := NewOTLPReceiver(":0", ing)

	ctx := context.Background()

	if err := recv.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer recv.Stop()

	spans := []TraceSpan{
		{
			TraceID:     "t1",
			ServiceName: "svc-a",
			PeerService: "svc-b",
			Attributes:  map[string]string{"http.method": "GET", "http.route": "/users"},
		},
		{
			TraceID:     "t2",
			ServiceName: "svc-c",
			PeerService: "svc-d",
			Attributes:  map[string]string{"rpc.service": "Svc", "rpc.method": "Do"},
		},
	}

	if err := recv.ExportSpans(ctx, spans); err != nil {
		t.Fatalf("ExportSpans: %v", err)
	}

	// Spans should be in the pending batch. Flush them.
	if err := ing.FlushBatch(ctx); err != nil {
		t.Fatalf("FlushBatch: %v", err)
	}

	// Verify edges were created.
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM edges`).Scan(&count); err != nil {
		t.Fatalf("count edges: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 edges, got %d", count)
	}
}

func TestOTLPReceiver_ExportSpansAfterStop(t *testing.T) {
	db := setupIngestDB(t)
	resolver := NewSymbolResolver(db)
	ing := NewIngestor(db, resolver, TraceIngestConfig{})
	recv := NewOTLPReceiver(":0", ing)

	ctx := context.Background()

	if err := recv.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := recv.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	err := recv.ExportSpans(ctx, []TraceSpan{{ServiceName: "x"}})
	if err == nil {
		t.Fatal("expected error after Stop")
	}
}

func TestOTLPReceiver_StopWithoutStart(t *testing.T) {
	db := setupIngestDB(t)
	resolver := NewSymbolResolver(db)
	ing := NewIngestor(db, resolver, TraceIngestConfig{})
	recv := NewOTLPReceiver(":0", ing)

	// Stop on a receiver that was never started should not panic.
	if err := recv.Stop(); err != nil {
		t.Fatalf("Stop without Start: %v", err)
	}
	if recv.Health() != HealthDisabled {
		t.Errorf("Health after Stop = %q, want %q", recv.Health(), HealthDisabled)
	}
}

func TestHealthState_Constants(t *testing.T) {
	// Verify the string values of health states.
	tests := []struct {
		state HealthState
		want  string
	}{
		{HealthConnected, "CONNECTED"},
		{HealthReconnecting, "RECONNECTING"},
		{HealthDisconnected, "DISCONNECTED"},
		{HealthDisabled, "DISABLED"},
	}
	for _, tt := range tests {
		if string(tt.state) != tt.want {
			t.Errorf("HealthState %v = %q, want %q", tt.state, string(tt.state), tt.want)
		}
	}
}

func TestOTLPReceiver_DoubleStop(t *testing.T) {
	db := setupIngestDB(t)
	resolver := NewSymbolResolver(db)
	ing := NewIngestor(db, resolver, TraceIngestConfig{})
	recv := NewOTLPReceiver(":0", ing)

	ctx := context.Background()
	if err := recv.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if err := recv.Stop(); err != nil {
		t.Fatalf("first Stop: %v", err)
	}
	// Second stop should not panic or error.
	if err := recv.Stop(); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
}

func TestOTLPReceiver_ExportSpans_EmptyBatch(t *testing.T) {
	db := setupIngestDB(t)
	resolver := NewSymbolResolver(db)
	ing := NewIngestor(db, resolver, TraceIngestConfig{})
	recv := NewOTLPReceiver(":0", ing)

	ctx := context.Background()
	if err := recv.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer recv.Stop()

	// Exporting an empty batch should succeed.
	if err := recv.ExportSpans(ctx, nil); err != nil {
		t.Fatalf("ExportSpans(nil): %v", err)
	}
	if err := recv.ExportSpans(ctx, []TraceSpan{}); err != nil {
		t.Fatalf("ExportSpans(empty): %v", err)
	}
}

// Test that Addr returns the configured address.
func TestOTLPReceiver_Addr(t *testing.T) {
	tests := []string{":4317", "localhost:4317", "0.0.0.0:4318"}
	for _, addr := range tests {
		db := setupIngestDB(t)
		resolver := NewSymbolResolver(db)
		ing := NewIngestor(db, resolver, TraceIngestConfig{})
		recv := NewOTLPReceiver(addr, ing)
		if recv.Addr() != addr {
			t.Errorf("Addr() = %q, want %q", recv.Addr(), addr)
		}
	}
}

// Test the auto-flush behavior when batch reaches BatchSize.
func TestIngestor_AutoFlush(t *testing.T) {
	db := setupIngestDB(t)
	resolver := NewSymbolResolver(db)
	ing := NewIngestor(db, resolver, TraceIngestConfig{BatchSize: 2})

	// Add spans up to BatchSize. The third span triggers auto-flush of the first batch.
	ing.AddToBatch(TraceSpan{
		TraceID:     "t1",
		ServiceName: "svc-a",
		PeerService: "svc-b",
		Attributes:  map[string]string{"http.method": "GET", "http.route": "/a"},
	})

	// At 1 span, no flush yet.
	var count int
	ctx := context.Background()
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM edges`).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 edges before auto-flush, got %d", count)
	}

	// Second span triggers auto-flush (BatchSize=2).
	ing.AddToBatch(TraceSpan{
		TraceID:     "t2",
		ServiceName: "svc-c",
		PeerService: "svc-d",
		Attributes:  map[string]string{"http.method": "POST", "http.route": "/b"},
	})

	// Give auto-flush a moment to complete (it runs synchronously in AddToBatch).
	time.Sleep(50 * time.Millisecond)

	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM edges`).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 edges after auto-flush, got %d", count)
	}
}

// Test FlushBatch on an empty pending batch.
func TestIngestor_FlushBatch_Empty(t *testing.T) {
	db := setupIngestDB(t)
	resolver := NewSymbolResolver(db)
	ing := NewIngestor(db, resolver, TraceIngestConfig{})

	// Flushing an empty batch should succeed without error.
	if err := ing.FlushBatch(context.Background()); err != nil {
		t.Fatalf("FlushBatch(empty): %v", err)
	}
}

// Test edgeTypeFromAttributes with messaging attributes.
func TestEdgeTypeFromAttributes(t *testing.T) {
	tests := []struct {
		name  string
		attrs map[string]string
		want  string
	}{
		{"http", map[string]string{"http.method": "GET"}, "runtime_calls"},
		{"grpc", map[string]string{"rpc.service": "UserService"}, "runtime_rpc"},
		{"messaging producer", map[string]string{"messaging.system": "kafka", "messaging.destination": "orders"}, "runtime_produces"},
		{"messaging consumer", map[string]string{"messaging.system": "rabbitmq"}, "runtime_consumes"},
		{"unknown empty", map[string]string{}, "runtime_calls"},
		{"unknown other", map[string]string{"db.system": "postgresql"}, "runtime_calls"},
		{"nil attrs", nil, "runtime_calls"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := edgeTypeFromAttributes(tt.attrs)
			if got != tt.want {
				t.Errorf("edgeTypeFromAttributes(%v) = %q, want %q", tt.attrs, got, tt.want)
			}
		})
	}
}

// Test IngestHTTPLogs with multiple entries.
func TestIngestHTTPLogs_MultipleEntries(t *testing.T) {
	db := setupIngestDB(t)
	ctx := context.Background()
	resolver := NewSymbolResolver(db)
	ing := NewIngestor(db, resolver, TraceIngestConfig{})

	entries := []HTTPLogEntry{
		{
			Timestamp:   time.Now(),
			Method:      "GET",
			Path:        "/api/users",
			StatusCode:  200,
			ServiceName: "api",
			Duration:    10 * time.Millisecond,
		},
		{
			Timestamp:   time.Now(),
			Method:      "POST",
			Path:        "/api/orders",
			StatusCode:  201,
			ServiceName: "api",
			Duration:    20 * time.Millisecond,
		},
		{
			Timestamp:   time.Now(),
			Method:      "DELETE",
			Path:        "/api/items/123",
			StatusCode:  204,
			ServiceName: "api",
			Duration:    5 * time.Millisecond,
		},
	}

	result, err := ing.IngestHTTPLogs(ctx, entries)
	if err != nil {
		t.Fatalf("IngestHTTPLogs: %v", err)
	}
	if result.Created != 3 {
		t.Errorf("Created: got %d, want 3", result.Created)
	}
}

// Test IngestHTTPLogs with empty entries.
func TestIngestHTTPLogs_Empty(t *testing.T) {
	db := setupIngestDB(t)
	ctx := context.Background()
	resolver := NewSymbolResolver(db)
	ing := NewIngestor(db, resolver, TraceIngestConfig{})

	result, err := ing.IngestHTTPLogs(ctx, nil)
	if err != nil {
		t.Fatalf("IngestHTTPLogs(nil): %v", err)
	}
	if result.Created != 0 || result.Updated != 0 {
		t.Errorf("expected zero result for empty input, got Created=%d Updated=%d", result.Created, result.Updated)
	}
}

// Test DecayConfidence when no edges match.
func TestDecayConfidence_NoMatch(t *testing.T) {
	db := setupIngestDB(t)
	ctx := context.Background()
	resolver := NewSymbolResolver(db)
	ing := NewIngestor(db, resolver, TraceIngestConfig{})

	// No edges in the database.
	updated, err := ing.DecayConfidence(ctx)
	if err != nil {
		t.Fatalf("DecayConfidence: %v", err)
	}
	if updated != 0 {
		t.Errorf("expected 0 updated, got %d", updated)
	}
}

// Test RuntimeEdgeStats with empty database.
func TestRuntimeEdgeStats_Empty(t *testing.T) {
	db := setupIngestDB(t)
	ctx := context.Background()
	resolver := NewSymbolResolver(db)
	ing := NewIngestor(db, resolver, TraceIngestConfig{})

	stats, err := ing.RuntimeEdgeStats(ctx, [32]byte{})
	if err != nil {
		t.Fatalf("RuntimeEdgeStats: %v", err)
	}
	if stats.TotalEdges != 0 {
		t.Errorf("TotalEdges: got %d, want 0", stats.TotalEdges)
	}
	if stats.ActiveCount != 0 {
		t.Errorf("ActiveCount: got %d, want 0", stats.ActiveCount)
	}
	if stats.StaleCount != 0 {
		t.Errorf("StaleCount: got %d, want 0", stats.StaleCount)
	}
	if stats.GCEligible != 0 {
		t.Errorf("GCEligible: got %d, want 0", stats.GCEligible)
	}
}

// Test RuntimeEdgeStats includes GC-eligible edges.
func TestRuntimeEdgeStats_GCEligible(t *testing.T) {
	db := setupIngestDB(t)
	ctx := context.Background()
	resolver := NewSymbolResolver(db)
	ing := NewIngestor(db, resolver, TraceIngestConfig{})

	now := time.Now().Unix()

	// Insert a GC-eligible edge (observed 100 days ago).
	_, err := db.ExecContext(ctx,
		`INSERT INTO edges (edge_hash, source_hash, target_hash, edge_type, confidence, provenance, callsite_line, callsite_col, callsite_file, observation_count, last_observed)
		 VALUES (?, ?, ?, 'runtime_calls', 0.5, 'otel_trace', 0, 0, '', 1, ?)`,
		[]byte("gc-edge-hash-padded-to-32-bytes!"),
		[]byte("gc-src-hash-padded-to-32-bytes!!"),
		[]byte("gc-tgt-hash-padded-to-32-bytes!!"),
		now-100*86400,
	)
	if err != nil {
		t.Fatalf("insert gc edge: %v", err)
	}

	stats, err := ing.RuntimeEdgeStats(ctx, [32]byte{})
	if err != nil {
		t.Fatalf("RuntimeEdgeStats: %v", err)
	}
	if stats.TotalEdges != 1 {
		t.Errorf("TotalEdges: got %d, want 1", stats.TotalEdges)
	}
	if stats.GCEligible != 1 {
		t.Errorf("GCEligible: got %d, want 1", stats.GCEligible)
	}
}

// Test ResolveSpan with unknown attributes (fallback path).
func TestResolveSpan_Unknown(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	span := TraceSpan{
		ServiceName:   "my-service",
		OperationName: "custom-operation",
		Attributes:    map[string]string{"db.system": "postgresql"},
	}

	resolver := NewSymbolResolver(db)
	sourceHash, _, _, err := resolver.ResolveSpan(ctx, span)
	if err != nil {
		t.Fatalf("ResolveSpan: %v", err)
	}

	// Source should be computed from the service name.
	if sourceHash.IsZero() {
		t.Error("expected non-zero source hash")
	}
}

// Test ResolveSpan without PeerService (target uses own service name).
func TestResolveSpan_NoPeerService(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	span := TraceSpan{
		ServiceName:   "my-service",
		OperationName: "internal-op",
		PeerService:   "", // no peer service
		Attributes: map[string]string{
			"http.method": "GET",
			"http.route":  "/internal",
		},
	}

	resolver := NewSymbolResolver(db)
	_, _, confidence, err := resolver.ResolveSpan(ctx, span)
	if err != nil {
		t.Fatalf("ResolveSpan: %v", err)
	}

	// Unresolved target should give 0.3 confidence.
	if confidence != 0.3 {
		t.Errorf("confidence: got %f, want 0.3", confidence)
	}
}

// Test IngestResult zero value.
func TestIngestResult_ZeroValue(t *testing.T) {
	var r IngestResult
	if r.Created != 0 || r.Updated != 0 {
		t.Errorf("zero IngestResult should have Created=0 Updated=0")
	}
}

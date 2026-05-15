package trace

import (
	"context"
	"database/sql"
	"testing"

	"github.com/blackwell-systems/knowing/internal/types"

	_ "modernc.org/sqlite"
)

// setupTestDB creates an in-memory SQLite database with the route_symbols table.
func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	_, err = db.Exec(`
		CREATE TABLE route_symbols (
			service_name  TEXT NOT NULL,
			route_pattern TEXT NOT NULL,
			node_hash     BLOB NOT NULL,
			mapping_type  TEXT NOT NULL,
			created_at    INTEGER NOT NULL,
			PRIMARY KEY (service_name, route_pattern, mapping_type)
		)`)
	if err != nil {
		t.Fatalf("create table: %v", err)
	}
	return db
}

func TestResolve_ExactMatch(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	// Pre-compute the expected node hash.
	expectedHash := types.ComputeNodeHash("order-service", "internal/api", types.EmptyHash, "CreateOrder", "function")

	// Insert a route symbol row.
	_, err := db.Exec(
		`INSERT INTO route_symbols (service_name, route_pattern, node_hash, mapping_type, created_at) VALUES (?, ?, ?, ?, ?)`,
		"order-service", "POST /api/orders", expectedHash[:], "http_route", 1000,
	)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	resolver := NewSymbolResolver(db)
	hash, confidence, err := resolver.Resolve(ctx, "order-service", "POST /api/orders", "http_route")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if hash != expectedHash {
		t.Errorf("hash mismatch: got %x, want %x", hash, expectedHash)
	}
	if confidence != 1.0 {
		t.Errorf("confidence: got %f, want 1.0", confidence)
	}
}

func TestResolve_Unresolved(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	resolver := NewSymbolResolver(db)
	hash, confidence, err := resolver.Resolve(ctx, "user-service", "GET /api/users", "http_route")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	// Should get a synthetic unresolved hash.
	expectedHash := types.ComputeNodeHash("user-service", "UNRESOLVED", types.EmptyHash, "GET /api/users", "runtime_endpoint")
	if hash != expectedHash {
		t.Errorf("hash mismatch: got %x, want %x", hash, expectedHash)
	}
	if confidence != 0.3 {
		t.Errorf("confidence: got %f, want 0.3", confidence)
	}
}

func TestResolveSpan_HTTP(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	// Insert a route symbol for the target.
	targetHash := types.ComputeNodeHash("user-service", "internal/handler", types.EmptyHash, "CreateUser", "function")
	_, err := db.Exec(
		`INSERT INTO route_symbols (service_name, route_pattern, node_hash, mapping_type, created_at) VALUES (?, ?, ?, ?, ?)`,
		"user-service", "POST /api/users", targetHash[:], "http_route", 1000,
	)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	span := TraceSpan{
		ServiceName:   "gateway",
		OperationName: "HTTP POST",
		PeerService:   "user-service",
		Attributes: map[string]string{
			"http.method": "POST",
			"http.route":  "/api/users",
		},
	}

	resolver := NewSymbolResolver(db)
	sourceHash, gotTarget, confidence, err := resolver.ResolveSpan(ctx, span)
	if err != nil {
		t.Fatalf("ResolveSpan: %v", err)
	}

	expectedSource := types.ComputeNodeHash("gateway", "", types.EmptyHash, "gateway", "service")
	if sourceHash != expectedSource {
		t.Errorf("source hash mismatch: got %x, want %x", sourceHash, expectedSource)
	}
	if gotTarget != targetHash {
		t.Errorf("target hash mismatch: got %x, want %x", gotTarget, targetHash)
	}
	if confidence != 1.0 {
		t.Errorf("confidence: got %f, want 1.0", confidence)
	}
}

func TestResolveSpan_GRPC(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	// Insert a route symbol for the gRPC target.
	targetHash := types.ComputeNodeHash("user-service", "internal/grpc", types.EmptyHash, "GetUser", "function")
	_, err := db.Exec(
		`INSERT INTO route_symbols (service_name, route_pattern, node_hash, mapping_type, created_at) VALUES (?, ?, ?, ?, ?)`,
		"user-service", "UserService.GetUser", targetHash[:], "grpc_method", 1000,
	)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	span := TraceSpan{
		ServiceName:   "api-gateway",
		OperationName: "gRPC call",
		PeerService:   "user-service",
		Attributes: map[string]string{
			"rpc.service": "UserService",
			"rpc.method":  "GetUser",
		},
	}

	resolver := NewSymbolResolver(db)
	sourceHash, gotTarget, confidence, err := resolver.ResolveSpan(ctx, span)
	if err != nil {
		t.Fatalf("ResolveSpan: %v", err)
	}

	expectedSource := types.ComputeNodeHash("api-gateway", "", types.EmptyHash, "api-gateway", "service")
	if sourceHash != expectedSource {
		t.Errorf("source hash mismatch: got %x, want %x", sourceHash, expectedSource)
	}
	if gotTarget != targetHash {
		t.Errorf("target hash mismatch: got %x, want %x", gotTarget, targetHash)
	}
	if confidence != 1.0 {
		t.Errorf("confidence: got %f, want 1.0", confidence)
	}
}

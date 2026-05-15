package trace

import (
	"context"
	"database/sql"

	"github.com/blackwell-systems/knowing/internal/types"
)

// SymbolResolver resolves runtime identifiers (HTTP routes, gRPC methods,
// queue topics) to graph node hashes using the route_symbols table.
type SymbolResolver struct {
	db *sql.DB
}

// NewSymbolResolver creates a new SymbolResolver backed by the given database.
func NewSymbolResolver(db *sql.DB) *SymbolResolver {
	return &SymbolResolver{db: db}
}

// Resolve looks up a route symbol by exact match on (service_name, route_pattern,
// mapping_type). If found, returns the node hash with confidence 1.0. If not
// found, returns a synthetic unresolved node hash with confidence 0.3.
func (r *SymbolResolver) Resolve(ctx context.Context, serviceName, routePattern, mappingType string) (types.Hash, float64, error) {
	var hashBytes []byte
	err := r.db.QueryRowContext(ctx,
		`SELECT node_hash FROM route_symbols WHERE service_name = ? AND route_pattern = ? AND mapping_type = ?`,
		serviceName, routePattern, mappingType,
	).Scan(&hashBytes)

	if err == nil {
		var h types.Hash
		copy(h[:], hashBytes)
		return h, 1.0, nil
	}

	if err != sql.ErrNoRows {
		return types.EmptyHash, 0, err
	}

	// No match: create a synthetic unresolved node hash.
	syntheticHash := types.ComputeNodeHash(serviceName, "UNRESOLVED", types.EmptyHash, routePattern, "runtime_endpoint")
	return syntheticHash, 0.3, nil
}

// ResolveSpan extracts runtime identifiers from a TraceSpan's attributes and
// resolves them to source and target node hashes.
func (r *SymbolResolver) ResolveSpan(ctx context.Context, span TraceSpan) (sourceHash, targetHash types.Hash, confidence float64, err error) {
	// Source: the service that emitted this span.
	sourceHash = types.ComputeNodeHash(span.ServiceName, "", types.EmptyHash, span.ServiceName, "service")

	// Determine the target mapping type and route pattern from span attributes.
	var mappingType, routePattern string

	httpMethod := span.Attributes["http.method"]
	httpRoute := span.Attributes["http.route"]
	rpcService := span.Attributes["rpc.service"]
	rpcMethod := span.Attributes["rpc.method"]

	switch {
	case httpMethod != "" && httpRoute != "":
		mappingType = "http_route"
		routePattern = httpMethod + " " + httpRoute
	case rpcService != "" && rpcMethod != "":
		mappingType = "grpc_method"
		routePattern = rpcService + "." + rpcMethod
	default:
		mappingType = "unknown"
		routePattern = span.OperationName
	}

	// Resolve target against peer service if available, otherwise against
	// the span's own service.
	targetService := span.ServiceName
	if span.PeerService != "" {
		targetService = span.PeerService
	}

	targetHash, confidence, err = r.Resolve(ctx, targetService, routePattern, mappingType)
	return sourceHash, targetHash, confidence, err
}

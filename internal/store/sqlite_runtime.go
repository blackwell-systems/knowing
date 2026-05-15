package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/blackwell-systems/knowing/internal/types"
)

// RouteSymbolRow represents a row in the route_symbols table, mapping
// a service route pattern to a graph node hash. This is a local struct
// to avoid importing the trace package from the store layer.
type RouteSymbolRow struct {
	ServiceName  string
	RoutePattern string
	MappingType  string
	NodeHash     types.Hash
	CreatedAt    int64
}

// PutRouteSymbol upserts a route symbol mapping into the route_symbols table.
func (s *SQLiteStore) PutRouteSymbol(ctx context.Context, serviceName, routePattern string, nodeHash types.Hash, mappingType string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO route_symbols (service_name, route_pattern, node_hash, mapping_type, created_at)
		 VALUES (?, ?, ?, ?, ?)`,
		serviceName, routePattern, nodeHash[:], mappingType, time.Now().Unix(),
	)
	return err
}

// GetRouteSymbol retrieves a route symbol mapping by its composite key.
// Returns (nil, nil) if no matching row exists.
func (s *SQLiteStore) GetRouteSymbol(ctx context.Context, serviceName, routePattern, mappingType string) (*RouteSymbolRow, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT service_name, route_pattern, node_hash, mapping_type, created_at
		 FROM route_symbols WHERE service_name = ? AND route_pattern = ? AND mapping_type = ?`,
		serviceName, routePattern, mappingType,
	)

	var r RouteSymbolRow
	var nodeHash []byte
	if err := row.Scan(&r.ServiceName, &r.RoutePattern, &nodeHash, &r.MappingType, &r.CreatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	copy(r.NodeHash[:], nodeHash)
	return &r, nil
}

// UpdateObservation updates the runtime observation fields on an edge.
func (s *SQLiteStore) UpdateObservation(ctx context.Context, edgeHash types.Hash, count int, lastObserved int64, confidence float64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE edges SET observation_count = ?, last_observed = ?, confidence = ? WHERE edge_hash = ?`,
		count, lastObserved, confidence, edgeHash[:],
	)
	return err
}

// RuntimeEdgesByProvenance returns all edges whose provenance starts with the
// given prefix. The returned edges include the observation_count and
// last_observed columns populated on the Edge struct.
func (s *SQLiteStore) RuntimeEdgesByProvenance(ctx context.Context, provenancePrefix string) ([]types.Edge, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT edge_hash, source_hash, target_hash, edge_type, confidence, provenance,
		        callsite_line, callsite_col, callsite_file, observation_count, last_observed
		 FROM edges WHERE provenance LIKE ?`,
		provenancePrefix+"%",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEdgesRuntime(rows)
}

// DecayRuntimeConfidence reduces the confidence of stale runtime-derived edges.
// An edge is considered stale if it has provenance starting with "otel_",
// last_observed is older than staleDays ago, and its confidence is higher than
// newConfidence. Returns the number of rows affected.
func (s *SQLiteStore) DecayRuntimeConfidence(ctx context.Context, staleDays int, newConfidence float64) (int, error) {
	threshold := time.Now().Unix() - int64(staleDays)*86400
	result, err := s.db.ExecContext(ctx,
		`UPDATE edges SET confidence = ?
		 WHERE provenance LIKE 'otel_%'
		 AND last_observed < ?
		 AND last_observed > 0
		 AND confidence > ?`,
		newConfidence, threshold, newConfidence,
	)
	if err != nil {
		return 0, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

// scanEdgeRuntime scans a single row with all 11 edge columns including
// observation_count and last_observed.
func scanEdgeRuntime(row scannable) (*types.Edge, error) {
	var e types.Edge
	var edgeHash, sourceHash, targetHash []byte
	if err := row.Scan(
		&edgeHash, &sourceHash, &targetHash,
		&e.EdgeType, &e.Confidence, &e.Provenance,
		&e.CallSiteLine, &e.CallSiteCol, &e.CallSiteFile,
		&e.ObservationCount, &e.LastObserved,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	copy(e.EdgeHash[:], edgeHash)
	copy(e.SourceHash[:], sourceHash)
	copy(e.TargetHash[:], targetHash)
	return &e, nil
}

// scanEdgesRuntime scans multiple rows with all 11 edge columns including
// observation_count and last_observed.
func scanEdgesRuntime(rows *sql.Rows) ([]types.Edge, error) {
	var edges []types.Edge
	for rows.Next() {
		var e types.Edge
		var edgeHash, sourceHash, targetHash []byte
		if err := rows.Scan(
			&edgeHash, &sourceHash, &targetHash,
			&e.EdgeType, &e.Confidence, &e.Provenance,
			&e.CallSiteLine, &e.CallSiteCol, &e.CallSiteFile,
			&e.ObservationCount, &e.LastObserved,
		); err != nil {
			return nil, err
		}
		copy(e.EdgeHash[:], edgeHash)
		copy(e.SourceHash[:], sourceHash)
		copy(e.TargetHash[:], targetHash)
		edges = append(edges, e)
	}
	return edges, rows.Err()
}

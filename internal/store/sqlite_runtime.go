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

// RuntimeStatsRow contains aggregate statistics about runtime-derived edges.
type RuntimeStatsRow struct {
	TotalEdges  int
	ActiveEdges int            // observed in last 7 days
	StaleEdges  int            // not observed in 30+ days
	GCEligible  int            // not observed in 90+ days
	ByEdgeType  map[string]int // counts keyed by edge_type
}

// RuntimeEdgesByService returns runtime edges filtered by service name and
// optional route pattern. Only edges with provenance starting with "otel_" are
// returned. If serviceName is empty, all runtime edges are returned (up to
// limit). If routePattern is non-empty, it is used as a LIKE filter on the
// route_symbols.route_pattern column.
func (s *SQLiteStore) RuntimeEdgesByService(ctx context.Context, serviceName string, routePattern string, limit int) ([]types.Edge, error) {
	query := `SELECT e.edge_hash, e.source_hash, e.target_hash, e.edge_type,
	                  e.confidence, e.provenance, e.callsite_line, e.callsite_col,
	                  e.callsite_file, e.observation_count, e.last_observed
	           FROM edges e`

	var args []interface{}

	if serviceName != "" {
		query += ` JOIN route_symbols rs ON rs.node_hash = e.target_hash`
	}

	query += ` WHERE e.provenance LIKE 'otel_%'`

	if serviceName != "" {
		query += ` AND rs.service_name = ?`
		args = append(args, serviceName)
	}
	if routePattern != "" {
		if serviceName == "" {
			query += ` AND EXISTS (SELECT 1 FROM route_symbols rs WHERE rs.node_hash = e.target_hash AND rs.route_pattern LIKE ?)`
		} else {
			query += ` AND rs.route_pattern LIKE ?`
		}
		args = append(args, routePattern)
	}

	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEdgesRuntime(rows)
}

// DeadRoutes returns route symbols that have no runtime observations in the
// last staleDays days (or have never been observed at all).
func (s *SQLiteStore) DeadRoutes(ctx context.Context, staleDays int) ([]RouteSymbolRow, error) {
	threshold := time.Now().Unix() - int64(staleDays)*86400

	rows, err := s.db.QueryContext(ctx,
		`SELECT rs.service_name, rs.route_pattern, rs.node_hash, rs.mapping_type, rs.created_at
		 FROM route_symbols rs
		 LEFT JOIN edges e ON rs.node_hash = e.target_hash AND e.provenance LIKE 'otel_%'
		 WHERE e.last_observed IS NULL OR e.last_observed < ?
		 GROUP BY rs.service_name, rs.route_pattern, rs.mapping_type`,
		threshold,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []RouteSymbolRow
	for rows.Next() {
		var r RouteSymbolRow
		var nodeHash []byte
		if err := rows.Scan(&r.ServiceName, &r.RoutePattern, &nodeHash, &r.MappingType, &r.CreatedAt); err != nil {
			return nil, err
		}
		copy(r.NodeHash[:], nodeHash)
		result = append(result, r)
	}
	return result, rows.Err()
}

// RuntimeEdgeStatsAggregate returns aggregate statistics about runtime-derived
// edges (those with provenance starting with "otel_").
func (s *SQLiteStore) RuntimeEdgeStatsAggregate(ctx context.Context) (*RuntimeStatsRow, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT edge_type, observation_count, last_observed
		 FROM edges WHERE provenance LIKE 'otel_%'`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	now := time.Now().Unix()
	sevenDaysAgo := now - 7*86400
	thirtyDaysAgo := now - 30*86400
	ninetyDaysAgo := now - 90*86400

	stats := &RuntimeStatsRow{
		ByEdgeType: make(map[string]int),
	}

	for rows.Next() {
		var edgeType string
		var obsCount int
		var lastObs int64
		if err := rows.Scan(&edgeType, &obsCount, &lastObs); err != nil {
			return nil, err
		}

		stats.TotalEdges++
		stats.ByEdgeType[edgeType]++

		if lastObs >= sevenDaysAgo {
			stats.ActiveEdges++
		}
		if lastObs > 0 && lastObs < thirtyDaysAgo {
			stats.StaleEdges++
		}
		if lastObs > 0 && lastObs < ninetyDaysAgo {
			stats.GCEligible++
		}
	}
	return stats, rows.Err()
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

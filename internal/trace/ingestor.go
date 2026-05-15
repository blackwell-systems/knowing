package trace

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/blackwell-systems/knowing/internal/types"
)

// Ingestor processes trace spans and HTTP log entries into graph edges.
// It implements the TraceIngestor interface.
type Ingestor struct {
	db       *sql.DB
	resolver *SymbolResolver
	config   TraceIngestConfig
	mu       sync.Mutex // protects pending batch
	pending  []TraceSpan
}

// Compile-time check that Ingestor implements TraceIngestor.
var _ TraceIngestor = (*Ingestor)(nil)

// NewIngestor creates a new Ingestor backed by the given database, symbol
// resolver, and configuration.
func NewIngestor(db *sql.DB, resolver *SymbolResolver, config TraceIngestConfig) *Ingestor {
	return &Ingestor{
		db:       db,
		resolver: resolver,
		config:   config,
	}
}

// IngestSpans processes a batch of trace spans, creating or updating graph
// edges for each span. For each span it resolves source/target node hashes,
// determines edge type from span attributes, and upserts the edge.
func (ing *Ingestor) IngestSpans(ctx context.Context, spans []TraceSpan) (IngestResult, error) {
	var result IngestResult

	for _, span := range spans {
		sourceHash, targetHash, confidence, err := ing.resolver.ResolveSpan(ctx, span)
		if err != nil {
			return result, fmt.Errorf("resolve span %q: %w", span.OperationName, err)
		}

		edgeType := edgeTypeFromAttributes(span.Attributes)
		provenance := BuildProvenance([]string{span.TraceID})

		// Edge hash uses "otel_trace" as provenance so the same relationship
		// always maps to the same hash regardless of sampled trace IDs.
		edgeHash := types.ComputeEdgeHash(sourceHash, targetHash, edgeType, "otel_trace")

		// Check if edge already exists.
		var existingCount int
		var existingLastObserved int64
		err = ing.db.QueryRowContext(ctx,
			`SELECT observation_count, last_observed FROM edges WHERE edge_hash = ?`,
			edgeHash[:],
		).Scan(&existingCount, &existingLastObserved)

		now := time.Now().Unix()

		if err == nil {
			// Edge exists: update observation count, last_observed, confidence.
			newCount := existingCount + 1
			newConfidence := ComputeConfidence(newCount, 0) // just observed, so 0 days
			_, err = ing.db.ExecContext(ctx,
				`UPDATE edges SET observation_count = ?, last_observed = ?, confidence = ? WHERE edge_hash = ?`,
				newCount, now, newConfidence, edgeHash[:],
			)
			if err != nil {
				return result, fmt.Errorf("update edge: %w", err)
			}
			result.Updated++
		} else if err == sql.ErrNoRows {
			// New edge: insert with full provenance and record an edge event.
			newConfidence := ComputeConfidence(1, 0)
			if confidence < newConfidence {
				newConfidence = confidence
			}
			_, err = ing.db.ExecContext(ctx,
				`INSERT INTO edges (edge_hash, source_hash, target_hash, edge_type, confidence, provenance, callsite_line, callsite_col, callsite_file, observation_count, last_observed)
				 VALUES (?, ?, ?, ?, ?, ?, 0, 0, '', 1, ?)`,
				edgeHash[:], sourceHash[:], targetHash[:], edgeType, newConfidence, provenance, now,
			)
			if err != nil {
				return result, fmt.Errorf("insert edge: %w", err)
			}

			// Record an "added" edge event.
			_, err = ing.db.ExecContext(ctx,
				`INSERT INTO edge_events (edge_hash, event_type, snapshot_hash, source_commit, indexer_ver, timestamp)
				 VALUES (?, 'added', ?, '', 'otel_ingestor', ?)`,
				edgeHash[:], types.EmptyHash[:], now,
			)
			if err != nil {
				return result, fmt.Errorf("record edge event: %w", err)
			}

			result.Created++
		} else {
			return result, fmt.Errorf("check edge existence: %w", err)
		}
	}

	return result, nil
}

// IngestHTTPLogs converts HTTP log entries to trace spans and delegates to
// IngestSpans.
func (ing *Ingestor) IngestHTTPLogs(ctx context.Context, entries []HTTPLogEntry) (IngestResult, error) {
	spans := make([]TraceSpan, len(entries))
	for i, entry := range entries {
		spans[i] = TraceSpan{
			ServiceName:   entry.ServiceName,
			OperationName: entry.Method + " " + entry.Path,
			Attributes: map[string]string{
				"http.method": entry.Method,
				"http.route":  entry.Path,
			},
			StartTime: entry.Timestamp,
			Duration:  entry.Duration,
		}
	}
	return ing.IngestSpans(ctx, spans)
}

// RuntimeEdgeStats returns aggregated statistics for runtime-derived edges.
// The snapshot parameter is accepted for interface compatibility but is not
// currently used for filtering.
func (ing *Ingestor) RuntimeEdgeStats(ctx context.Context, _ types.Hash) (*RuntimeStats, error) {
	stats := &RuntimeStats{
		BySourceType: make(map[string]int),
	}

	rows, err := ing.db.QueryContext(ctx,
		`SELECT edge_type, observation_count, last_observed FROM edges WHERE provenance LIKE 'otel_%'`,
	)
	if err != nil {
		return nil, fmt.Errorf("query runtime edges: %w", err)
	}
	defer rows.Close()

	now := time.Now().Unix()

	for rows.Next() {
		var edgeType string
		var obsCount int
		var lastObs int64
		if err := rows.Scan(&edgeType, &obsCount, &lastObs); err != nil {
			return nil, fmt.Errorf("scan edge: %w", err)
		}

		stats.TotalEdges++
		stats.BySourceType[edgeType]++

		daysSince := int((now - lastObs) / 86400)
		switch {
		case daysSince <= 7:
			stats.ActiveCount++
		case daysSince > 90:
			stats.GCEligible++
		case daysSince > 30:
			stats.StaleCount++
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return stats, nil
}

// DecayConfidence reduces confidence on stale runtime edges. Edges derived
// from OTel traces that have not been observed in 30+ days get their
// confidence set to 0.2. Returns the count of updated edges.
func (ing *Ingestor) DecayConfidence(ctx context.Context) (int, error) {
	threshold := time.Now().Unix() - 30*86400
	result, err := ing.db.ExecContext(ctx,
		`UPDATE edges SET confidence = 0.2
		 WHERE provenance LIKE 'otel_%'
		 AND last_observed < ?
		 AND last_observed > 0
		 AND confidence > 0.2`,
		threshold,
	)
	if err != nil {
		return 0, fmt.Errorf("decay confidence: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

// AddToBatch appends a span to the pending batch. If the batch reaches the
// configured BatchSize, FlushBatch is triggered automatically.
func (ing *Ingestor) AddToBatch(span TraceSpan) {
	ing.mu.Lock()
	ing.pending = append(ing.pending, span)
	shouldFlush := ing.config.BatchSize > 0 && len(ing.pending) >= ing.config.BatchSize
	ing.mu.Unlock()

	if shouldFlush {
		// Best-effort flush; errors are silently dropped for batch accumulation.
		_ = ing.FlushBatch(context.Background())
	}
}

// FlushBatch ingests all pending spans and clears the batch.
func (ing *Ingestor) FlushBatch(ctx context.Context) error {
	ing.mu.Lock()
	batch := ing.pending
	ing.pending = nil
	ing.mu.Unlock()

	if len(batch) == 0 {
		return nil
	}

	_, err := ing.IngestSpans(ctx, batch)
	return err
}

// edgeTypeFromAttributes determines the edge type from span attributes.
func edgeTypeFromAttributes(attrs map[string]string) string {
	if _, ok := attrs["http.method"]; ok {
		return "runtime_calls"
	}
	if _, ok := attrs["rpc.service"]; ok {
		return "runtime_rpc"
	}
	if system := attrs["messaging.system"]; system != "" {
		if _, ok := attrs["messaging.destination"]; ok {
			return "runtime_produces"
		}
		return "runtime_consumes"
	}
	return "runtime_calls"
}

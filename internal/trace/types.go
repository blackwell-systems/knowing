package trace

import (
	"context"
	"time"

	"github.com/blackwell-systems/knowing/internal/types"
)

// TraceSpan is a normalized representation of a single span from any
// tracing system.
type TraceSpan struct {
	TraceID       string
	SpanID        string
	ParentSpanID  string
	ServiceName   string
	OperationName string
	PeerService   string
	Attributes    map[string]string
	StartTime     time.Time
	Duration      time.Duration
}

// HTTPLogEntry represents a single HTTP access log line.
type HTTPLogEntry struct {
	Timestamp   time.Time
	Method      string
	Path        string
	StatusCode  int
	ServiceName string
	ClientIP    string
	Duration    time.Duration
}

// RuntimeStats holds aggregated statistics for runtime edges.
type RuntimeStats struct {
	TotalEdges   int
	BySourceType map[string]int
	ActiveCount  int // edges with observations in last 7 days
	StaleCount   int // edges with no observations in last 30 days
	GCEligible   int // edges with no observations in last 90 days
}

// IngestResult reports the outcome of a batch ingestion.
type IngestResult struct {
	Created int
	Updated int
}

// ConfidenceFromCount computes confidence score from observation count
// within the last 7 days, per the design doc scoring table.
func ConfidenceFromCount(count int) float64 {
	switch {
	case count > 1000:
		return 0.95
	case count >= 100:
		return 0.85
	case count >= 10:
		return 0.7
	case count >= 1:
		return 0.5
	default:
		return 0.2
	}
}

// RouteMapping represents a mapping from a runtime identifier (HTTP route,
// gRPC method, queue topic) to a graph node.
type RouteMapping struct {
	ServiceName  string
	RoutePattern string
	NodeHash     types.Hash
	MappingType  string // "http_route", "grpc_method", "queue_topic"
}

// HealthState represents the connection health of the trace ingestor.
type HealthState string

const (
	HealthConnected    HealthState = "CONNECTED"
	HealthReconnecting HealthState = "RECONNECTING"
	HealthDisconnected HealthState = "DISCONNECTED"
	HealthDisabled     HealthState = "DISABLED"
)

// TraceIngestConfig holds configuration for the trace ingestion pipeline.
type TraceIngestConfig struct {
	Enabled                 bool
	OTLPEndpoint            string
	BatchSize               int
	BatchInterval           time.Duration
	ConfidenceDecayInterval time.Duration
	GCThresholdDays         int
	ServiceMap              map[string]string // OTel name -> knowing repo name
}

// TraceIngestor defines the interface for converting raw observability data
// into graph edges. Implemented by the ingestor in internal/trace/ingestor.go.
type TraceIngestor interface {
	IngestSpans(ctx context.Context, spans []TraceSpan) (IngestResult, error)
	IngestHTTPLogs(ctx context.Context, entries []HTTPLogEntry) (IngestResult, error)
	RuntimeEdgeStats(ctx context.Context, snapshot types.Hash) (*RuntimeStats, error)
	DecayConfidence(ctx context.Context) (updated int, err error)
}

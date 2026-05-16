package trace

import (
	"context"
	"encoding/hex"
	"fmt"
	"net"
	"sync"
	"time"

	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/grpc"
)

// OTLPReceiver is a gRPC server that implements the OTLP trace receiver
// protocol. It receives ExportTraceServiceRequest messages, normalizes the
// spans to TraceSpan, and passes them to the TraceIngestor via batching.
type OTLPReceiver struct {
	coltracepb.UnimplementedTraceServiceServer
	ingestor   TraceIngestor
	addr       string
	listenAddr string // actual address after binding (useful for port :0)
	health     HealthState
	mu         sync.RWMutex
	stopFn     func() // set by Start, called by Stop
	grpcServer *grpc.Server
}

// NewOTLPReceiver creates a new OTLPReceiver that will listen on addr and
// forward spans to the given ingestor.
func NewOTLPReceiver(addr string, ingestor TraceIngestor) *OTLPReceiver {
	return &OTLPReceiver{
		ingestor: ingestor,
		addr:     addr,
		health:   HealthDisabled,
	}
}

// Start begins accepting OTLP trace data. It creates a gRPC server,
// registers the OTLP trace service, and listens on the configured address.
func (r *OTLPReceiver) Start(_ context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.health == HealthConnected {
		return fmt.Errorf("otlp receiver already started")
	}

	lis, err := net.Listen("tcp", r.addr)
	if err != nil {
		return fmt.Errorf("otlp receiver listen %s: %w", r.addr, err)
	}

	r.listenAddr = lis.Addr().String()
	srv := grpc.NewServer()
	r.grpcServer = srv
	coltracepb.RegisterTraceServiceServer(srv, r)

	go func() {
		// Serve blocks until GracefulStop or Stop is called.
		_ = srv.Serve(lis)
	}()

	r.health = HealthConnected
	r.stopFn = func() {
		r.grpcServer.GracefulStop()
	}
	return nil
}

// Stop gracefully shuts down the OTLP receiver.
func (r *OTLPReceiver) Stop() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.stopFn != nil {
		r.stopFn()
		r.stopFn = nil
	}
	r.grpcServer = nil
	r.health = HealthDisabled
	return nil
}

// Health returns the current health state of the receiver.
func (r *OTLPReceiver) Health() HealthState {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.health
}

// Export implements coltracepb.TraceServiceServer. It accepts real OTLP
// ExportTraceServiceRequest messages, converts ResourceSpans to TraceSpan
// structs, and passes them to the ingestor via AddToBatch.
func (r *OTLPReceiver) Export(_ context.Context, req *coltracepb.ExportTraceServiceRequest) (*coltracepb.ExportTraceServiceResponse, error) {
	r.mu.RLock()
	health := r.health
	r.mu.RUnlock()

	if health != HealthConnected {
		return nil, fmt.Errorf("otlp receiver not connected (state: %s)", health)
	}

	for _, rs := range req.GetResourceSpans() {
		serviceName := extractServiceName(rs.GetResource())

		for _, ss := range rs.GetScopeSpans() {
			for _, span := range ss.GetSpans() {
				ts := convertOTLPSpan(span, serviceName)
				r.ingestor.(*Ingestor).AddToBatch(ts)
			}
		}
	}

	return &coltracepb.ExportTraceServiceResponse{}, nil
}

// ExportSpans processes a batch of pre-converted TraceSpan values. Kept for
// backward compatibility with existing tests.
func (r *OTLPReceiver) ExportSpans(ctx context.Context, spans []TraceSpan) error {
	r.mu.RLock()
	health := r.health
	r.mu.RUnlock()

	if health != HealthConnected {
		return fmt.Errorf("otlp receiver not connected (state: %s)", health)
	}

	for _, span := range spans {
		r.ingestor.(*Ingestor).AddToBatch(span)
	}
	return nil
}

// Addr returns the configured listen address.
func (r *OTLPReceiver) Addr() string {
	return r.addr
}

// ListenAddr returns the actual address the server is listening on.
// This is useful when Start was called with port :0 (random port).
func (r *OTLPReceiver) ListenAddr() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.listenAddr
}

// extractServiceName extracts the "service.name" attribute from an OTLP Resource.
func extractServiceName(res *resourcepb.Resource) string {
	if res == nil {
		return ""
	}
	for _, attr := range res.GetAttributes() {
		if attr.GetKey() == "service.name" {
			return attr.GetValue().GetStringValue()
		}
	}
	return ""
}

// convertOTLPSpan maps an OTLP Span proto to the internal TraceSpan.
func convertOTLPSpan(span *tracepb.Span, serviceName string) TraceSpan {
	attrs := make(map[string]string, len(span.GetAttributes()))
	var peerService string

	for _, kv := range span.GetAttributes() {
		key := kv.GetKey()
		val := stringValue(kv.GetValue())
		attrs[key] = val

		if key == "peer.service" {
			peerService = val
		}
	}

	startNano := span.GetStartTimeUnixNano()
	endNano := span.GetEndTimeUnixNano()
	var startTime time.Time
	var duration time.Duration
	if startNano > 0 {
		startTime = time.Unix(0, int64(startNano))
	}
	if endNano > startNano {
		duration = time.Duration(endNano - startNano)
	}

	return TraceSpan{
		TraceID:       hex.EncodeToString(span.GetTraceId()),
		SpanID:        hex.EncodeToString(span.GetSpanId()),
		ParentSpanID:  hex.EncodeToString(span.GetParentSpanId()),
		ServiceName:   serviceName,
		OperationName: span.GetName(),
		PeerService:   peerService,
		Attributes:    attrs,
		StartTime:     startTime,
		Duration:      duration,
	}
}

// stringValue extracts a string from an OTLP AnyValue.
func stringValue(v *commonpb.AnyValue) string {
	if v == nil {
		return ""
	}
	switch val := v.GetValue().(type) {
	case *commonpb.AnyValue_StringValue:
		return val.StringValue
	case *commonpb.AnyValue_IntValue:
		return fmt.Sprintf("%d", val.IntValue)
	case *commonpb.AnyValue_BoolValue:
		return fmt.Sprintf("%t", val.BoolValue)
	case *commonpb.AnyValue_DoubleValue:
		return fmt.Sprintf("%g", val.DoubleValue)
	default:
		return fmt.Sprintf("%v", v)
	}
}

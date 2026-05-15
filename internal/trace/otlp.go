package trace

import (
	"context"
	"fmt"
	"sync"
)

// OTLPReceiver is a gRPC server that implements the OTLP trace receiver
// protocol. It receives ExportTraceServiceRequest messages, normalizes the
// spans to TraceSpan, and passes them to the TraceIngestor via batching.
//
// This implementation uses a simplified interface abstraction so the package
// compiles without the OTel proto dependency. When the dependency is added
// (go.opentelemetry.io/proto/otlp, google.golang.org/grpc), the Export
// method should be updated to accept the real protobuf types and the
// Start method should register with a grpc.Server.
type OTLPReceiver struct {
	ingestor TraceIngestor
	addr     string
	health   HealthState
	mu       sync.RWMutex
	stopFn   func() // set by Start, called by Stop
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

// Start begins accepting OTLP trace data. In the full implementation this
// creates a gRPC server, registers the OTLP trace service, and listens on
// the configured address.
//
// Current placeholder: sets health to CONNECTED and stores a cancel function
// for Stop. When the gRPC dependency is available, this should:
//  1. Create a grpc.NewServer()
//  2. Register collectortrace.RegisterTraceServiceServer(server, receiver)
//  3. net.Listen("tcp", addr)
//  4. server.Serve(lis)
func (r *OTLPReceiver) Start(_ context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.health == HealthConnected {
		return fmt.Errorf("otlp receiver already started")
	}

	// Placeholder: in the real implementation, start a gRPC server here.
	r.health = HealthConnected
	r.stopFn = func() {
		// Placeholder for grpcServer.GracefulStop()
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
	r.health = HealthDisabled
	return nil
}

// Health returns the current health state of the receiver.
func (r *OTLPReceiver) Health() HealthState {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.health
}

// ExportSpans processes a batch of OTLP-normalized spans. This is the
// simplified version that accepts pre-converted TraceSpan values. When the
// real OTLP proto dependency is available, this should be replaced by an
// Export method that accepts *collectortrace.ExportTraceServiceRequest and
// performs the conversion:
//
//  1. Extract service.name from Resource attributes
//  2. For each InstrumentationLibrarySpans -> Spans:
//     - Map Name to OperationName
//     - Map Attributes to TraceSpan.Attributes
//     - Extract http.method, http.route, rpc.service, rpc.method
//     - Extract peer service from span links or attributes
//  3. Call ingestor.AddToBatch for each span
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

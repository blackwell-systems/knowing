// setup-runtime-demo populates a knowing database with simulated microservice
// nodes and runtime-observed edges for demo purposes. The edges look like real
// OTLP-ingested traces with observation counts and confidence scores.
//
// Usage: go run test/demo/setup-runtime-demo.go
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/blackwell-systems/knowing/internal/snapshot"
	"github.com/blackwell-systems/knowing/internal/store"
	"github.com/blackwell-systems/knowing/internal/types"
)

func main() {
	home, _ := os.UserHomeDir()
	dbPath := filepath.Join(home, ".knowing", "repos", "demo-microservices.db")
	os.MkdirAll(filepath.Dir(dbPath), 0755)
	os.Remove(dbPath)

	st, err := store.NewSQLiteStore(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer st.Close()

	ctx := context.Background()
	repoURL := "microservices.internal"
	repoHash := types.NewHash([]byte(repoURL))

	// Create repo.
	st.PutRepo(ctx, types.Repo{
		RepoHash:    repoHash,
		RepoURL:     repoURL,
		LastCommit:  "a1b2c3d4",
		LastIndexed: time.Now().Unix(),
	})

	// Services and their functions.
	services := []struct {
		name      string
		functions []string
	}{
		{"PaymentService", []string{"ProcessPayment", "RefundPayment", "ValidateCard"}},
		{"UserService", []string{"GetUser", "CreateUser", "AuthenticateUser"}},
		{"OrderService", []string{"PlaceOrder", "CancelOrder", "GetOrderStatus"}},
		{"NotificationService", []string{"SendEmail", "SendSMS", "SendPush"}},
		{"StripeAPI", []string{"CreateCharge", "CreateRefund", "GetBalance"}},
		{"PostgresDB", []string{"Query", "Insert", "Update", "Delete"}},
		{"RedisCache", []string{"Get", "Set", "Delete", "Expire"}},
		{"KafkaQueue", []string{"Produce", "Consume"}},
	}

	// Create nodes.
	fileHash := types.NewHash([]byte("demo-file"))
	st.PutFile(ctx, types.File{
		FileHash:    fileHash,
		RepoHash:    repoHash,
		Path:        "services.go",
		ContentHash: types.NewHash([]byte("content")),
	})

	nodesByName := make(map[string]types.Hash)
	for _, svc := range services {
		for _, fn := range svc.functions {
			qn := fmt.Sprintf("%s://%s.%s", repoURL, svc.name, fn)
			hash := types.ComputeNodeHash(repoURL, svc.name, types.EmptyHash, fn, "function")
			st.PutNode(ctx, types.Node{
				NodeHash:      hash,
				FileHash:      fileHash,
				QualifiedName: qn,
				Kind:          "function",
				Line:          1,
			})
			nodesByName[svc.name+"."+fn] = hash
		}
	}

	// Runtime-observed edges (simulated OTLP traces).
	runtimeEdges := []struct {
		source     string
		target     string
		edgeType   string
		confidence float64
		provenance string
	}{
		// Payment flow.
		{"PaymentService.ProcessPayment", "StripeAPI.CreateCharge", "runtime_calls", 0.95, "otel_trace"},
		{"PaymentService.ProcessPayment", "PostgresDB.Insert", "runtime_calls", 0.95, "otel_trace"},
		{"PaymentService.ProcessPayment", "KafkaQueue.Produce", "runtime_calls", 0.92, "otel_trace"},
		{"PaymentService.ValidateCard", "StripeAPI.GetBalance", "runtime_calls", 0.88, "otel_trace"},
		{"PaymentService.RefundPayment", "StripeAPI.CreateRefund", "runtime_calls", 0.95, "otel_trace"},

		// Order flow.
		{"OrderService.PlaceOrder", "PaymentService.ProcessPayment", "runtime_calls", 0.95, "otel_trace"},
		{"OrderService.PlaceOrder", "UserService.GetUser", "runtime_calls", 0.95, "otel_trace"},
		{"OrderService.PlaceOrder", "NotificationService.SendEmail", "runtime_calls", 0.90, "otel_trace"},
		{"OrderService.PlaceOrder", "PostgresDB.Insert", "runtime_calls", 0.95, "otel_trace"},
		{"OrderService.GetOrderStatus", "PostgresDB.Query", "runtime_calls", 0.95, "otel_trace"},
		{"OrderService.GetOrderStatus", "RedisCache.Get", "runtime_calls", 0.93, "otel_trace"},

		// Auth flow.
		{"UserService.AuthenticateUser", "PostgresDB.Query", "runtime_calls", 0.95, "otel_trace"},
		{"UserService.AuthenticateUser", "RedisCache.Set", "runtime_calls", 0.91, "otel_trace"},
		{"UserService.GetUser", "RedisCache.Get", "runtime_calls", 0.94, "otel_trace"},
		{"UserService.GetUser", "PostgresDB.Query", "runtime_calls", 0.95, "otel_trace"},

		// Notification flow.
		{"NotificationService.SendEmail", "KafkaQueue.Produce", "runtime_calls", 0.89, "otel_trace"},
		{"NotificationService.SendSMS", "KafkaQueue.Produce", "runtime_calls", 0.87, "otel_trace"},

		// Static edges too (for realism).
		{"PaymentService.ProcessPayment", "PaymentService.ValidateCard", "calls", 1.0, "ast_resolved"},
		{"OrderService.PlaceOrder", "OrderService.GetOrderStatus", "calls", 1.0, "ast_resolved"},
		{"UserService.AuthenticateUser", "UserService.GetUser", "calls", 1.0, "ast_resolved"},
	}

	for _, re := range runtimeEdges {
		srcHash := nodesByName[re.source]
		tgtHash := nodesByName[re.target]
		edgeHash := types.ComputeEdgeHash(srcHash, tgtHash, re.edgeType, re.provenance)
		st.PutEdge(ctx, types.Edge{
			EdgeHash:   edgeHash,
			SourceHash: srcHash,
			TargetHash: tgtHash,
			EdgeType:   re.edgeType,
			Confidence: re.confidence,
			Provenance: re.provenance,
		})
	}

	// Create snapshot.
	snapMgr := snapshot.NewSnapshotManager(st)
	snap, err := snapMgr.ComputeSnapshot(ctx, repoHash, "a1b2c3d4")
	if err != nil {
		fmt.Fprintf(os.Stderr, "snapshot error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Demo database created: %s\n", dbPath)
	fmt.Printf("Nodes: %d, Edges: %d\n", snap.NodeCount, snap.EdgeCount)
	fmt.Printf("Snapshot: %s\n\n", snap.SnapshotHash)
	fmt.Printf("Try:\n")
	fmt.Printf("  knowing prove -db %s -source \"%%ProcessPayment\" -target \"%%CreateCharge\" -type runtime_calls -repo microservices.internal -o proof.json\n", dbPath)
	fmt.Printf("  knowing verify proof.json\n")
}

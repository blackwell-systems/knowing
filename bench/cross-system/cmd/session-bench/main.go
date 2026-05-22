package main

import (
	"context"
	"fmt"
	"time"

	knowingctx "github.com/blackwell-systems/knowing/internal/context"
	"github.com/blackwell-systems/knowing/internal/store"
)

func main() {
	s, err := store.NewSQLiteStore("bench/cross-system/corpus/repos/flask/.knowing/graph.db")
	if err != nil {
		fmt.Println("ERROR:", err)
		return
	}
	defer s.Close()

	tasks := []string{
		"Add a before_request hook for API key validation",
		"Implement custom error handler for 404 pages",
		"Add URL converter for UUID parameters",
		"Create a new Blueprint for admin routes",
		"Add session cookie configuration",
		"Implement request context teardown",
		"Add JSON response helper",
		"Configure CORS headers middleware",
		"Add template filter for date formatting",
		"Implement file upload endpoint",
	}

	engine := knowingctx.NewContextEngine(s)
	tm := knowingctx.NewTaskMemory(s.DB())
	engine.SetTaskMemory(tm)

	ctx := context.Background()
	start := time.Now()

	for i, task := range tasks {
		qStart := time.Now()
		result, err := engine.ForTask(ctx, knowingctx.TaskOptions{
			TaskDescription: task,
			TokenBudget:     5000,
			Format:          "json",
		})
		qLatency := time.Since(qStart)
		if err != nil {
			fmt.Printf("  [%d] ERROR: %v\n", i+1, err)
			continue
		}
		fmt.Printf("  [%d] %dms, %d symbols, %d tokens\n", i+1, qLatency.Milliseconds(), len(result.Symbols), result.TokensUsed)
	}

	total := time.Since(start)
	fmt.Printf("\n  10 queries in %dms (avg %dms/query)\n", total.Milliseconds(), total.Milliseconds()/10)
}

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	knowingmcp "github.com/blackwell-systems/knowing/internal/mcp"
	"github.com/blackwell-systems/knowing/internal/store"
)

// cmdMCP runs the MCP server over stdio. This is the mode used by AI agents
// via .mcp.json configuration. Opens the database read-only and serves MCP
// tool calls until stdin closes or SIGINT/SIGTERM is received.
func cmdMCP(args []string) error {
	fs := flag.NewFlagSet("mcp", flag.ExitOnError)
	dbPath := fs.String("db", defaultDB(), "Path to the SQLite database (env: KNOWING_DB)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if _, err := os.Stat(*dbPath); os.IsNotExist(err) {
		return fmt.Errorf("database not found: %s (run 'knowing index' first)", *dbPath)
	}

	st, err := store.NewSQLiteStore(*dbPath)
	if err != nil {
		return fmt.Errorf("opening store: %w", err)
	}
	defer st.Close()

	mcpServer := knowingmcp.NewServer(st)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	return mcpServer.ServeStdio(ctx)
}

package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestVersionSubcommand(t *testing.T) {
	// Capture stdout.
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w

	err = run([]string{"version"})

	w.Close()
	os.Stdout = old

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatal(err)
	}

	got := strings.TrimSpace(buf.String())
	if got != Version {
		t.Errorf("got %q, want %q", got, Version)
	}
}

func TestMainFunction_NoArgs_PrintsUsage(t *testing.T) {
	// Capture stderr where usage is printed.
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w

	err = run([]string{})

	w.Close()
	os.Stderr = old

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatal(err)
	}

	output := buf.String()
	if !strings.Contains(output, "Usage:") {
		t.Errorf("expected usage output, got %q", output)
	}
	if !strings.Contains(output, "serve") {
		t.Errorf("expected 'serve' in usage, got %q", output)
	}
	if !strings.Contains(output, "index") {
		t.Errorf("expected 'index' in usage, got %q", output)
	}
	if !strings.Contains(output, "query") {
		t.Errorf("expected 'query' in usage, got %q", output)
	}
	if !strings.Contains(output, "version") {
		t.Errorf("expected 'version' in usage, got %q", output)
	}
}

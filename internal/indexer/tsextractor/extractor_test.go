package tsextractor

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/blackwell-systems/knowing/internal/types"
)

// makeOpts creates ExtractOptions for testing with the given filename and source.
func makeOpts(t *testing.T, filename, source string) types.ExtractOptions {
	t.Helper()
	dir := t.TempDir()
	fileHash := types.NewHash([]byte(source))
	repoHash := types.NewHash([]byte("test://repo"))
	return types.ExtractOptions{
		RepoURL:    "test://repo",
		RepoHash:   repoHash,
		CommitHash: "abc123",
		FilePath:   filename,
		FileHash:   fileHash,
		Content:    []byte(source),
		ModuleRoot: dir,
	}
}

func TestTypeScriptExtractor_Name(t *testing.T) {
	ext := NewTypeScriptExtractor()
	if got := ext.Name(); got != "treesitter-typescript" {
		t.Errorf("Name() = %q, want %q", got, "treesitter-typescript")
	}
}

func TestTypeScriptExtractor_CanHandle(t *testing.T) {
	ext := NewTypeScriptExtractor()

	tests := []struct {
		path string
		want bool
	}{
		{"app.ts", true},
		{"component.tsx", true},
		{"index.js", true},
		{"component.jsx", true},
		{"src/utils/helper.ts", true},
		{"lib/index.js", true},
		{"main.go", false},
		{"script.py", false},
		{"lib.rs", false},
		{"node_modules/express/index.js", false},
		{"src/node_modules/lib/index.ts", false},
		{"styles.css", false},
		{"", false},
	}

	for _, tt := range tests {
		got := ext.CanHandle(tt.path)
		if got != tt.want {
			t.Errorf("CanHandle(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestTypeScriptExtractor_ExtractFunctions(t *testing.T) {
	ext := NewTypeScriptExtractor()
	source := `function handleRequest(req: Request): Response {
  return new Response("ok");
}

function processData(data: string[]): void {
  console.log(data);
}
`
	opts := makeOpts(t, "handler.ts", source)

	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	// Find function nodes.
	var funcNodes []types.Node
	for _, n := range result.Nodes {
		if n.Kind == "function" {
			funcNodes = append(funcNodes, n)
		}
	}

	if len(funcNodes) != 2 {
		t.Fatalf("expected 2 function nodes, got %d: %+v", len(funcNodes), result.Nodes)
	}

	// Check names are present in qualified names.
	names := make(map[string]bool)
	for _, n := range funcNodes {
		if n.QualifiedName != "" {
			names[n.QualifiedName] = true
		}
	}

	foundHandle := false
	foundProcess := false
	for q := range names {
		if filepath.Base(q) == "handleRequest" || contains(q, "handleRequest") {
			foundHandle = true
		}
		if filepath.Base(q) == "processData" || contains(q, "processData") {
			foundProcess = true
		}
	}
	if !foundHandle {
		t.Errorf("missing handleRequest in qualified names: %v", names)
	}
	if !foundProcess {
		t.Errorf("missing processData in qualified names: %v", names)
	}

	// Verify line numbers are set.
	for _, n := range funcNodes {
		if n.Line == 0 {
			t.Errorf("function %q has Line=0, expected non-zero", n.QualifiedName)
		}
	}
}

func TestTypeScriptExtractor_ExtractClasses(t *testing.T) {
	ext := NewTypeScriptExtractor()
	source := `class UserController {
  getUser(id: string) {
    return db.findUser(id);
  }

  createUser(name: string) {
    return db.insert(name);
  }
}
`
	opts := makeOpts(t, "controller.ts", source)

	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	var classNodes []types.Node
	var methodNodes []types.Node
	for _, n := range result.Nodes {
		switch n.Kind {
		case "type":
			classNodes = append(classNodes, n)
		case "method":
			methodNodes = append(methodNodes, n)
		}
	}

	if len(classNodes) != 1 {
		t.Fatalf("expected 1 class node, got %d", len(classNodes))
	}
	if !contains(classNodes[0].QualifiedName, "UserController") {
		t.Errorf("class QualifiedName = %q, want to contain UserController", classNodes[0].QualifiedName)
	}

	if len(methodNodes) != 2 {
		t.Fatalf("expected 2 method nodes, got %d", len(methodNodes))
	}

	// Methods should have class name in qualified name.
	for _, m := range methodNodes {
		if !contains(m.QualifiedName, "UserController") {
			t.Errorf("method QualifiedName %q should contain UserController", m.QualifiedName)
		}
	}
}

func TestTypeScriptExtractor_ExtractInterfaces(t *testing.T) {
	ext := NewTypeScriptExtractor()
	source := `interface UserService {
  getUser(id: string): User;
  deleteUser(id: string): void;
}

interface Logger {
  log(message: string): void;
}
`
	opts := makeOpts(t, "interfaces.ts", source)

	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	var ifaceNodes []types.Node
	for _, n := range result.Nodes {
		if n.Kind == "interface" {
			ifaceNodes = append(ifaceNodes, n)
		}
	}

	if len(ifaceNodes) != 2 {
		t.Fatalf("expected 2 interface nodes, got %d", len(ifaceNodes))
	}

	names := make(map[string]bool)
	for _, n := range ifaceNodes {
		names[n.QualifiedName] = true
	}

	foundUserService := false
	foundLogger := false
	for q := range names {
		if contains(q, "UserService") {
			foundUserService = true
		}
		if contains(q, "Logger") {
			foundLogger = true
		}
	}
	if !foundUserService {
		t.Errorf("missing UserService interface in qualified names: %v", names)
	}
	if !foundLogger {
		t.Errorf("missing Logger interface in qualified names: %v", names)
	}
}

func TestTypeScriptExtractor_ExtractImports(t *testing.T) {
	ext := NewTypeScriptExtractor()
	source := `import { Router } from 'express';
import * as path from 'path';
import fs from 'fs';
`
	opts := makeOpts(t, "app.ts", source)

	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	var importEdges []types.Edge
	for _, e := range result.Edges {
		if e.EdgeType == "imports" {
			importEdges = append(importEdges, e)
		}
	}

	if len(importEdges) != 3 {
		t.Fatalf("expected 3 import edges, got %d", len(importEdges))
	}

	// All import edges should have correct provenance and confidence.
	for _, e := range importEdges {
		if e.Confidence != 0.7 {
			t.Errorf("import edge confidence = %v, want 0.7", e.Confidence)
		}
		if e.Provenance != "ast_inferred" {
			t.Errorf("import edge provenance = %q, want %q", e.Provenance, "ast_inferred")
		}
	}
}

func TestTypeScriptExtractor_ExtractRequire(t *testing.T) {
	ext := NewTypeScriptExtractor()
	source := `const express = require('express');
const path = require('path');
`
	opts := makeOpts(t, "app.js", source)

	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	var importEdges []types.Edge
	for _, e := range result.Edges {
		if e.EdgeType == "imports" {
			importEdges = append(importEdges, e)
		}
	}

	if len(importEdges) != 2 {
		t.Fatalf("expected 2 import edges from require() calls, got %d", len(importEdges))
	}

	for _, e := range importEdges {
		if e.Confidence != 0.7 {
			t.Errorf("require edge confidence = %v, want 0.7", e.Confidence)
		}
		if e.Provenance != "ast_inferred" {
			t.Errorf("require edge provenance = %q, want %q", e.Provenance, "ast_inferred")
		}
	}
}

func TestTypeScriptExtractor_ExtractCallEdges(t *testing.T) {
	ext := NewTypeScriptExtractor()
	source := `function main() {
  console.log("start");
  processData();
  utils.format("test");
}

function processData() {}
`
	opts := makeOpts(t, "app.ts", source)

	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	var callEdges []types.Edge
	for _, e := range result.Edges {
		if e.EdgeType == "calls" {
			callEdges = append(callEdges, e)
		}
	}

	if len(callEdges) < 3 {
		t.Fatalf("expected at least 3 call edges, got %d", len(callEdges))
	}

	// All call edges should have call-site positions.
	for _, e := range callEdges {
		if e.CallSiteLine == 0 {
			t.Errorf("call edge has CallSiteLine=0, expected non-zero")
		}
		if e.CallSiteFile != "app.ts" {
			t.Errorf("call edge CallSiteFile = %q, want %q", e.CallSiteFile, "app.ts")
		}
		if e.Confidence != 0.7 {
			t.Errorf("call edge confidence = %v, want 0.7", e.Confidence)
		}
		if e.Provenance != "ast_inferred" {
			t.Errorf("call edge provenance = %q, want %q", e.Provenance, "ast_inferred")
		}
	}
}

func TestTypeScriptExtractor_ExtractArrowFunctions(t *testing.T) {
	ext := NewTypeScriptExtractor()
	source := `const greet = (name: string) => {
  return "Hello " + name;
};

const add = (a: number, b: number) => a + b;
`
	opts := makeOpts(t, "utils.ts", source)

	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	var funcNodes []types.Node
	for _, n := range result.Nodes {
		if n.Kind == "function" {
			funcNodes = append(funcNodes, n)
		}
	}

	if len(funcNodes) != 2 {
		t.Fatalf("expected 2 function nodes (arrow functions), got %d: %+v", len(funcNodes), result.Nodes)
	}

	names := make(map[string]bool)
	for _, n := range funcNodes {
		names[n.QualifiedName] = true
	}

	foundGreet := false
	foundAdd := false
	for q := range names {
		if contains(q, "greet") {
			foundGreet = true
		}
		if contains(q, "add") {
			foundAdd = true
		}
	}
	if !foundGreet {
		t.Errorf("missing greet arrow function: %v", names)
	}
	if !foundAdd {
		t.Errorf("missing add arrow function: %v", names)
	}
}

func TestTypeScriptExtractor_ExpressRoutes(t *testing.T) {
	ext := NewTypeScriptExtractor()
	source := `import { Router } from 'express';
const router = Router();

function handleRequest(req: any) {
  return "ok";
}

router.get('/users/:id', handleRequest);
router.post('/users', handleRequest);
`
	opts := makeOpts(t, "routes.ts", source)

	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	var routeNodes []types.Node
	for _, n := range result.Nodes {
		if n.Kind == "route_handler" {
			routeNodes = append(routeNodes, n)
		}
	}

	if len(routeNodes) != 2 {
		t.Fatalf("expected 2 route_handler nodes, got %d: %+v", len(routeNodes), result.Nodes)
	}

	patterns := make(map[string]bool)
	for _, n := range routeNodes {
		patterns[n.Signature] = true
	}
	if !patterns["GET /users/:id"] {
		t.Errorf("missing route pattern 'GET /users/:id', got %v", patterns)
	}
	if !patterns["POST /users"] {
		t.Errorf("missing route pattern 'POST /users', got %v", patterns)
	}

	// Check handles_route edges.
	var routeEdges []types.Edge
	for _, e := range result.Edges {
		if e.EdgeType == "handles_route" {
			routeEdges = append(routeEdges, e)
		}
	}
	if len(routeEdges) != 2 {
		t.Fatalf("expected 2 handles_route edges, got %d", len(routeEdges))
	}

	for _, e := range routeEdges {
		if e.Confidence != 0.7 {
			t.Errorf("handles_route edge confidence = %v, want 0.7", e.Confidence)
		}
		if e.Provenance != "ast_inferred" {
			t.Errorf("handles_route edge provenance = %q, want %q", e.Provenance, "ast_inferred")
		}
	}
}

func TestTypeScriptExtractor_NoRoutes(t *testing.T) {
	ext := NewTypeScriptExtractor()
	source := `function main() {
  console.log("no express here");
}
`
	opts := makeOpts(t, "app.ts", source)

	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	// No route_handler nodes should be present.
	for _, n := range result.Nodes {
		if n.Kind == "route_handler" {
			t.Errorf("unexpected route_handler node: %+v", n)
		}
	}

	// No handles_route edges should be present.
	for _, e := range result.Edges {
		if e.EdgeType == "handles_route" {
			t.Errorf("unexpected handles_route edge: %+v", e)
		}
	}
}

func TestTypeScriptExtractor_EdgeProvenanceAndConfidence(t *testing.T) {
	ext := NewTypeScriptExtractor()
	source := `import { something } from 'some-lib';

function doWork() {
  something();
}
`
	opts := makeOpts(t, "worker.ts", source)

	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	if len(result.Edges) == 0 {
		t.Fatal("expected at least one edge")
	}

	for _, e := range result.Edges {
		switch e.EdgeType {
		case "imports":
			// Import edges remain ast_inferred with confidence 0.7.
			if e.Confidence != 0.7 {
				t.Errorf("edge %s confidence = %v, want 0.7", e.EdgeType, e.Confidence)
			}
			if e.Provenance != "ast_inferred" {
				t.Errorf("edge %s provenance = %q, want %q", e.EdgeType, e.Provenance, "ast_inferred")
			}
		case "calls":
			// Call edges resolved through the import map to an external package
			// get ast_resolved provenance with confidence 0.85.
			if e.Confidence != 0.85 {
				t.Errorf("edge %s confidence = %v, want 0.85", e.EdgeType, e.Confidence)
			}
			if e.Provenance != "ast_resolved" {
				t.Errorf("edge %s provenance = %q, want %q", e.EdgeType, e.Provenance, "ast_resolved")
			}
		default:
			if e.Confidence != 0.7 {
				t.Errorf("edge %s confidence = %v, want 0.7", e.EdgeType, e.Confidence)
			}
		}
	}
}

func TestTypeScriptExtractor_FastifyRoutes(t *testing.T) {
	ext := NewTypeScriptExtractor()
	source := `import Fastify from 'fastify';
const app = Fastify();

function getUsers(req, reply) {
  return reply.send([]);
}

app.get('/users', getUsers);
app.post('/users', getUsers);
`
	opts := makeOpts(t, "server.ts", source)

	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	var routeNodes []types.Node
	for _, n := range result.Nodes {
		if n.Kind == "route_handler" {
			routeNodes = append(routeNodes, n)
		}
	}

	if len(routeNodes) != 2 {
		t.Fatalf("expected 2 route_handler nodes, got %d", len(routeNodes))
	}

	patterns := make(map[string]bool)
	for _, n := range routeNodes {
		patterns[n.Signature] = true
	}
	if !patterns["GET /users"] {
		t.Errorf("missing 'GET /users', got %v", patterns)
	}
	if !patterns["POST /users"] {
		t.Errorf("missing 'POST /users', got %v", patterns)
	}
}

func TestTypeScriptExtractor_HonoRoutes(t *testing.T) {
	ext := NewTypeScriptExtractor()
	source := `import { Hono } from 'hono';
const app = new Hono();

app.get('/api/health', (c) => c.json({ status: 'ok' }));
app.post('/api/items', (c) => c.json({ created: true }));
app.delete('/api/items/:id', (c) => c.json({ deleted: true }));
`
	opts := makeOpts(t, "index.ts", source)

	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	var routeNodes []types.Node
	for _, n := range result.Nodes {
		if n.Kind == "route_handler" {
			routeNodes = append(routeNodes, n)
		}
	}

	if len(routeNodes) != 3 {
		t.Fatalf("expected 3 route_handler nodes, got %d", len(routeNodes))
	}

	patterns := make(map[string]bool)
	for _, n := range routeNodes {
		patterns[n.Signature] = true
	}
	if !patterns["GET /api/health"] {
		t.Errorf("missing 'GET /api/health', got %v", patterns)
	}
	if !patterns["POST /api/items"] {
		t.Errorf("missing 'POST /api/items', got %v", patterns)
	}
	if !patterns["DELETE /api/items/:id"] {
		t.Errorf("missing 'DELETE /api/items/:id', got %v", patterns)
	}
}

func TestTypeScriptExtractor_NextJSRoutes(t *testing.T) {
	ext := NewTypeScriptExtractor()
	source := `import { NextResponse } from 'next/server';

export async function GET(request: Request) {
  return NextResponse.json({ users: [] });
}

export async function POST(request: Request) {
  return NextResponse.json({ created: true });
}
`
	opts := makeOpts(t, "app/api/users/route.ts", source)

	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	var routeNodes []types.Node
	for _, n := range result.Nodes {
		if n.Kind == "route_handler" {
			routeNodes = append(routeNodes, n)
		}
	}

	if len(routeNodes) != 2 {
		t.Fatalf("expected 2 route_handler nodes for Next.js, got %d", len(routeNodes))
	}

	patterns := make(map[string]bool)
	for _, n := range routeNodes {
		patterns[n.Signature] = true
	}
	if !patterns["GET /api/users"] {
		t.Errorf("missing 'GET /api/users', got %v", patterns)
	}
	if !patterns["POST /api/users"] {
		t.Errorf("missing 'POST /api/users', got %v", patterns)
	}

	// Should also have handles_route edges.
	var routeEdges []types.Edge
	for _, e := range result.Edges {
		if e.EdgeType == "handles_route" {
			routeEdges = append(routeEdges, e)
		}
	}
	if len(routeEdges) != 2 {
		t.Fatalf("expected 2 handles_route edges for Next.js, got %d", len(routeEdges))
	}
}

func TestTypeScriptExtractor_EmptyFile(t *testing.T) {
	ext := NewTypeScriptExtractor()
	source := ``
	opts := makeOpts(t, "empty.ts", source)

	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	if len(result.Nodes) != 0 {
		t.Errorf("expected 0 nodes, got %d: %+v", len(result.Nodes), result.Nodes)
	}
	if len(result.Edges) != 0 {
		t.Errorf("expected 0 edges, got %d: %+v", len(result.Edges), result.Edges)
	}
}

func TestTypeScriptExtractor_ExternalImportEdgeUsesExternalRepoURL(t *testing.T) {
	ext := NewTypeScriptExtractor()
	source := `import { useState } from 'react';
import { helper } from './utils';
`
	opts := makeOpts(t, "component.tsx", source)

	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	var importEdges []types.Edge
	for _, e := range result.Edges {
		if e.EdgeType == "imports" {
			importEdges = append(importEdges, e)
		}
	}

	if len(importEdges) != 2 {
		t.Fatalf("expected 2 import edges, got %d", len(importEdges))
	}

	// The external import ("react") should produce a different target hash
	// than a local import would. Compute what the external target hash should be.
	externalTargetHash := types.ComputeNodeHash("external://react", "react", types.EmptyHash, "react", "module")

	// The local import ("./utils") should use the local repo URL.
	localTargetHash := types.ComputeNodeHash("test://repo", "./utils", types.EmptyHash, "./utils", "module")

	foundExternal := false
	foundLocal := false
	for _, e := range importEdges {
		if e.TargetHash == externalTargetHash {
			foundExternal = true
		}
		if e.TargetHash == localTargetHash {
			foundLocal = true
		}
	}

	if !foundExternal {
		t.Error("external import (react) did not produce target hash with external://react repo URL")
		for _, e := range importEdges {
			t.Logf("  edge target hash: %v", e.TargetHash)
		}
		t.Logf("  expected target hash: %v", externalTargetHash)
	}
	if !foundLocal {
		t.Error("local import (./utils) did not produce target hash with local repo URL")
		for _, e := range importEdges {
			t.Logf("  edge target hash: %v", e.TargetHash)
		}
		t.Logf("  expected target hash: %v", localTargetHash)
	}
}

func TestTypeScriptExtractor_ScopedExternalImportEdge(t *testing.T) {
	ext := NewTypeScriptExtractor()
	source := `import { Controller } from '@nestjs/common';
`
	opts := makeOpts(t, "app.controller.ts", source)

	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	var importEdges []types.Edge
	for _, e := range result.Edges {
		if e.EdgeType == "imports" {
			importEdges = append(importEdges, e)
		}
	}

	if len(importEdges) != 1 {
		t.Fatalf("expected 1 import edge, got %d", len(importEdges))
	}

	// Scoped package should use "external://@nestjs/common" as repo URL.
	expectedTargetHash := types.ComputeNodeHash("external://@nestjs/common", "@nestjs/common", types.EmptyHash, "@nestjs/common", "module")
	if importEdges[0].TargetHash != expectedTargetHash {
		t.Errorf("scoped external import did not use external://@nestjs/common repo URL")
		t.Logf("  got target hash: %v", importEdges[0].TargetHash)
		t.Logf("  expected: %v", expectedTargetHash)
	}
}

// contains is a helper to check if a string contains a substring.
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

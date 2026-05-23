package csharpextractor

import (
	"context"
	"testing"

	"github.com/blackwell-systems/knowing/internal/types"
)

func makeOpts(content string) types.ExtractOptions {
	return types.ExtractOptions{
		RepoURL:    "github.com/test/repo",
		FilePath:   "src/MyApp/Controllers/UsersController.cs",
		Content:    []byte(content),
		FileHash:   types.NewHash([]byte("test-file-hash")),
		ModuleRoot: "/test/repo",
	}
}

func TestCSharpExtractor_Name(t *testing.T) {
	ext := NewCSharpExtractor()
	if got := ext.Name(); got != "treesitter-csharp" {
		t.Errorf("Name() = %q, want %q", got, "treesitter-csharp")
	}
}

func TestCSharpExtractor_CanHandle(t *testing.T) {
	ext := NewCSharpExtractor()

	tests := []struct {
		path string
		want bool
	}{
		{"Program.cs", true},
		{"src/Controllers/UsersController.cs", true},
		{"MyApp/Models/User.cs", true},
		{"README.md", false},
		{"main.go", false},
		{"script.py", false},
		{"bin/Debug/net6.0/MyApp.cs", false},
		{"obj/Release/net6.0/MyApp.cs", false},
		{"src/bin/output.cs", false},
		{"src/obj/generated.cs", false},
	}

	for _, tt := range tests {
		if got := ext.CanHandle(tt.path); got != tt.want {
			t.Errorf("CanHandle(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestCSharpExtractor_ExtractClasses(t *testing.T) {
	ext := NewCSharpExtractor()
	src := `
public class UserService
{
}

public class OrderService
{
}
`
	opts := makeOpts(src)
	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	var classNodes []types.Node
	for _, n := range result.Nodes {
		if n.Kind == "type" {
			classNodes = append(classNodes, n)
		}
	}

	if len(classNodes) != 2 {
		t.Fatalf("expected 2 class nodes, got %d", len(classNodes))
	}

	names := map[string]bool{}
	for _, n := range classNodes {
		if n.Signature == "class UserService" {
			names["UserService"] = true
		}
		if n.Signature == "class OrderService" {
			names["OrderService"] = true
		}
	}
	if !names["UserService"] || !names["OrderService"] {
		t.Errorf("expected UserService and OrderService class nodes, got signatures: %v, %v",
			classNodes[0].Signature, classNodes[1].Signature)
	}
}

func TestCSharpExtractor_ExtractMethods(t *testing.T) {
	ext := NewCSharpExtractor()
	src := `
public class MyService
{
    public void DoWork()
    {
    }

    public int Calculate(int x)
    {
        return x * 2;
    }
}
`
	opts := makeOpts(src)
	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	var methodNodes []types.Node
	for _, n := range result.Nodes {
		if n.Kind == "method" {
			methodNodes = append(methodNodes, n)
		}
	}

	if len(methodNodes) < 2 {
		t.Fatalf("expected at least 2 method nodes, got %d", len(methodNodes))
	}

	found := map[string]bool{}
	for _, n := range methodNodes {
		if n.Signature == "method DoWork" {
			found["DoWork"] = true
		}
		if n.Signature == "method Calculate" {
			found["Calculate"] = true
		}
	}
	if !found["DoWork"] {
		t.Error("expected method node for DoWork")
	}
	if !found["Calculate"] {
		t.Error("expected method node for Calculate")
	}
}

func TestCSharpExtractor_ExtractInterfaces(t *testing.T) {
	ext := NewCSharpExtractor()
	src := `
public interface IUserService
{
    void Create();
}

public interface IOrderService
{
    void Process();
}
`
	opts := makeOpts(src)
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

	found := map[string]bool{}
	for _, n := range ifaceNodes {
		if n.Signature == "interface IUserService" {
			found["IUserService"] = true
		}
		if n.Signature == "interface IOrderService" {
			found["IOrderService"] = true
		}
	}
	if !found["IUserService"] || !found["IOrderService"] {
		t.Errorf("expected IUserService and IOrderService, got: %+v", ifaceNodes)
	}
}

func TestCSharpExtractor_ExtractStructs(t *testing.T) {
	ext := NewCSharpExtractor()
	src := `
public struct Point
{
    public int X;
    public int Y;
}
`
	opts := makeOpts(src)
	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	var structNodes []types.Node
	for _, n := range result.Nodes {
		if n.Kind == "type" && n.Signature == "struct Point" {
			structNodes = append(structNodes, n)
		}
	}

	if len(structNodes) != 1 {
		t.Fatalf("expected 1 struct node, got %d", len(structNodes))
	}
}

func TestCSharpExtractor_ExtractEnums(t *testing.T) {
	ext := NewCSharpExtractor()
	src := `
public enum Color
{
    Red,
    Green,
    Blue
}
`
	opts := makeOpts(src)
	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	var enumNodes []types.Node
	for _, n := range result.Nodes {
		if n.Kind == "type" && n.Signature == "enum Color" {
			enumNodes = append(enumNodes, n)
		}
	}

	if len(enumNodes) != 1 {
		t.Fatalf("expected 1 enum node, got %d", len(enumNodes))
	}
}

func TestCSharpExtractor_ExtractConstructors(t *testing.T) {
	ext := NewCSharpExtractor()
	src := `
public class UserService
{
    private readonly ILogger _logger;

    public UserService(ILogger logger)
    {
        _logger = logger;
    }
}
`
	opts := makeOpts(src)
	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	var ctorNodes []types.Node
	for _, n := range result.Nodes {
		if n.Kind == "method" && n.Signature == "constructor UserService" {
			ctorNodes = append(ctorNodes, n)
		}
	}

	if len(ctorNodes) != 1 {
		t.Fatalf("expected 1 constructor node, got %d", len(ctorNodes))
	}
}

func TestCSharpExtractor_ExtractUsingDirectives(t *testing.T) {
	ext := NewCSharpExtractor()
	src := `
using Microsoft.AspNetCore.Mvc;
using System.Collections.Generic;
`
	opts := makeOpts(src)
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

	// Verify provenance and confidence.
	for _, e := range importEdges {
		if e.Provenance != "ast_inferred" {
			t.Errorf("import edge provenance = %q, want %q", e.Provenance, "ast_inferred")
		}
		if e.Confidence != 0.7 {
			t.Errorf("import edge confidence = %f, want %f", e.Confidence, 0.7)
		}
	}
}

func TestCSharpExtractor_ExtractInvocations(t *testing.T) {
	ext := NewCSharpExtractor()
	src := `
public class UserService
{
    public void DoWork()
    {
        _logger.LogInfo("starting");
        Process();
    }
}
`
	opts := makeOpts(src)
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

	if len(callEdges) < 1 {
		t.Fatalf("expected at least 1 call edge, got %d", len(callEdges))
	}

	// Verify call-site positions are set.
	for _, e := range callEdges {
		if e.CallSiteLine == 0 {
			t.Error("expected non-zero CallSiteLine on call edge")
		}
		if e.CallSiteFile == "" {
			t.Error("expected non-empty CallSiteFile on call edge")
		}
	}
}

func TestCSharpExtractor_AspNetRoutes(t *testing.T) {
	ext := NewCSharpExtractor()
	src := `
using Microsoft.AspNetCore.Mvc;

namespace MyApp.Controllers
{
    [ApiController]
    [Route("api/[controller]")]
    public class UsersController : ControllerBase
    {
        private readonly IUserService _userService;

        public UsersController(IUserService userService)
        {
            _userService = userService;
        }

        [HttpGet("{id}")]
        public ActionResult<User> GetUser(int id)
        {
            return _userService.FindById(id);
        }

        [HttpPost]
        public ActionResult<User> CreateUser(User user)
        {
            return _userService.Create(user);
        }
    }
}
`
	opts := makeOpts(src)
	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	// Check for route_handler nodes.
	var routeNodes []types.Node
	for _, n := range result.Nodes {
		if n.Kind == "route_handler" {
			routeNodes = append(routeNodes, n)
		}
	}

	if len(routeNodes) < 2 {
		t.Fatalf("expected at least 2 route_handler nodes (HttpGet + HttpPost + possibly class Route), got %d: %+v", len(routeNodes), routeNodes)
	}

	// Check for handles_route edges.
	var routeEdges []types.Edge
	for _, e := range result.Edges {
		if e.EdgeType == "handles_route" {
			routeEdges = append(routeEdges, e)
		}
	}

	if len(routeEdges) < 2 {
		t.Fatalf("expected at least 2 handles_route edges, got %d", len(routeEdges))
	}

	// Verify route edge provenance.
	for _, e := range routeEdges {
		if e.Provenance != "ast_inferred" {
			t.Errorf("route edge provenance = %q, want %q", e.Provenance, "ast_inferred")
		}
		if e.Confidence != 0.7 {
			t.Errorf("route edge confidence = %f, want %f", e.Confidence, 0.7)
		}
	}
}

func TestCSharpExtractor_NoRoutes(t *testing.T) {
	ext := NewCSharpExtractor()
	src := `
public class SimpleService
{
    public void DoWork()
    {
    }
}
`
	opts := makeOpts(src)
	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	for _, n := range result.Nodes {
		if n.Kind == "route_handler" {
			t.Errorf("unexpected route_handler node: %+v", n)
		}
	}

	for _, e := range result.Edges {
		if e.EdgeType == "handles_route" {
			t.Errorf("unexpected handles_route edge: %+v", e)
		}
	}
}

func TestCSharpExtractor_EdgeProvenanceAndConfidence(t *testing.T) {
	ext := NewCSharpExtractor()
	src := `
using System;

public class MyClass
{
    public void MyMethod()
    {
        Console.WriteLine("hello");
    }
}
`
	opts := makeOpts(src)
	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	if len(result.Edges) == 0 {
		t.Fatal("expected at least one edge")
	}

	for _, e := range result.Edges {
		if e.Provenance != "ast_inferred" {
			t.Errorf("edge provenance = %q, want %q", e.Provenance, "ast_inferred")
		}
		if e.Confidence != 0.7 {
			t.Errorf("edge confidence = %f, want %f", e.Confidence, 0.7)
		}
	}
}

func TestCSharpExtractor_EmptyFile(t *testing.T) {
	ext := NewCSharpExtractor()
	opts := makeOpts("")
	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}
	if len(result.Nodes) != 0 {
		t.Errorf("expected 0 nodes for empty file, got %d", len(result.Nodes))
	}
	if len(result.Edges) != 0 {
		t.Errorf("expected 0 edges for empty file, got %d", len(result.Edges))
	}
}

func TestCSharpExtractor_ImportResolution(t *testing.T) {
	ext := NewCSharpExtractor()
	src := `using Microsoft.Extensions.Logging;
using System.Collections.Generic;

public class App
{
    public void Run()
    {
        Logging.CreateLogger();
        Generic.Create();
        LocalMethod();
    }
}
`
	opts := makeOpts(src)
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

	// Find edges by provenance.
	var resolved, inferred []types.Edge
	for _, e := range callEdges {
		switch e.Provenance {
		case "ast_resolved":
			resolved = append(resolved, e)
		case "ast_inferred":
			inferred = append(inferred, e)
		}
	}

	// Logging.CreateLogger() and Generic.Create() should be ast_resolved.
	if len(resolved) < 2 {
		t.Errorf("expected at least 2 ast_resolved edges, got %d", len(resolved))
	}
	for _, e := range resolved {
		if e.Confidence != 0.85 {
			t.Errorf("ast_resolved edge confidence = %f, want 0.85", e.Confidence)
		}
	}

	// LocalMethod() should be ast_inferred (no member access, no import match).
	if len(inferred) < 1 {
		t.Errorf("expected at least 1 ast_inferred edge, got %d", len(inferred))
	}
	for _, e := range inferred {
		if e.Confidence != 0.7 {
			t.Errorf("ast_inferred edge confidence = %f, want 0.7", e.Confidence)
		}
	}
}

func TestCSharpExtractor_ImportResolution_Static(t *testing.T) {
	ext := NewCSharpExtractor()
	src := `using static System.Console;
using static MyApp.Helpers.StringHelper;

public class App
{
    public void Run()
    {
        Console.WriteLine("hello");
        StringHelper.Format("test");
    }
}
`
	opts := makeOpts(src)
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

	if len(callEdges) < 2 {
		t.Fatalf("expected at least 2 call edges, got %d", len(callEdges))
	}

	// Both Console.WriteLine and StringHelper.Format should resolve through
	// the static import map since "Console" and "StringHelper" are mapped.
	var resolved []types.Edge
	for _, e := range callEdges {
		if e.Provenance == "ast_resolved" {
			resolved = append(resolved, e)
		}
	}

	if len(resolved) < 2 {
		t.Errorf("expected at least 2 ast_resolved edges for static imports, got %d", len(resolved))
	}
	for _, e := range resolved {
		if e.Confidence != 0.85 {
			t.Errorf("ast_resolved edge confidence = %f, want 0.85", e.Confidence)
		}
	}
}

func TestInferExternalRepoURL(t *testing.T) {
	tests := []struct {
		namespace string
		want      string
	}{
		{"System.Collections.Generic", "stdlib"},
		{"System", "stdlib"},
		{"System.IO", "stdlib"},
		{"Microsoft.Extensions.DependencyInjection", "stdlib"},
		{"Microsoft.AspNetCore.Mvc", "stdlib"},
		{"Newtonsoft.Json", "external://Newtonsoft.Json"},
		{"AutoMapper.Extensions", "external://AutoMapper.Extensions"},
		{"Serilog.Sinks.Console", "external://Serilog.Sinks"},
		{"FluentValidation", "external://FluentValidation"},
		{"", ""},
	}

	for _, tt := range tests {
		got := inferExternalRepoURL(tt.namespace)
		if got != tt.want {
			t.Errorf("inferExternalRepoURL(%q) = %q, want %q", tt.namespace, got, tt.want)
		}
	}
}

func TestCSharpExtractor_ExternalRepoURL_UsingDirective(t *testing.T) {
	ext := NewCSharpExtractor()
	src := `using Newtonsoft.Json;
using System.Collections.Generic;
`
	opts := makeOpts(src)
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

	// Verify that external namespaces produce target hashes using external repo URLs.
	// The target hash for "Newtonsoft.Json" should use "external://Newtonsoft.Json" as repoURL.
	expectedNewtonsoftTarget := types.ComputeNodeHash("external://Newtonsoft.Json", "Newtonsoft.Json", types.EmptyHash, "Newtonsoft.Json", "package")
	// The target hash for "System.Collections.Generic" should use "stdlib" as repoURL.
	expectedStdlibTarget := types.ComputeNodeHash("stdlib", "System.Collections.Generic", types.EmptyHash, "System.Collections.Generic", "package")

	foundNewtonsoft := false
	foundStdlib := false
	for _, e := range importEdges {
		if e.TargetHash == expectedNewtonsoftTarget {
			foundNewtonsoft = true
		}
		if e.TargetHash == expectedStdlibTarget {
			foundStdlib = true
		}
	}

	if !foundNewtonsoft {
		t.Error("expected import edge for Newtonsoft.Json to use external://Newtonsoft.Json repo URL")
	}
	if !foundStdlib {
		t.Error("expected import edge for System.Collections.Generic to use stdlib repo URL")
	}
}

func TestCSharpExtractor_ExternalRepoURL_InvocationResolution(t *testing.T) {
	ext := NewCSharpExtractor()
	src := `using Newtonsoft.Json;

public class App
{
    public void Run()
    {
        Json.SerializeObject("test");
    }
}
`
	opts := makeOpts(src)
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

	if len(callEdges) < 1 {
		t.Fatalf("expected at least 1 call edge, got %d", len(callEdges))
	}

	// The call to Json.SerializeObject should resolve through imports and use
	// the external repo URL "external://Newtonsoft.Json" for the target hash.
	expectedTarget := types.ComputeNodeHash("external://Newtonsoft.Json", "Newtonsoft.Json", types.EmptyHash, "SerializeObject", "method")

	found := false
	for _, e := range callEdges {
		if e.TargetHash == expectedTarget {
			found = true
			if e.Provenance != "ast_resolved" {
				t.Errorf("expected provenance ast_resolved, got %q", e.Provenance)
			}
			if e.Confidence != 0.85 {
				t.Errorf("expected confidence 0.85, got %f", e.Confidence)
			}
		}
	}

	if !found {
		t.Error("expected call edge to use external://Newtonsoft.Json repo URL for resolved import")
	}
}

func TestCSharpExtractor_ImportResolution_NoMatch(t *testing.T) {
	ext := NewCSharpExtractor()
	src := `public class App
{
    public void Run()
    {
        unknownService.DoWork();
    }
}
`
	opts := makeOpts(src)
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

	if len(callEdges) < 1 {
		t.Fatalf("expected at least 1 call edge, got %d", len(callEdges))
	}

	// "unknownService" starts with lowercase, so it should NOT be resolved
	// through imports. It should stay at ast_inferred / 0.7.
	for _, e := range callEdges {
		if e.Provenance != "ast_inferred" {
			t.Errorf("edge provenance = %q, want %q for unresolved call", e.Provenance, "ast_inferred")
		}
		if e.Confidence != 0.7 {
			t.Errorf("edge confidence = %f, want 0.7 for unresolved call", e.Confidence)
		}
	}
}

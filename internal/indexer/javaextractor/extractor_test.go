package javaextractor

import (
	"context"
	"testing"

	"github.com/blackwell-systems/knowing/internal/types"
)

// makeOpts creates ExtractOptions for testing Java source.
func makeOpts(filePath, source string) types.ExtractOptions {
	fileHash := types.NewHash([]byte(source))
	repoHash := types.NewHash([]byte("test://repo"))
	return types.ExtractOptions{
		RepoURL:    "test://repo",
		RepoHash:   repoHash,
		CommitHash: "abc123",
		FilePath:   filePath,
		FileHash:   fileHash,
		Content:    []byte(source),
		ModuleRoot: "/test/project",
	}
}

func TestJavaExtractor_Name(t *testing.T) {
	ext := NewJavaExtractor()
	if got := ext.Name(); got != "treesitter-java" {
		t.Errorf("Name() = %q, want %q", got, "treesitter-java")
	}
}

func TestJavaExtractor_CanHandle(t *testing.T) {
	ext := NewJavaExtractor()

	tests := []struct {
		path string
		want bool
	}{
		{"Main.java", true},
		{"src/com/example/Main.java", true},
		{"src/main/java/App.java", true},
		{"Main.go", false},
		{"Main.py", false},
		{"README.md", false},
		{"build/generated/Main.java", false},
		{"target/classes/Main.java", false},
		{"src/build/Main.java", false},
		{"src/target/Main.java", false},
		{"", false},
	}

	for _, tt := range tests {
		got := ext.CanHandle(tt.path)
		if got != tt.want {
			t.Errorf("CanHandle(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestJavaExtractor_ExtractClasses(t *testing.T) {
	ext := NewJavaExtractor()
	source := `public class UserService {
}

class InternalHelper {
}
`
	opts := makeOpts("src/UserService.java", source)

	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	// Should have at least 2 class nodes.
	classNodes := filterNodesByKind(result.Nodes, "type")
	if len(classNodes) < 2 {
		t.Fatalf("expected at least 2 type nodes, got %d: %v", len(classNodes), nodeNames(result.Nodes))
	}

	foundUserService := false
	foundInternalHelper := false
	for _, n := range classNodes {
		if containsName(n.QualifiedName, "UserService") && n.Signature == "class UserService" {
			foundUserService = true
		}
		if containsName(n.QualifiedName, "InternalHelper") && n.Signature == "class InternalHelper" {
			foundInternalHelper = true
		}
	}
	if !foundUserService {
		t.Error("missing UserService class node")
	}
	if !foundInternalHelper {
		t.Error("missing InternalHelper class node")
	}
}

func TestJavaExtractor_ExtractMethods(t *testing.T) {
	ext := NewJavaExtractor()
	source := `public class Calculator {
    public int add(int a, int b) {
        return a + b;
    }

    public int subtract(int a, int b) {
        return a - b;
    }
}
`
	opts := makeOpts("src/Calculator.java", source)

	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	methodNodes := filterNodesByKind(result.Nodes, "method")
	if len(methodNodes) < 2 {
		t.Fatalf("expected at least 2 method nodes, got %d: %v", len(methodNodes), nodeNames(result.Nodes))
	}

	foundAdd := false
	foundSubtract := false
	for _, n := range methodNodes {
		if containsName(n.QualifiedName, "Calculator.add") {
			foundAdd = true
		}
		if containsName(n.QualifiedName, "Calculator.subtract") {
			foundSubtract = true
		}
	}
	if !foundAdd {
		t.Error("missing Calculator.add method node")
	}
	if !foundSubtract {
		t.Error("missing Calculator.subtract method node")
	}
}

func TestJavaExtractor_ExtractInterfaces(t *testing.T) {
	ext := NewJavaExtractor()
	source := `public interface UserRepository {
    User findById(Long id);
    List<User> findAll();
}
`
	opts := makeOpts("src/UserRepository.java", source)

	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	ifaceNodes := filterNodesByKind(result.Nodes, "interface")
	if len(ifaceNodes) != 1 {
		t.Fatalf("expected 1 interface node, got %d: %v", len(ifaceNodes), nodeNames(result.Nodes))
	}
	if !containsName(ifaceNodes[0].QualifiedName, "UserRepository") {
		t.Errorf("interface QualifiedName = %q, want to contain UserRepository", ifaceNodes[0].QualifiedName)
	}

	// Interface methods should be extracted too.
	methodNodes := filterNodesByKind(result.Nodes, "method")
	if len(methodNodes) < 2 {
		t.Fatalf("expected at least 2 method nodes from interface, got %d", len(methodNodes))
	}
}

func TestJavaExtractor_ExtractEnums(t *testing.T) {
	ext := NewJavaExtractor()
	source := `public enum Status {
    ACTIVE,
    INACTIVE,
    PENDING
}
`
	opts := makeOpts("src/Status.java", source)

	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	typeNodes := filterNodesByKind(result.Nodes, "type")
	if len(typeNodes) != 1 {
		t.Fatalf("expected 1 type node for enum, got %d: %v", len(typeNodes), nodeNames(result.Nodes))
	}
	if !containsName(typeNodes[0].QualifiedName, "Status") {
		t.Errorf("enum QualifiedName = %q, want to contain Status", typeNodes[0].QualifiedName)
	}
	if typeNodes[0].Signature != "enum Status" {
		t.Errorf("enum signature = %q, want %q", typeNodes[0].Signature, "enum Status")
	}
}

func TestJavaExtractor_ExtractConstructors(t *testing.T) {
	ext := NewJavaExtractor()
	source := `public class User {
    private String name;

    public User(String name) {
        this.name = name;
    }
}
`
	opts := makeOpts("src/User.java", source)

	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	// Find the constructor node.
	methodNodes := filterNodesByKind(result.Nodes, "method")
	foundConstructor := false
	for _, n := range methodNodes {
		if containsName(n.QualifiedName, "<init>") {
			foundConstructor = true
			break
		}
	}
	if !foundConstructor {
		t.Errorf("missing constructor (<init>) node. Got nodes: %v", nodeNames(result.Nodes))
	}
}

func TestJavaExtractor_ExtractImports(t *testing.T) {
	ext := NewJavaExtractor()
	source := `import java.util.List;
import java.util.Map;
import org.springframework.web.bind.annotation.RestController;

public class App {
}
`
	opts := makeOpts("src/App.java", source)

	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	importEdges := filterEdgesByType(result.Edges, "imports")
	if len(importEdges) < 3 {
		t.Fatalf("expected at least 3 import edges, got %d", len(importEdges))
	}

	// All import edges should have ast_inferred provenance and 0.7 confidence.
	for _, e := range importEdges {
		if e.Provenance != "ast_inferred" {
			t.Errorf("import edge provenance = %q, want %q", e.Provenance, "ast_inferred")
		}
		if e.Confidence != 0.7 {
			t.Errorf("import edge confidence = %v, want 0.7", e.Confidence)
		}
	}
}

func TestJavaExtractor_ExtractMethodInvocations(t *testing.T) {
	ext := NewJavaExtractor()
	source := `public class Service {
    public void process() {
        helper.doWork();
        validate();
    }
}
`
	opts := makeOpts("src/Service.java", source)

	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	callEdges := filterEdgesByType(result.Edges, "calls")
	if len(callEdges) < 2 {
		t.Fatalf("expected at least 2 call edges, got %d", len(callEdges))
	}

	// Verify call-site positions are set.
	for _, e := range callEdges {
		if e.CallSiteLine == 0 {
			t.Error("call edge has zero CallSiteLine")
		}
		if e.CallSiteFile == "" {
			t.Error("call edge has empty CallSiteFile")
		}
		if e.CallSiteFile != "src/Service.java" {
			t.Errorf("call edge CallSiteFile = %q, want %q", e.CallSiteFile, "src/Service.java")
		}
	}
}

func TestJavaExtractor_SpringRoutes(t *testing.T) {
	ext := NewJavaExtractor()
	source := `import org.springframework.web.bind.annotation.*;

@RestController
@RequestMapping("/api/users")
public class UserController {
    private final UserService userService;

    public UserController(UserService userService) {
        this.userService = userService;
    }

    @GetMapping("/{id}")
    public User getUser(@PathVariable Long id) {
        return userService.findById(id);
    }

    @PostMapping
    public User createUser(@RequestBody User user) {
        return userService.save(user);
    }
}
`
	opts := makeOpts("src/UserController.java", source)

	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	// Should have route_handler nodes.
	routeNodes := filterNodesByKind(result.Nodes, "route_handler")
	if len(routeNodes) == 0 {
		t.Fatalf("expected route_handler nodes, got 0. All nodes: %v", nodeNames(result.Nodes))
	}

	// Should have handles_route edges.
	routeEdges := filterEdgesByType(result.Edges, "handles_route")
	if len(routeEdges) == 0 {
		t.Fatalf("expected handles_route edges, got 0")
	}

	// Verify provenance and confidence on route edges.
	for _, e := range routeEdges {
		if e.Provenance != "ast_inferred" {
			t.Errorf("route edge provenance = %q, want %q", e.Provenance, "ast_inferred")
		}
		if e.Confidence != 0.7 {
			t.Errorf("route edge confidence = %v, want 0.7", e.Confidence)
		}
	}
}

func TestJavaExtractor_NoRoutes(t *testing.T) {
	ext := NewJavaExtractor()
	source := `public class PlainService {
    public void doWork() {
        System.out.println("working");
    }
}
`
	opts := makeOpts("src/PlainService.java", source)

	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	routeNodes := filterNodesByKind(result.Nodes, "route_handler")
	if len(routeNodes) != 0 {
		t.Errorf("expected 0 route_handler nodes, got %d", len(routeNodes))
	}

	routeEdges := filterEdgesByType(result.Edges, "handles_route")
	if len(routeEdges) != 0 {
		t.Errorf("expected 0 handles_route edges, got %d", len(routeEdges))
	}
}

func TestJavaExtractor_EdgeProvenanceAndConfidence(t *testing.T) {
	ext := NewJavaExtractor()
	source := `import java.util.List;

public class App {
    public void run() {
        helper.process();
    }
}
`
	opts := makeOpts("src/App.java", source)

	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	for _, e := range result.Edges {
		if e.Provenance != "ast_inferred" {
			t.Errorf("edge provenance = %q, want %q", e.Provenance, "ast_inferred")
		}
		if e.Confidence != 0.7 {
			t.Errorf("edge confidence = %v, want 0.7", e.Confidence)
		}
	}
}

func TestJavaExtractor_EmptyFile(t *testing.T) {
	ext := NewJavaExtractor()
	source := `public class Empty {
}
`
	opts := makeOpts("src/Empty.java", source)

	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	// Should have at least the class node.
	if len(result.Nodes) == 0 {
		t.Error("expected at least 1 node for empty class, got 0")
	}

	classNodes := filterNodesByKind(result.Nodes, "type")
	if len(classNodes) != 1 {
		t.Fatalf("expected 1 type node, got %d", len(classNodes))
	}
	if !containsName(classNodes[0].QualifiedName, "Empty") {
		t.Errorf("class QualifiedName = %q, want to contain Empty", classNodes[0].QualifiedName)
	}
}

func TestJavaExtractor_ImportResolution(t *testing.T) {
	ext := NewJavaExtractor()
	source := `package com.example.app;

import com.example.service.UserService;
import com.example.util.StringHelper;

public class App {
    public void run() {
        UserService.findAll();
        StringHelper.format("test");
        localMethod();
    }
}
`
	opts := makeOpts("src/com/example/app/App.java", source)

	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	callEdges := filterEdgesByType(result.Edges, "calls")
	if len(callEdges) < 3 {
		t.Fatalf("expected at least 3 call edges, got %d", len(callEdges))
	}

	// Track which edges we found and their provenance/confidence.
	var foundUserService, foundStringHelper, foundLocalMethod bool
	for _, e := range callEdges {
		// Check edge provenance and confidence by looking at the target hash.
		// UserService.findAll should be ast_resolved with 0.85.
		if e.Provenance == "ast_resolved" && e.Confidence == 0.85 {
			if e.CallSiteLine > 0 {
				// This is one of the import-resolved edges.
				if !foundUserService {
					foundUserService = true
				} else {
					foundStringHelper = true
				}
			}
		}
		if e.Provenance == "ast_inferred" && e.Confidence == 0.7 {
			if e.CallSiteLine > 0 && e.EdgeType == "calls" {
				foundLocalMethod = true
			}
		}
	}

	// Verify import-resolved edges.
	resolvedEdges := filterEdgesByProvenance(callEdges, "ast_resolved")
	if len(resolvedEdges) < 2 {
		t.Errorf("expected at least 2 ast_resolved call edges (UserService.findAll, StringHelper.format), got %d", len(resolvedEdges))
		for _, e := range callEdges {
			t.Logf("  edge: provenance=%s confidence=%v line=%d", e.Provenance, e.Confidence, e.CallSiteLine)
		}
	}
	for _, e := range resolvedEdges {
		if e.Confidence != 0.85 {
			t.Errorf("ast_resolved edge has confidence %v, want 0.85", e.Confidence)
		}
	}

	// Verify unresolved edges (localMethod should remain ast_inferred).
	inferredCallEdges := filterEdgesByProvenance(callEdges, "ast_inferred")
	if len(inferredCallEdges) < 1 {
		t.Errorf("expected at least 1 ast_inferred call edge (localMethod), got %d", len(inferredCallEdges))
	}
	for _, e := range inferredCallEdges {
		if e.Confidence != 0.7 {
			t.Errorf("ast_inferred edge has confidence %v, want 0.7", e.Confidence)
		}
	}

	_ = foundUserService
	_ = foundStringHelper
	_ = foundLocalMethod
}

func TestJavaExtractor_ImportResolution_Static(t *testing.T) {
	ext := NewJavaExtractor()
	source := `package com.example.app;

import static com.example.util.MathHelper.calculate;

public class App {
    public void run() {
        MathHelper.doWork();
    }
}
`
	opts := makeOpts("src/com/example/app/App.java", source)

	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	callEdges := filterEdgesByType(result.Edges, "calls")
	if len(callEdges) < 1 {
		t.Fatalf("expected at least 1 call edge, got %d", len(callEdges))
	}

	// MathHelper should resolve through the static import map.
	resolvedEdges := filterEdgesByProvenance(callEdges, "ast_resolved")
	if len(resolvedEdges) < 1 {
		t.Errorf("expected at least 1 ast_resolved call edge (MathHelper.doWork), got %d", len(resolvedEdges))
		for _, e := range callEdges {
			t.Logf("  edge: provenance=%s confidence=%v line=%d", e.Provenance, e.Confidence, e.CallSiteLine)
		}
	}
	for _, e := range resolvedEdges {
		if e.Confidence != 0.85 {
			t.Errorf("ast_resolved edge has confidence %v, want 0.85", e.Confidence)
		}
	}
}

func TestJavaExtractor_ImportResolution_Wildcard(t *testing.T) {
	ext := NewJavaExtractor()
	source := `package com.example.app;

import com.example.model.*;

public class App {
    public void run() {
        User.create();
    }
}
`
	opts := makeOpts("src/com/example/app/App.java", source)

	result, err := ext.Extract(context.Background(), opts)
	if err != nil {
		t.Fatalf("Extract() error: %v", err)
	}

	callEdges := filterEdgesByType(result.Edges, "calls")
	if len(callEdges) < 1 {
		t.Fatalf("expected at least 1 call edge, got %d", len(callEdges))
	}

	// Wildcard imports should NOT resolve; call edge should be ast_inferred.
	resolvedEdges := filterEdgesByProvenance(callEdges, "ast_resolved")
	if len(resolvedEdges) != 0 {
		t.Errorf("expected 0 ast_resolved call edges for wildcard import, got %d", len(resolvedEdges))
		for _, e := range resolvedEdges {
			t.Logf("  resolved edge: provenance=%s confidence=%v line=%d", e.Provenance, e.Confidence, e.CallSiteLine)
		}
	}

	// Should have ast_inferred edges.
	inferredEdges := filterEdgesByProvenance(callEdges, "ast_inferred")
	if len(inferredEdges) < 1 {
		t.Errorf("expected at least 1 ast_inferred call edge (User.create), got %d", len(inferredEdges))
	}
	for _, e := range inferredEdges {
		if e.Confidence != 0.7 {
			t.Errorf("ast_inferred edge has confidence %v, want 0.7", e.Confidence)
		}
	}
}

// --- Helpers ---

func filterNodesByKind(nodes []types.Node, kind string) []types.Node {
	var result []types.Node
	for _, n := range nodes {
		if n.Kind == kind {
			result = append(result, n)
		}
	}
	return result
}

func filterEdgesByType(edges []types.Edge, edgeType string) []types.Edge {
	var result []types.Edge
	for _, e := range edges {
		if e.EdgeType == edgeType {
			result = append(result, e)
		}
	}
	return result
}

func filterEdgesByProvenance(edges []types.Edge, provenance string) []types.Edge {
	var result []types.Edge
	for _, e := range edges {
		if e.Provenance == provenance {
			result = append(result, e)
		}
	}
	return result
}

func containsName(qname, name string) bool {
	return len(qname) > 0 && len(name) > 0 && contains(qname, name)
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func nodeNames(nodes []types.Node) []string {
	var names []string
	for _, n := range nodes {
		names = append(names, n.Kind+":"+n.QualifiedName)
	}
	return names
}

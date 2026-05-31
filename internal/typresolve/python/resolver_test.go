package pyresolve

import (
	"context"
	"testing"

	"github.com/blackwell-systems/knowing/internal/edgetype"
	"github.com/blackwell-systems/knowing/internal/typresolve"
	"github.com/blackwell-systems/knowing/internal/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// parsePython is defined in eval_test.go (same package).

func TestPythonResolverLanguage(t *testing.T) {
	r := NewPythonResolver()
	assert.Equal(t, "python", r.Language())
}

func TestPythonResolverInitWorkspace(t *testing.T) {
	r := NewPythonResolver()
	defs := []typresolve.ResolverDef{
		{QualifiedName: "os.path.join", Kind: "function", PackagePath: "os.path"},
		{QualifiedName: "mymod.MyClass", Kind: "class", PackagePath: "mymod"},
		{QualifiedName: "mymod.MyClass.method", Kind: "method", PackagePath: "mymod"},
		{QualifiedName: "mymod.MyInterface", Kind: "interface", PackagePath: "mymod"},
	}

	err := r.InitWorkspace(context.Background(), defs)
	require.NoError(t, err)
	require.NotNil(t, r.registry)

	// Verify functions are registered.
	assert.NotNil(t, r.registry.LookupFunc("os.path.join"), "expected os.path.join in registry")

	// Verify types are registered.
	assert.NotNil(t, r.registry.LookupType("mymod.MyClass"), "expected mymod.MyClass in registry")

	// Verify interfaces are registered as types with IsInterface=true.
	tp := r.registry.LookupType("mymod.MyInterface")
	require.NotNil(t, tp, "expected mymod.MyInterface in registry")
	assert.True(t, tp.IsInterface)

	// Verify methods are registered.
	assert.NotNil(t, r.registry.LookupMethod("mymod.MyClass", "method"),
		"expected mymod.MyClass.method in registry")
}

func TestPythonResolverResolveFile_ImportCall(t *testing.T) {
	src := `from os.path import join

def process():
    result = join("a", "b")
`
	r := NewPythonResolver()
	defs := []typresolve.ResolverDef{
		{QualifiedName: "os.path.join", Kind: "function", PackagePath: "os.path"},
	}
	err := r.InitWorkspace(context.Background(), defs)
	require.NoError(t, err)

	content := []byte(src)
	edges, err := r.ResolveFile(context.Background(), typresolve.ResolveFileOpts{
		FilePath: "mymodule.py",
		FileHash: types.EmptyHash,
		Content:  content,
	})
	require.NoError(t, err)

	// Should produce a "calls" edge from process to os.path.join.
	require.NotEmpty(t, edges, "expected at least one edge")

	found := false
	for _, e := range edges {
		if e.EdgeType == edgetype.Calls {
			assert.Equal(t, typresolve.ProvenanceResolverResolved, e.Provenance)
			assert.Equal(t, typresolve.ResolverConfidence, e.Confidence)
			found = true
		}
	}
	assert.True(t, found, "expected a calls edge")
}

func TestPythonResolverResolveFile_LocalCalls(t *testing.T) {
	src := `def helper():
    pass

def main():
    helper()
`
	r := NewPythonResolver()
	defs := []typresolve.ResolverDef{
		{QualifiedName: "mymodule.helper", Kind: "function", PackagePath: "mymodule"},
	}
	err := r.InitWorkspace(context.Background(), defs)
	require.NoError(t, err)

	content := []byte(src)
	edges, err := r.ResolveFile(context.Background(), typresolve.ResolveFileOpts{
		FilePath: "mymodule.py",
		FileHash: types.EmptyHash,
		Content:  content,
	})
	require.NoError(t, err)

	// Should produce an edge for the helper() call.
	found := false
	for _, e := range edges {
		if e.EdgeType == edgetype.Calls {
			found = true
		}
	}
	assert.True(t, found, "expected a calls edge for helper()")
}

func TestPythonResolverResolveFile_MethodCalls(t *testing.T) {
	src := `class MyClass:
    def method(self):
        pass

def use_it(obj: MyClass):
    obj.method()
`
	r := NewPythonResolver()
	defs := []typresolve.ResolverDef{
		{QualifiedName: "mymodule.MyClass", Kind: "class", PackagePath: "mymodule"},
		{QualifiedName: "mymodule.MyClass.method", Kind: "method", PackagePath: "mymodule"},
	}
	err := r.InitWorkspace(context.Background(), defs)
	require.NoError(t, err)

	content := []byte(src)
	edges, err := r.ResolveFile(context.Background(), typresolve.ResolveFileOpts{
		FilePath: "mymodule.py",
		FileHash: types.EmptyHash,
		Content:  content,
	})
	require.NoError(t, err)

	// Should produce a method dispatch edge.
	found := false
	for _, e := range edges {
		if e.EdgeType == edgetype.Calls && e.Provenance == typresolve.ProvenanceResolverResolved {
			found = true
		}
	}
	assert.True(t, found, "expected a method dispatch edge for obj.method()")
}

func TestResolveCallsInFile_ScopeTracking(t *testing.T) {
	src := `def process():
    items = get_items()
    items.sort()
`
	reg := typresolve.NewRegistry()
	// Register get_items returning a list type.
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "mymodule.get_items",
		ShortName:     "get_items",
		Signature: typresolve.Func(nil, []*typresolve.Type{
			typresolve.Named("builtins.list"),
		}),
		MinParams: -1,
	})
	// Register list.sort method.
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "builtins.list.sort",
		ShortName:     "sort",
		ReceiverType:  "builtins.list",
		MinParams:     -1,
	})

	content := []byte(src)
	root := parsePython(t, src)

	rctx := &ResolveContext{
		Registry: reg,
		Scope:    typresolve.NewScope(nil),
		Imports:  make(map[string]ImportInfo),
		ModuleQN: "mymodule",
		Content:  content,
	}

	edges := ResolveCallsInFile(rctx, root, types.EmptyHash, "", "mymodule.py")

	// Should have at least get_items() call edge.
	assert.NotEmpty(t, edges, "expected edges from scope tracking")

	// Check we got a call edge.
	hasCall := false
	for _, e := range edges {
		if e.EdgeType == edgetype.Calls {
			hasCall = true
		}
	}
	assert.True(t, hasCall, "expected calls edge")
}

func TestResolveCallsInFile_ForLoop(t *testing.T) {
	src := `def process(items: list):
    for item in items:
        item.validate()
`
	reg := typresolve.NewRegistry()
	// Register a type with validate method.
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "mymodule.Item",
		ShortName:     "Item",
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "mymodule.Item.validate",
		ShortName:     "validate",
		ReceiverType:  "mymodule.Item",
		MinParams:     -1,
	})

	content := []byte(src)
	root := parsePython(t, src)

	rctx := &ResolveContext{
		Registry: reg,
		Scope:    typresolve.NewScope(nil),
		Imports:  make(map[string]ImportInfo),
		ModuleQN: "mymodule",
		Content:  content,
	}

	// This test verifies the for loop processes without crashing.
	// The item type from iterating a list is Unknown (since list is not
	// Slice with a known element type), so method dispatch won't resolve,
	// but the walk should complete without error.
	edges := ResolveCallsInFile(rctx, root, types.EmptyHash, "", "mymodule.py")
	_ = edges // no assertion on edge count; validates no crash
}

func TestBuildRegistry(t *testing.T) {
	defs := []typresolve.ResolverDef{
		{QualifiedName: "mymod.func1", Kind: "function"},
		{QualifiedName: "mymod.MyClass", Kind: "class"},
		{QualifiedName: "mymod.MyClass.method1", Kind: "method"},
		{QualifiedName: "mymod.MyInterface", Kind: "interface"},
	}

	reg := BuildRegistry(defs)
	require.NotNil(t, reg)

	assert.NotNil(t, reg.LookupFunc("mymod.func1"))
	assert.NotNil(t, reg.LookupType("mymod.MyClass"))
	assert.NotNil(t, reg.LookupMethod("mymod.MyClass", "method1"))

	iface := reg.LookupType("mymod.MyInterface")
	require.NotNil(t, iface)
	assert.True(t, iface.IsInterface)
}

func TestInferModuleQNFromPath(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"foo/bar/baz.py", "foo.bar.baz"},
		{"foo/bar/__init__.py", "foo.bar"},
		{"baz.py", "baz"},
		{"./foo/bar.py", "foo.bar"},
		{"__init__.py", ""},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := inferModuleQNFromPath(tt.path)
			assert.Equal(t, tt.want, got)
		})
	}
}

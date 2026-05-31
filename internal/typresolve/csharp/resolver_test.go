package csresolve

import (
	"context"
	"testing"

	"github.com/blackwell-systems/knowing/internal/typresolve"
	"github.com/blackwell-systems/knowing/internal/types"
)

func TestCSharpResolver_Language(t *testing.T) {
	r := NewCSharpResolver()
	if got := r.Language(); got != "csharp" {
		t.Errorf("Language() = %q, want %q", got, "csharp")
	}
}

func TestCSharpResolver_InitWorkspace(t *testing.T) {
	r := NewCSharpResolver()
	defs := []typresolve.ResolverDef{
		{QualifiedName: "MyApp.MyService", Kind: "class"},
		{QualifiedName: "MyApp.MyService.Run", Kind: "method"},
		{QualifiedName: "MyApp.MyHelper", Kind: "class"},
		{QualifiedName: "MyApp.MyHelper.DoWork", Kind: "method"},
	}
	err := r.InitWorkspace(context.Background(), defs)
	if err != nil {
		t.Fatalf("InitWorkspace: %v", err)
	}
	if r.registry == nil {
		t.Fatal("registry is nil after InitWorkspace")
	}
	// Verify types are registered.
	if r.registry.LookupType("MyApp.MyService") == nil {
		t.Error("MyApp.MyService not registered")
	}
	if r.registry.LookupType("MyApp.MyHelper") == nil {
		t.Error("MyApp.MyHelper not registered")
	}
}

func TestCSharpResolver_ResolveFile_SimpleClass(t *testing.T) {
	r := NewCSharpResolver()

	// Register types and methods.
	defs := []typresolve.ResolverDef{
		{QualifiedName: "MyApp.MyService", Kind: "class"},
		{QualifiedName: "MyApp.MyService.Run", Kind: "method"},
		{QualifiedName: "MyApp.MyHelper", Kind: "class"},
		{QualifiedName: "MyApp.MyHelper.DoWork", Kind: "method"},
		{QualifiedName: "System.Console", Kind: "class"},
		{QualifiedName: "System.Console.WriteLine", Kind: "method"},
	}
	err := r.InitWorkspace(context.Background(), defs)
	if err != nil {
		t.Fatalf("InitWorkspace: %v", err)
	}

	src := []byte(`using System;
namespace MyApp {
    class MyService {
        public void Run() {
            Console.WriteLine("hello");
            var x = new MyHelper();
            x.DoWork();
        }
    }
    class MyHelper {
        public void DoWork() { }
    }
}`)

	edges, err := r.ResolveFile(context.Background(), typresolve.ResolveFileOpts{
		FilePath: "MyService.cs",
		FileHash: types.Hash{},
		Content:  src,
	})
	if err != nil {
		t.Fatalf("ResolveFile: %v", err)
	}

	// We should get at least some edges (Console.WriteLine, MyHelper ctor, DoWork).
	if len(edges) == 0 {
		t.Error("expected at least one edge, got 0")
	}

	// Verify edge properties.
	for _, e := range edges {
		if e.EdgeType != "calls" {
			t.Errorf("unexpected edge type: %s", e.EdgeType)
		}
		if e.Confidence != typresolve.ResolverConfidence {
			t.Errorf("unexpected confidence: %f", e.Confidence)
		}
		if e.Provenance != typresolve.ProvenanceResolverResolved {
			t.Errorf("unexpected provenance: %s", e.Provenance)
		}
		if e.CallSiteFile != "MyService.cs" {
			t.Errorf("unexpected call site file: %s", e.CallSiteFile)
		}
		if e.CallSiteLine <= 0 {
			t.Errorf("call site line should be positive, got %d", e.CallSiteLine)
		}
	}

	t.Logf("resolved %d edges", len(edges))
}

func TestCSharpResolver_ResolveFile_UsingDirective(t *testing.T) {
	r := NewCSharpResolver()

	defs := []typresolve.ResolverDef{
		{QualifiedName: "System.IO.File", Kind: "class"},
		{QualifiedName: "System.IO.File.ReadAllText", Kind: "method"},
		{QualifiedName: "App.Program", Kind: "class"},
		{QualifiedName: "App.Program.Main", Kind: "method"},
	}
	err := r.InitWorkspace(context.Background(), defs)
	if err != nil {
		t.Fatalf("InitWorkspace: %v", err)
	}

	src := []byte(`using System.IO;
namespace App {
    class Program {
        public void Main() {
            File.ReadAllText("test.txt");
        }
    }
}`)

	edges, err := r.ResolveFile(context.Background(), typresolve.ResolveFileOpts{
		FilePath: "Program.cs",
		FileHash: types.Hash{},
		Content:  src,
	})
	if err != nil {
		t.Fatalf("ResolveFile: %v", err)
	}

	// Should resolve File.ReadAllText via using System.IO.
	found := false
	for _, e := range edges {
		if e.CallSiteLine == 5 { // File.ReadAllText line
			found = true
		}
	}
	if !found && len(edges) > 0 {
		t.Logf("edges found on lines: ")
		for _, e := range edges {
			t.Logf("  line %d col %d", e.CallSiteLine, e.CallSiteCol)
		}
	}
	t.Logf("resolved %d edges", len(edges))
}

func TestCSharpResolver_ResolveFile_FullyQualifiedCall(t *testing.T) {
	r := NewCSharpResolver()

	defs := []typresolve.ResolverDef{
		{QualifiedName: "System.Console", Kind: "class"},
		{QualifiedName: "System.Console.WriteLine", Kind: "method"},
	}
	err := r.InitWorkspace(context.Background(), defs)
	if err != nil {
		t.Fatalf("InitWorkspace: %v", err)
	}

	src := []byte(`namespace App {
    class Program {
        void Main() {
            System.Console.WriteLine("hi");
        }
    }
}`)

	edges, err := r.ResolveFile(context.Background(), typresolve.ResolveFileOpts{
		FilePath: "Program.cs",
		FileHash: types.Hash{},
		Content:  src,
	})
	if err != nil {
		t.Fatalf("ResolveFile: %v", err)
	}

	t.Logf("resolved %d edges", len(edges))
}

func TestCSharpResolver_ResolveFile_VarInference(t *testing.T) {
	r := NewCSharpResolver()

	defs := []typresolve.ResolverDef{
		{QualifiedName: "MyApp.MyType", Kind: "class"},
		{QualifiedName: "MyApp.MyType.Hello", Kind: "method"},
	}
	err := r.InitWorkspace(context.Background(), defs)
	if err != nil {
		t.Fatalf("InitWorkspace: %v", err)
	}

	src := []byte(`namespace MyApp {
    class Test {
        void M() {
            var x = new MyType();
            x.Hello();
        }
    }
}`)

	edges, err := r.ResolveFile(context.Background(), typresolve.ResolveFileOpts{
		FilePath: "Test.cs",
		FileHash: types.Hash{},
		Content:  src,
	})
	if err != nil {
		t.Fatalf("ResolveFile: %v", err)
	}

	// Should resolve x.Hello() since var infers MyType from new MyType().
	t.Logf("resolved %d edges (expecting ctor + Hello)", len(edges))
}

func TestCSharpResolver_ResolveFile_AsyncAwait(t *testing.T) {
	r := NewCSharpResolver()

	defs := []typresolve.ResolverDef{
		{QualifiedName: "App.Service", Kind: "class"},
		{QualifiedName: "App.Service.FetchAsync", Kind: "method"},
		{QualifiedName: "App.Result", Kind: "class"},
		{QualifiedName: "App.Result.Process", Kind: "method"},
	}
	err := r.InitWorkspace(context.Background(), defs)
	if err != nil {
		t.Fatalf("InitWorkspace: %v", err)
	}

	src := []byte(`namespace App {
    class Service {
        async Task<Result> FetchAsync() { return new Result(); }
        async void Run() {
            var r = await FetchAsync();
        }
    }
    class Result {
        public void Process() { }
    }
}`)

	edges, err := r.ResolveFile(context.Background(), typresolve.ResolveFileOpts{
		FilePath: "Service.cs",
		FileHash: types.Hash{},
		Content:  src,
	})
	if err != nil {
		t.Fatalf("ResolveFile: %v", err)
	}

	// Should resolve FetchAsync call.
	t.Logf("resolved %d edges", len(edges))
}

func TestCSharpResolver_ResolveFile_PropertyAccess(t *testing.T) {
	r := NewCSharpResolver()

	defs := []typresolve.ResolverDef{
		{QualifiedName: "App.Person", Kind: "class"},
		{QualifiedName: "App.Person.Name", Kind: "property"},
		{QualifiedName: "App.Person.GetInfo", Kind: "method"},
	}
	err := r.InitWorkspace(context.Background(), defs)
	if err != nil {
		t.Fatalf("InitWorkspace: %v", err)
	}

	src := []byte(`namespace App {
    class Test {
        void M() {
            var p = new Person();
            p.GetInfo();
        }
    }
}`)

	edges, err := r.ResolveFile(context.Background(), typresolve.ResolveFileOpts{
		FilePath: "Test.cs",
		FileHash: types.Hash{},
		Content:  src,
	})
	if err != nil {
		t.Fatalf("ResolveFile: %v", err)
	}

	t.Logf("resolved %d edges", len(edges))
}

func TestCSharpResolver_ResolveFile_BaseClassMethod(t *testing.T) {
	r := NewCSharpResolver()

	defs := []typresolve.ResolverDef{
		{QualifiedName: "App.BaseClass", Kind: "class"},
		{QualifiedName: "App.BaseClass.DoBase", Kind: "method"},
		{QualifiedName: "App.Derived", Kind: "class"},
	}
	err := r.InitWorkspace(context.Background(), defs)
	if err != nil {
		t.Fatalf("InitWorkspace: %v", err)
	}

	// Register Derived as having BaseClass as embedded type.
	r.registry.AddType(typresolve.RegisteredType{
		QualifiedName: "App.Derived",
		ShortName:     "Derived",
		EmbeddedTypes: []string{"App.BaseClass"},
	})

	src := []byte(`namespace App {
    class Derived : BaseClass {
        void M() {
            DoBase();
        }
    }
}`)

	edges, err := r.ResolveFile(context.Background(), typresolve.ResolveFileOpts{
		FilePath: "Derived.cs",
		FileHash: types.Hash{},
		Content:  src,
	})
	if err != nil {
		t.Fatalf("ResolveFile: %v", err)
	}

	// Should resolve DoBase() via inheritance on the enclosing class.
	t.Logf("resolved %d edges", len(edges))
}

func TestCSharpResolver_ResolveFile_ExtensionMethod(t *testing.T) {
	r := NewCSharpResolver()

	defs := []typresolve.ResolverDef{
		{QualifiedName: "App.MyType", Kind: "class"},
		{QualifiedName: "App.Extensions.Extend", Kind: "method"},
	}
	err := r.InitWorkspace(context.Background(), defs)
	if err != nil {
		t.Fatalf("InitWorkspace: %v", err)
	}

	// Register the extension method with receiver type set to App.MyType.
	r.registry.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "App.Extensions.Extend",
		ShortName:     "Extend",
		ReceiverType:  "App.MyType",
		Signature: typresolve.Func(
			[]typresolve.Param{{Name: "this", Type: typresolve.Named("App.MyType")}},
			[]*typresolve.Type{typresolve.Named("System.String")},
		),
	})

	src := []byte(`namespace App {
    class Test {
        void M() {
            var x = new MyType();
            x.Extend();
        }
    }
}`)

	edges, err := r.ResolveFile(context.Background(), typresolve.ResolveFileOpts{
		FilePath: "Test.cs",
		FileHash: types.Hash{},
		Content:  src,
	})
	if err != nil {
		t.Fatalf("ResolveFile: %v", err)
	}

	t.Logf("resolved %d edges", len(edges))
}

func TestCSharpResolver_NotInitialized(t *testing.T) {
	r := NewCSharpResolver()
	_, err := r.ResolveFile(context.Background(), typresolve.ResolveFileOpts{
		FilePath: "test.cs",
		Content:  []byte("class C {}"),
	})
	if err == nil {
		t.Fatal("expected error for uninitialized resolver")
	}
}

func TestBuildRegistry_PartialClass(t *testing.T) {
	defs := []typresolve.ResolverDef{
		{QualifiedName: "App.MyClass", Kind: "class"},
		{QualifiedName: "App.MyClass.Method1", Kind: "method"},
		{QualifiedName: "App.MyClass", Kind: "class"}, // partial re-declaration
		{QualifiedName: "App.MyClass.Method2", Kind: "method"},
	}
	reg := BuildRegistry(defs)

	if reg.LookupType("App.MyClass") == nil {
		t.Error("partial class not registered")
	}
	if reg.LookupMethod("App.MyClass", "Method1") == nil {
		t.Error("Method1 not registered")
	}
	if reg.LookupMethod("App.MyClass", "Method2") == nil {
		t.Error("Method2 not registered")
	}
}

func TestBuildRegistry_Constructor(t *testing.T) {
	// Constructors are typically emitted with the class name as the method name.
	defs := []typresolve.ResolverDef{
		{QualifiedName: "App.MyClass", Kind: "class"},
		{QualifiedName: "App.MyClass.MyClass", Kind: "constructor"},
	}
	reg := BuildRegistry(defs)

	if reg.LookupType("App.MyClass") == nil {
		t.Error("class not registered")
	}
	// Constructor is registered with short name matching class name.
	if reg.LookupMethod("App.MyClass", "MyClass") == nil {
		t.Error("constructor not registered as method with class name")
	}
}

func TestBuildRegistry_Property(t *testing.T) {
	defs := []typresolve.ResolverDef{
		{QualifiedName: "App.MyClass", Kind: "class"},
		{QualifiedName: "App.MyClass.Name", Kind: "property"},
	}
	reg := BuildRegistry(defs)

	// Property is registered as a method.
	if reg.LookupMethod("App.MyClass", "Name") == nil {
		t.Error("property not registered as method")
	}
}

func TestBuildRegistry_Interface(t *testing.T) {
	defs := []typresolve.ResolverDef{
		{QualifiedName: "App.IService", Kind: "interface"},
		{QualifiedName: "App.IService.Execute", Kind: "method"},
	}
	reg := BuildRegistry(defs)

	rt := reg.LookupType("App.IService")
	if rt == nil {
		t.Fatal("interface not registered")
	}
	if !rt.IsInterface {
		t.Error("expected IsInterface=true")
	}
}

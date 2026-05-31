package csresolve

import (
	"testing"

	"github.com/blackwell-systems/knowing/internal/typresolve"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- PredefinedAlias tests ---

func TestPredefinedAlias(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"int", "System.Int32"},
		{"uint", "System.UInt32"},
		{"long", "System.Int64"},
		{"ulong", "System.UInt64"},
		{"short", "System.Int16"},
		{"ushort", "System.UInt16"},
		{"byte", "System.Byte"},
		{"sbyte", "System.SByte"},
		{"float", "System.Single"},
		{"double", "System.Double"},
		{"decimal", "System.Decimal"},
		{"bool", "System.Boolean"},
		{"char", "System.Char"},
		{"string", "System.String"},
		{"object", "System.Object"},
		{"nint", "System.IntPtr"},
		{"nuint", "System.UIntPtr"},
		{"void", "System.Void"},
		{"dynamic", "System.Object"},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got := PredefinedAlias(tc.input)
			assert.Equal(t, tc.want, got)
		})
	}

	// Non-aliases return empty string.
	assert.Equal(t, "", PredefinedAlias("MyClass"))
	assert.Equal(t, "", PredefinedAlias(""))
	assert.Equal(t, "", PredefinedAlias("System.Int32"))
}

// --- IsBuiltinFunc tests ---

func TestIsBuiltinFunc(t *testing.T) {
	trueNames := []string{"typeof", "nameof", "sizeof", "default", "checked", "unchecked", "stackalloc"}
	for _, name := range trueNames {
		assert.True(t, IsBuiltinFunc(name), "expected %q to be a builtin func", name)
	}

	falseNames := []string{"Console", "Math", "new", "return", "if", ""}
	for _, name := range falseNames {
		assert.False(t, IsBuiltinFunc(name), "expected %q to NOT be a builtin func", name)
	}
}

// --- IsKeywordSelf tests ---

func TestIsKeywordSelf(t *testing.T) {
	assert.True(t, IsKeywordSelf("this"))
	assert.True(t, IsKeywordSelf("base"))
	assert.False(t, IsKeywordSelf("self"))
	assert.False(t, IsKeywordSelf(""))
	assert.False(t, IsKeywordSelf("This"))
}

// --- UnwrapTask tests ---

func TestUnwrapTask(t *testing.T) {
	// Task types should unwrap to Unknown.
	taskType := typresolve.Named("System.Threading.Tasks.Task")
	got := UnwrapTask(taskType)
	require.NotNil(t, got)
	assert.Equal(t, typresolve.KindUnknown, got.Kind)

	// ValueTask types should unwrap to Unknown.
	valueTaskType := typresolve.Named("System.Threading.Tasks.ValueTask")
	got = UnwrapTask(valueTaskType)
	require.NotNil(t, got)
	assert.Equal(t, typresolve.KindUnknown, got.Kind)

	// Generic Task<T> (name-based heuristic).
	genericTask := typresolve.Named("System.Threading.Tasks.Task<System.String>")
	got = UnwrapTask(genericTask)
	require.NotNil(t, got)
	assert.Equal(t, typresolve.KindUnknown, got.Kind)

	// Non-task types should pass through unchanged.
	normalType := typresolve.Named("MyApp.MyService")
	got = UnwrapTask(normalType)
	assert.Equal(t, "MyApp.MyService", got.Name)

	// Nil returns nil.
	assert.Nil(t, UnwrapTask(nil))

	// Non-named types pass through.
	sliceType := typresolve.Slice(typresolve.Named("System.Int32"))
	got = UnwrapTask(sliceType)
	assert.Equal(t, typresolve.KindSlice, got.Kind)
}

// --- UnwrapNullable tests ---

func TestUnwrapNullable(t *testing.T) {
	nullableType := typresolve.Named("System.Nullable")
	got := UnwrapNullable(nullableType)
	require.NotNil(t, got)
	assert.Equal(t, typresolve.KindUnknown, got.Kind)

	genericNullable := typresolve.Named("System.Nullable<System.Int32>")
	got = UnwrapNullable(genericNullable)
	require.NotNil(t, got)
	assert.Equal(t, typresolve.KindUnknown, got.Kind)

	// Non-nullable passes through.
	normalType := typresolve.Named("System.Int32")
	got = UnwrapNullable(normalType)
	assert.Equal(t, "System.Int32", got.Name)
}

// --- LookupMethod tests ---

func TestLookupMethod_Direct(t *testing.T) {
	reg := typresolve.NewRegistry()
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "MyApp.MyService",
		ShortName:     "MyService",
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "MyApp.MyService.DoWork",
		ReceiverType:  "MyApp.MyService",
		ShortName:     "DoWork",
	})

	f := LookupMethod(reg, "MyApp.MyService", "DoWork")
	require.NotNil(t, f)
	assert.Equal(t, "MyApp.MyService.DoWork", f.QualifiedName)
}

func TestLookupMethod_Inherited(t *testing.T) {
	reg := typresolve.NewRegistry()
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "MyApp.BaseClass",
		ShortName:     "BaseClass",
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "MyApp.BaseClass.BaseMethod",
		ReceiverType:  "MyApp.BaseClass",
		ShortName:     "BaseMethod",
	})
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "MyApp.ChildClass",
		ShortName:     "ChildClass",
		EmbeddedTypes: []string{"MyApp.BaseClass"},
	})

	f := LookupMethod(reg, "MyApp.ChildClass", "BaseMethod")
	require.NotNil(t, f)
	assert.Equal(t, "MyApp.BaseClass.BaseMethod", f.QualifiedName)
}

func TestLookupMethod_DeepInheritance(t *testing.T) {
	reg := typresolve.NewRegistry()
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "MyApp.GrandBase",
		ShortName:     "GrandBase",
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "MyApp.GrandBase.DeepMethod",
		ReceiverType:  "MyApp.GrandBase",
		ShortName:     "DeepMethod",
	})
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "MyApp.MiddleClass",
		ShortName:     "MiddleClass",
		EmbeddedTypes: []string{"MyApp.GrandBase"},
	})
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "MyApp.LeafClass",
		ShortName:     "LeafClass",
		EmbeddedTypes: []string{"MyApp.MiddleClass"},
	})

	f := LookupMethod(reg, "MyApp.LeafClass", "DeepMethod")
	require.NotNil(t, f)
	assert.Equal(t, "MyApp.GrandBase.DeepMethod", f.QualifiedName)
}

func TestLookupMethod_SystemObjectFallback(t *testing.T) {
	reg := typresolve.NewRegistry()
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "MyApp.MyClass",
		ShortName:     "MyClass",
	})

	// ToString should fall back to System.Object.
	f := LookupMethod(reg, "MyApp.MyClass", "ToString")
	require.NotNil(t, f)
	assert.Equal(t, "System.Object.ToString", f.QualifiedName)

	f = LookupMethod(reg, "MyApp.MyClass", "GetHashCode")
	require.NotNil(t, f)
	assert.Equal(t, "System.Object.GetHashCode", f.QualifiedName)
}

func TestLookupMethod_NotFound(t *testing.T) {
	reg := typresolve.NewRegistry()
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "MyApp.MyClass",
		ShortName:     "MyClass",
	})

	f := LookupMethod(reg, "MyApp.MyClass", "NonExistent")
	assert.Nil(t, f)
}

func TestLookupMethod_CycleDetection(t *testing.T) {
	// Create a cycle: A embeds B, B embeds A.
	reg := typresolve.NewRegistry()
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "MyApp.A",
		ShortName:     "A",
		EmbeddedTypes: []string{"MyApp.B"},
	})
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "MyApp.B",
		ShortName:     "B",
		EmbeddedTypes: []string{"MyApp.A"},
	})

	// Should not hang; should return nil for non-existent method.
	f := LookupMethod(reg, "MyApp.A", "NonExistent")
	assert.Nil(t, f)
}

// --- LookupField tests ---

func TestLookupField_Direct(t *testing.T) {
	reg := typresolve.NewRegistry()
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "MyApp.Person",
		ShortName:     "Person",
		Fields: []typresolve.Field{
			{Name: "Name", Type: typresolve.Named("System.String")},
			{Name: "Age", Type: typresolve.Named("System.Int32")},
		},
	})

	ft := LookupField(reg, "MyApp.Person", "Name")
	require.NotNil(t, ft)
	assert.Equal(t, "System.String", ft.Name)

	ft = LookupField(reg, "MyApp.Person", "Age")
	require.NotNil(t, ft)
	assert.Equal(t, "System.Int32", ft.Name)
}

func TestLookupField_Inherited(t *testing.T) {
	reg := typresolve.NewRegistry()
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "MyApp.BaseEntity",
		ShortName:     "BaseEntity",
		Fields: []typresolve.Field{
			{Name: "Id", Type: typresolve.Named("System.Int32")},
		},
	})
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "MyApp.User",
		ShortName:     "User",
		EmbeddedTypes: []string{"MyApp.BaseEntity"},
		Fields: []typresolve.Field{
			{Name: "Email", Type: typresolve.Named("System.String")},
		},
	})

	// Direct field.
	ft := LookupField(reg, "MyApp.User", "Email")
	require.NotNil(t, ft)
	assert.Equal(t, "System.String", ft.Name)

	// Inherited field.
	ft = LookupField(reg, "MyApp.User", "Id")
	require.NotNil(t, ft)
	assert.Equal(t, "System.Int32", ft.Name)
}

func TestLookupField_NotFound(t *testing.T) {
	reg := typresolve.NewRegistry()
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "MyApp.MyClass",
		ShortName:     "MyClass",
	})

	ft := LookupField(reg, "MyApp.MyClass", "NonExistent")
	assert.Nil(t, ft)
}

// --- LookupExtensionMethod tests ---

func TestLookupExtensionMethod_Found(t *testing.T) {
	reg := typresolve.NewRegistry()
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "System.String",
		ShortName:     "String",
	})
	// Register an extension method with the receiver as ReceiverType.
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "MyApp.StringExtensions.ToSlug",
		ReceiverType:  "System.String",
		ShortName:     "ToSlug",
		Signature: typresolve.Func(
			[]typresolve.Param{{Name: "this source", Type: typresolve.Named("System.String")}},
			[]*typresolve.Type{typresolve.Named("System.String")},
		),
	})

	f := LookupExtensionMethod(reg, "System.String", "ToSlug")
	require.NotNil(t, f)
	assert.Equal(t, "MyApp.StringExtensions.ToSlug", f.QualifiedName)
}

func TestLookupExtensionMethod_ViaBaseType(t *testing.T) {
	reg := typresolve.NewRegistry()
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "System.Collections.IEnumerable",
		ShortName:     "IEnumerable",
	})
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "System.Collections.Generic.List",
		ShortName:     "List",
		EmbeddedTypes: []string{"System.Collections.IEnumerable"},
	})
	// Extension on IEnumerable.
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "System.Linq.Enumerable.Where",
		ReceiverType:  "System.Collections.IEnumerable",
		ShortName:     "Where",
		Signature: typresolve.Func(
			[]typresolve.Param{{Name: "this source", Type: typresolve.Named("System.Collections.IEnumerable")}},
			[]*typresolve.Type{typresolve.Named("System.Collections.IEnumerable")},
		),
	})

	// Should find via base type walk.
	f := LookupExtensionMethod(reg, "System.Collections.Generic.List", "Where")
	require.NotNil(t, f)
	assert.Equal(t, "System.Linq.Enumerable.Where", f.QualifiedName)
}

func TestLookupExtensionMethod_NotFound(t *testing.T) {
	reg := typresolve.NewRegistry()
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "MyApp.MyClass",
		ShortName:     "MyClass",
	})

	f := LookupExtensionMethod(reg, "MyApp.MyClass", "NonExistent")
	assert.Nil(t, f)
}

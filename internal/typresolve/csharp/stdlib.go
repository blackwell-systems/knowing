package csresolve

import "github.com/blackwell-systems/knowing/internal/typresolve"

// RegisterCSharpStdlib registers core System types and their methods/fields
// into the given registry. This enables the resolver to track return types
// through stdlib call chains (e.g. Console.WriteLine, String.Contains,
// List<T>.Add, Task<T>.Result).
//
// Port of the implicit stdlib knowledge from cs_lsp.c: the C reference
// registers these at runtime via the registry builder; we pre-seed them
// so resolution works even without LSP enrichment.
func RegisterCSharpStdlib(reg *typresolve.Registry) {
	// --- System.Object ---
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "System.Object",
		ShortName:     "Object",
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "System.Object.ToString",
		ShortName:     "ToString",
		ReceiverType:  "System.Object",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Named("System.String")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "System.Object.Equals",
		ShortName:     "Equals",
		ReceiverType:  "System.Object",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Named("System.Boolean")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "System.Object.GetHashCode",
		ShortName:     "GetHashCode",
		ReceiverType:  "System.Object",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Named("System.Int32")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "System.Object.GetType",
		ShortName:     "GetType",
		ReceiverType:  "System.Object",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Named("System.Type")}),
	})

	// --- System.String ---
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "System.String",
		ShortName:     "String",
		EmbeddedTypes: []string{"System.Object"},
		Fields: []typresolve.Field{
			{Name: "Length", Type: typresolve.Named("System.Int32")},
		},
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "System.String.Contains",
		ShortName:     "Contains",
		ReceiverType:  "System.String",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Named("System.Boolean")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "System.String.StartsWith",
		ShortName:     "StartsWith",
		ReceiverType:  "System.String",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Named("System.Boolean")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "System.String.EndsWith",
		ShortName:     "EndsWith",
		ReceiverType:  "System.String",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Named("System.Boolean")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "System.String.Substring",
		ShortName:     "Substring",
		ReceiverType:  "System.String",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Named("System.String")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "System.String.Replace",
		ShortName:     "Replace",
		ReceiverType:  "System.String",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Named("System.String")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "System.String.Trim",
		ShortName:     "Trim",
		ReceiverType:  "System.String",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Named("System.String")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "System.String.TrimStart",
		ShortName:     "TrimStart",
		ReceiverType:  "System.String",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Named("System.String")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "System.String.TrimEnd",
		ShortName:     "TrimEnd",
		ReceiverType:  "System.String",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Named("System.String")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "System.String.ToLower",
		ShortName:     "ToLower",
		ReceiverType:  "System.String",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Named("System.String")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "System.String.ToUpper",
		ShortName:     "ToUpper",
		ReceiverType:  "System.String",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Named("System.String")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "System.String.Split",
		ShortName:     "Split",
		ReceiverType:  "System.String",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Slice(typresolve.Named("System.String"))}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "System.String.IndexOf",
		ShortName:     "IndexOf",
		ReceiverType:  "System.String",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Named("System.Int32")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "System.String.Format",
		ShortName:     "Format",
		ReceiverType:  "System.String",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Named("System.String")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "System.String.Join",
		ShortName:     "Join",
		ReceiverType:  "System.String",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Named("System.String")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "System.String.IsNullOrEmpty",
		ShortName:     "IsNullOrEmpty",
		ReceiverType:  "System.String",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Named("System.Boolean")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "System.String.IsNullOrWhiteSpace",
		ShortName:     "IsNullOrWhiteSpace",
		ReceiverType:  "System.String",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Named("System.Boolean")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "System.String.Concat",
		ShortName:     "Concat",
		ReceiverType:  "System.String",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Named("System.String")}),
	})

	// --- System.Int32 ---
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "System.Int32",
		ShortName:     "Int32",
		EmbeddedTypes: []string{"System.Object"},
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "System.Int32.Parse",
		ShortName:     "Parse",
		ReceiverType:  "System.Int32",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Named("System.Int32")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "System.Int32.TryParse",
		ShortName:     "TryParse",
		ReceiverType:  "System.Int32",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Named("System.Boolean")}),
	})

	// --- System.Int64 ---
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "System.Int64",
		ShortName:     "Int64",
		EmbeddedTypes: []string{"System.Object"},
	})

	// --- System.Boolean ---
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "System.Boolean",
		ShortName:     "Boolean",
		EmbeddedTypes: []string{"System.Object"},
	})

	// --- System.Double ---
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "System.Double",
		ShortName:     "Double",
		EmbeddedTypes: []string{"System.Object"},
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "System.Double.Parse",
		ShortName:     "Parse",
		ReceiverType:  "System.Double",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Named("System.Double")}),
	})

	// --- System.Single ---
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "System.Single",
		ShortName:     "Single",
		EmbeddedTypes: []string{"System.Object"},
	})

	// --- System.Decimal ---
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "System.Decimal",
		ShortName:     "Decimal",
		EmbeddedTypes: []string{"System.Object"},
	})

	// --- System.Char ---
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "System.Char",
		ShortName:     "Char",
		EmbeddedTypes: []string{"System.Object"},
	})

	// --- System.Byte ---
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "System.Byte",
		ShortName:     "Byte",
		EmbeddedTypes: []string{"System.Object"},
	})

	// --- System.Type ---
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "System.Type",
		ShortName:     "Type",
		EmbeddedTypes: []string{"System.Object"},
		Fields: []typresolve.Field{
			{Name: "Name", Type: typresolve.Named("System.String")},
			{Name: "FullName", Type: typresolve.Named("System.String")},
			{Name: "Namespace", Type: typresolve.Named("System.String")},
		},
	})

	// --- System.Console ---
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "System.Console",
		ShortName:     "Console",
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "System.Console.WriteLine",
		ShortName:     "WriteLine",
		ReceiverType:  "System.Console",
		Signature:     typresolve.Func(nil, nil),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "System.Console.Write",
		ShortName:     "Write",
		ReceiverType:  "System.Console",
		Signature:     typresolve.Func(nil, nil),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "System.Console.ReadLine",
		ShortName:     "ReadLine",
		ReceiverType:  "System.Console",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Named("System.String")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "System.Console.ReadKey",
		ShortName:     "ReadKey",
		ReceiverType:  "System.Console",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Named("System.ConsoleKeyInfo")}),
	})

	// --- System.Math ---
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "System.Math",
		ShortName:     "Math",
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "System.Math.Abs",
		ShortName:     "Abs",
		ReceiverType:  "System.Math",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Named("System.Double")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "System.Math.Max",
		ShortName:     "Max",
		ReceiverType:  "System.Math",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Named("System.Double")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "System.Math.Min",
		ShortName:     "Min",
		ReceiverType:  "System.Math",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Named("System.Double")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "System.Math.Sqrt",
		ShortName:     "Sqrt",
		ReceiverType:  "System.Math",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Named("System.Double")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "System.Math.Round",
		ShortName:     "Round",
		ReceiverType:  "System.Math",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Named("System.Double")}),
	})

	// --- System.Convert ---
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "System.Convert",
		ShortName:     "Convert",
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "System.Convert.ToInt32",
		ShortName:     "ToInt32",
		ReceiverType:  "System.Convert",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Named("System.Int32")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "System.Convert.ToString",
		ShortName:     "ToString",
		ReceiverType:  "System.Convert",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Named("System.String")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "System.Convert.ToBoolean",
		ShortName:     "ToBoolean",
		ReceiverType:  "System.Convert",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Named("System.Boolean")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "System.Convert.ToDouble",
		ShortName:     "ToDouble",
		ReceiverType:  "System.Convert",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Named("System.Double")}),
	})

	// --- System.Collections.Generic.List<T> ---
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "System.Collections.Generic.List",
		ShortName:     "List",
		TypeParams:    []string{"T"},
		EmbeddedTypes: []string{"System.Collections.Generic.IEnumerable", "System.Object"},
		Fields: []typresolve.Field{
			{Name: "Count", Type: typresolve.Named("System.Int32")},
		},
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "System.Collections.Generic.List.Add",
		ShortName:     "Add",
		ReceiverType:  "System.Collections.Generic.List",
		Signature:     typresolve.Func(nil, nil),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "System.Collections.Generic.List.Remove",
		ShortName:     "Remove",
		ReceiverType:  "System.Collections.Generic.List",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Named("System.Boolean")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "System.Collections.Generic.List.Contains",
		ShortName:     "Contains",
		ReceiverType:  "System.Collections.Generic.List",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Named("System.Boolean")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "System.Collections.Generic.List.Clear",
		ShortName:     "Clear",
		ReceiverType:  "System.Collections.Generic.List",
		Signature:     typresolve.Func(nil, nil),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "System.Collections.Generic.List.ToArray",
		ShortName:     "ToArray",
		ReceiverType:  "System.Collections.Generic.List",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Slice(typresolve.Named("System.Object"))}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "System.Collections.Generic.List.Find",
		ShortName:     "Find",
		ReceiverType:  "System.Collections.Generic.List",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.TypeParamType("T")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "System.Collections.Generic.List.FindAll",
		ShortName:     "FindAll",
		ReceiverType:  "System.Collections.Generic.List",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Named("System.Collections.Generic.List")}),
	})

	// --- System.Collections.Generic.Dictionary<TKey, TValue> ---
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "System.Collections.Generic.Dictionary",
		ShortName:     "Dictionary",
		TypeParams:    []string{"TKey", "TValue"},
		EmbeddedTypes: []string{"System.Object"},
		Fields: []typresolve.Field{
			{Name: "Count", Type: typresolve.Named("System.Int32")},
			{Name: "Keys", Type: typresolve.Named("System.Collections.Generic.IEnumerable")},
			{Name: "Values", Type: typresolve.Named("System.Collections.Generic.IEnumerable")},
		},
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "System.Collections.Generic.Dictionary.Add",
		ShortName:     "Add",
		ReceiverType:  "System.Collections.Generic.Dictionary",
		Signature:     typresolve.Func(nil, nil),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "System.Collections.Generic.Dictionary.ContainsKey",
		ShortName:     "ContainsKey",
		ReceiverType:  "System.Collections.Generic.Dictionary",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Named("System.Boolean")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "System.Collections.Generic.Dictionary.ContainsValue",
		ShortName:     "ContainsValue",
		ReceiverType:  "System.Collections.Generic.Dictionary",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Named("System.Boolean")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "System.Collections.Generic.Dictionary.TryGetValue",
		ShortName:     "TryGetValue",
		ReceiverType:  "System.Collections.Generic.Dictionary",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Named("System.Boolean")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "System.Collections.Generic.Dictionary.Remove",
		ShortName:     "Remove",
		ReceiverType:  "System.Collections.Generic.Dictionary",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Named("System.Boolean")}),
	})

	// --- System.Collections.Generic.IEnumerable<T> ---
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "System.Collections.Generic.IEnumerable",
		ShortName:     "IEnumerable",
		TypeParams:    []string{"T"},
		IsInterface:   true,
	})

	// --- System.Collections.Generic.IList<T> ---
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "System.Collections.Generic.IList",
		ShortName:     "IList",
		TypeParams:    []string{"T"},
		IsInterface:   true,
		EmbeddedTypes: []string{"System.Collections.Generic.IEnumerable"},
	})

	// --- System.Collections.Generic.HashSet<T> ---
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "System.Collections.Generic.HashSet",
		ShortName:     "HashSet",
		TypeParams:    []string{"T"},
		EmbeddedTypes: []string{"System.Collections.Generic.IEnumerable", "System.Object"},
		Fields: []typresolve.Field{
			{Name: "Count", Type: typresolve.Named("System.Int32")},
		},
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "System.Collections.Generic.HashSet.Add",
		ShortName:     "Add",
		ReceiverType:  "System.Collections.Generic.HashSet",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Named("System.Boolean")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "System.Collections.Generic.HashSet.Contains",
		ShortName:     "Contains",
		ReceiverType:  "System.Collections.Generic.HashSet",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Named("System.Boolean")}),
	})

	// --- System.Threading.Tasks.Task ---
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "System.Threading.Tasks.Task",
		ShortName:     "Task",
		TypeParams:    []string{"TResult"},
		EmbeddedTypes: []string{"System.Object"},
		Fields: []typresolve.Field{
			{Name: "Result", Type: typresolve.TypeParamType("TResult")},
			{Name: "IsCompleted", Type: typresolve.Named("System.Boolean")},
			{Name: "IsFaulted", Type: typresolve.Named("System.Boolean")},
			{Name: "Status", Type: typresolve.Named("System.Threading.Tasks.TaskStatus")},
		},
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "System.Threading.Tasks.Task.Run",
		ShortName:     "Run",
		ReceiverType:  "System.Threading.Tasks.Task",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Named("System.Threading.Tasks.Task")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "System.Threading.Tasks.Task.WhenAll",
		ShortName:     "WhenAll",
		ReceiverType:  "System.Threading.Tasks.Task",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Named("System.Threading.Tasks.Task")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "System.Threading.Tasks.Task.WhenAny",
		ShortName:     "WhenAny",
		ReceiverType:  "System.Threading.Tasks.Task",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Named("System.Threading.Tasks.Task")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "System.Threading.Tasks.Task.Delay",
		ShortName:     "Delay",
		ReceiverType:  "System.Threading.Tasks.Task",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Named("System.Threading.Tasks.Task")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "System.Threading.Tasks.Task.FromResult",
		ShortName:     "FromResult",
		ReceiverType:  "System.Threading.Tasks.Task",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Named("System.Threading.Tasks.Task")}),
	})

	// --- System.Linq.Enumerable (extension methods on IEnumerable) ---
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "System.Linq.Enumerable",
		ShortName:     "Enumerable",
	})
	// LINQ extension methods registered on IEnumerable for member access resolution.
	linqMethods := []struct {
		name    string
		retType *typresolve.Type
	}{
		{"Where", typresolve.Named("System.Collections.Generic.IEnumerable")},
		{"Select", typresolve.Named("System.Collections.Generic.IEnumerable")},
		{"SelectMany", typresolve.Named("System.Collections.Generic.IEnumerable")},
		{"OrderBy", typresolve.Named("System.Collections.Generic.IEnumerable")},
		{"OrderByDescending", typresolve.Named("System.Collections.Generic.IEnumerable")},
		{"ThenBy", typresolve.Named("System.Collections.Generic.IEnumerable")},
		{"GroupBy", typresolve.Named("System.Collections.Generic.IEnumerable")},
		{"First", typresolve.TypeParamType("T")},
		{"FirstOrDefault", typresolve.TypeParamType("T")},
		{"Last", typresolve.TypeParamType("T")},
		{"LastOrDefault", typresolve.TypeParamType("T")},
		{"Single", typresolve.TypeParamType("T")},
		{"SingleOrDefault", typresolve.TypeParamType("T")},
		{"Any", typresolve.Named("System.Boolean")},
		{"All", typresolve.Named("System.Boolean")},
		{"Count", typresolve.Named("System.Int32")},
		{"Sum", typresolve.Named("System.Int32")},
		{"Average", typresolve.Named("System.Double")},
		{"Min", typresolve.TypeParamType("T")},
		{"Max", typresolve.TypeParamType("T")},
		{"ToList", typresolve.Named("System.Collections.Generic.List")},
		{"ToArray", typresolve.Slice(typresolve.Named("System.Object"))},
		{"ToDictionary", typresolve.Named("System.Collections.Generic.Dictionary")},
		{"Distinct", typresolve.Named("System.Collections.Generic.IEnumerable")},
		{"Take", typresolve.Named("System.Collections.Generic.IEnumerable")},
		{"Skip", typresolve.Named("System.Collections.Generic.IEnumerable")},
		{"Concat", typresolve.Named("System.Collections.Generic.IEnumerable")},
		{"Zip", typresolve.Named("System.Collections.Generic.IEnumerable")},
		{"Aggregate", typresolve.TypeParamType("T")},
	}
	for _, m := range linqMethods {
		reg.AddFunc(typresolve.RegisteredFunc{
			QualifiedName: "System.Collections.Generic.IEnumerable." + m.name,
			ShortName:     m.name,
			ReceiverType:  "System.Collections.Generic.IEnumerable",
			Signature:     typresolve.Func(nil, []*typresolve.Type{m.retType}),
		})
	}

	// --- System.IO.File ---
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "System.IO.File",
		ShortName:     "File",
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "System.IO.File.ReadAllText",
		ShortName:     "ReadAllText",
		ReceiverType:  "System.IO.File",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Named("System.String")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "System.IO.File.WriteAllText",
		ShortName:     "WriteAllText",
		ReceiverType:  "System.IO.File",
		Signature:     typresolve.Func(nil, nil),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "System.IO.File.Exists",
		ShortName:     "Exists",
		ReceiverType:  "System.IO.File",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Named("System.Boolean")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "System.IO.File.ReadAllLines",
		ShortName:     "ReadAllLines",
		ReceiverType:  "System.IO.File",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Slice(typresolve.Named("System.String"))}),
	})

	// --- System.IO.Path ---
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "System.IO.Path",
		ShortName:     "Path",
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "System.IO.Path.Combine",
		ShortName:     "Combine",
		ReceiverType:  "System.IO.Path",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Named("System.String")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "System.IO.Path.GetFileName",
		ShortName:     "GetFileName",
		ReceiverType:  "System.IO.Path",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Named("System.String")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "System.IO.Path.GetDirectoryName",
		ShortName:     "GetDirectoryName",
		ReceiverType:  "System.IO.Path",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Named("System.String")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "System.IO.Path.GetExtension",
		ShortName:     "GetExtension",
		ReceiverType:  "System.IO.Path",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Named("System.String")}),
	})

	// --- System.Environment ---
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "System.Environment",
		ShortName:     "Environment",
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "System.Environment.GetEnvironmentVariable",
		ShortName:     "GetEnvironmentVariable",
		ReceiverType:  "System.Environment",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Named("System.String")}),
	})

	// --- System.Nullable (bare, non-generic form for lookup) ---
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "System.Nullable",
		ShortName:     "Nullable",
		TypeParams:    []string{"T"},
		EmbeddedTypes: []string{"System.Object"},
		Fields: []typresolve.Field{
			{Name: "HasValue", Type: typresolve.Named("System.Boolean")},
			{Name: "Value", Type: typresolve.TypeParamType("T")},
		},
	})

	// --- System.Exception ---
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "System.Exception",
		ShortName:     "Exception",
		EmbeddedTypes: []string{"System.Object"},
		Fields: []typresolve.Field{
			{Name: "Message", Type: typresolve.Named("System.String")},
			{Name: "StackTrace", Type: typresolve.Named("System.String")},
			{Name: "InnerException", Type: typresolve.Named("System.Exception")},
		},
	})

	// --- System.DateTime ---
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "System.DateTime",
		ShortName:     "DateTime",
		EmbeddedTypes: []string{"System.Object"},
		Fields: []typresolve.Field{
			{Name: "Year", Type: typresolve.Named("System.Int32")},
			{Name: "Month", Type: typresolve.Named("System.Int32")},
			{Name: "Day", Type: typresolve.Named("System.Int32")},
			{Name: "Hour", Type: typresolve.Named("System.Int32")},
			{Name: "Minute", Type: typresolve.Named("System.Int32")},
			{Name: "Second", Type: typresolve.Named("System.Int32")},
		},
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "System.DateTime.Now",
		ShortName:     "Now",
		ReceiverType:  "System.DateTime",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Named("System.DateTime")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "System.DateTime.Parse",
		ShortName:     "Parse",
		ReceiverType:  "System.DateTime",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Named("System.DateTime")}),
	})

	// --- System.Guid ---
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "System.Guid",
		ShortName:     "Guid",
		EmbeddedTypes: []string{"System.Object"},
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "System.Guid.NewGuid",
		ShortName:     "NewGuid",
		ReceiverType:  "System.Guid",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Named("System.Guid")}),
	})

	// --- System.Text.StringBuilder ---
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "System.Text.StringBuilder",
		ShortName:     "StringBuilder",
		EmbeddedTypes: []string{"System.Object"},
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "System.Text.StringBuilder.Append",
		ShortName:     "Append",
		ReceiverType:  "System.Text.StringBuilder",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Named("System.Text.StringBuilder")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "System.Text.StringBuilder.AppendLine",
		ShortName:     "AppendLine",
		ReceiverType:  "System.Text.StringBuilder",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Named("System.Text.StringBuilder")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "System.Text.StringBuilder.ToString",
		ShortName:     "ToString",
		ReceiverType:  "System.Text.StringBuilder",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Named("System.String")}),
	})
}

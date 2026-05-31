package goresolve

import "github.com/blackwell-systems/knowing/internal/typresolve"

// registerGoStdlib registers the most common Go stdlib packages with
// their function signatures into the given registry. This enables the
// resolver to track return types through stdlib call chains (e.g.
// os.Open() returns *File, then file.Read() returns (int, error)).
func registerGoStdlib(reg *typresolve.Registry) {
	// --- fmt ---
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "fmt.Sprintf",
		ShortName:     "Sprintf",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("string")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "fmt.Errorf",
		ShortName:     "Errorf",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("error")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "fmt.Println",
		ShortName:     "Println",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("int"), typresolve.Builtin("error")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "fmt.Printf",
		ShortName:     "Printf",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("int"), typresolve.Builtin("error")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "fmt.Fprintf",
		ShortName:     "Fprintf",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("int"), typresolve.Builtin("error")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "fmt.Sprint",
		ShortName:     "Sprint",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("string")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "fmt.Sprintln",
		ShortName:     "Sprintln",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("string")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "fmt.Sscanf",
		ShortName:     "Sscanf",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("int"), typresolve.Builtin("error")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "fmt.Fscanf",
		ShortName:     "Fscanf",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("int"), typresolve.Builtin("error")}),
	})

	// --- strings ---
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "strings.Builder",
		ShortName:     "Builder",
	})
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "strings.Reader",
		ShortName:     "Reader",
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "strings.Contains",
		ShortName:     "Contains",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("bool")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "strings.HasPrefix",
		ShortName:     "HasPrefix",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("bool")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "strings.HasSuffix",
		ShortName:     "HasSuffix",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("bool")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "strings.Split",
		ShortName:     "Split",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Slice(typresolve.Builtin("string"))}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "strings.SplitN",
		ShortName:     "SplitN",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Slice(typresolve.Builtin("string"))}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "strings.Join",
		ShortName:     "Join",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("string")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "strings.Replace",
		ShortName:     "Replace",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("string")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "strings.ReplaceAll",
		ShortName:     "ReplaceAll",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("string")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "strings.TrimSpace",
		ShortName:     "TrimSpace",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("string")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "strings.Trim",
		ShortName:     "Trim",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("string")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "strings.TrimPrefix",
		ShortName:     "TrimPrefix",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("string")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "strings.TrimSuffix",
		ShortName:     "TrimSuffix",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("string")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "strings.ToLower",
		ShortName:     "ToLower",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("string")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "strings.ToUpper",
		ShortName:     "ToUpper",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("string")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "strings.Index",
		ShortName:     "Index",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("int")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "strings.LastIndex",
		ShortName:     "LastIndex",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("int")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "strings.Count",
		ShortName:     "Count",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("int")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "strings.Repeat",
		ShortName:     "Repeat",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("string")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "strings.NewReader",
		ShortName:     "NewReader",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Pointer(typresolve.Named("strings.Reader"))}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "strings.NewReplacer",
		ShortName:     "NewReplacer",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Pointer(typresolve.Named("strings.Replacer"))}),
	})
	// Builder methods
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "strings.Builder.WriteString",
		ShortName:     "WriteString",
		ReceiverType:  "strings.Builder",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("int"), typresolve.Builtin("error")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "strings.Builder.String",
		ShortName:     "String",
		ReceiverType:  "strings.Builder",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("string")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "strings.Builder.Len",
		ShortName:     "Len",
		ReceiverType:  "strings.Builder",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("int")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "strings.Builder.Reset",
		ShortName:     "Reset",
		ReceiverType:  "strings.Builder",
		Signature:     typresolve.Func(nil, nil),
	})

	// --- context ---
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "context.Context",
		ShortName:     "Context",
		IsInterface:   true,
	})
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "context.CancelFunc",
		ShortName:     "CancelFunc",
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "context.Background",
		ShortName:     "Background",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Named("context.Context")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "context.TODO",
		ShortName:     "TODO",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Named("context.Context")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "context.WithCancel",
		ShortName:     "WithCancel",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Named("context.Context"), typresolve.Named("context.CancelFunc")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "context.WithTimeout",
		ShortName:     "WithTimeout",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Named("context.Context"), typresolve.Named("context.CancelFunc")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "context.WithDeadline",
		ShortName:     "WithDeadline",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Named("context.Context"), typresolve.Named("context.CancelFunc")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "context.WithValue",
		ShortName:     "WithValue",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Named("context.Context")}),
	})

	// --- io ---
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "io.Reader",
		ShortName:     "Reader",
		IsInterface:   true,
	})
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "io.Writer",
		ShortName:     "Writer",
		IsInterface:   true,
	})
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "io.ReadCloser",
		ShortName:     "ReadCloser",
		IsInterface:   true,
	})
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "io.WriteCloser",
		ShortName:     "WriteCloser",
		IsInterface:   true,
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "io.ReadAll",
		ShortName:     "ReadAll",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Slice(typresolve.Builtin("byte")), typresolve.Builtin("error")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "io.Copy",
		ShortName:     "Copy",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("int64"), typresolve.Builtin("error")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "io.NopCloser",
		ShortName:     "NopCloser",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Named("io.ReadCloser")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "io.WriteString",
		ShortName:     "WriteString",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("int"), typresolve.Builtin("error")}),
	})

	// --- os ---
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "os.File",
		ShortName:     "File",
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "os.Open",
		ShortName:     "Open",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Pointer(typresolve.Named("os.File")), typresolve.Builtin("error")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "os.Create",
		ShortName:     "Create",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Pointer(typresolve.Named("os.File")), typresolve.Builtin("error")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "os.OpenFile",
		ShortName:     "OpenFile",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Pointer(typresolve.Named("os.File")), typresolve.Builtin("error")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "os.Getenv",
		ShortName:     "Getenv",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("string")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "os.LookupEnv",
		ShortName:     "LookupEnv",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("string"), typresolve.Builtin("bool")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "os.Stat",
		ShortName:     "Stat",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Named("os.FileInfo"), typresolve.Builtin("error")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "os.ReadFile",
		ShortName:     "ReadFile",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Slice(typresolve.Builtin("byte")), typresolve.Builtin("error")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "os.WriteFile",
		ShortName:     "WriteFile",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("error")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "os.MkdirAll",
		ShortName:     "MkdirAll",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("error")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "os.Remove",
		ShortName:     "Remove",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("error")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "os.RemoveAll",
		ShortName:     "RemoveAll",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("error")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "os.Getwd",
		ShortName:     "Getwd",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("string"), typresolve.Builtin("error")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "os.Exit",
		ShortName:     "Exit",
		Signature:     typresolve.Func(nil, nil),
	})
	// File methods
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "os.File.Read",
		ShortName:     "Read",
		ReceiverType:  "os.File",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("int"), typresolve.Builtin("error")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "os.File.Write",
		ShortName:     "Write",
		ReceiverType:  "os.File",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("int"), typresolve.Builtin("error")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "os.File.Close",
		ShortName:     "Close",
		ReceiverType:  "os.File",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("error")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "os.File.Name",
		ShortName:     "Name",
		ReceiverType:  "os.File",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("string")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "os.File.Stat",
		ShortName:     "Stat",
		ReceiverType:  "os.File",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Named("os.FileInfo"), typresolve.Builtin("error")}),
	})

	// --- net/http ---
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "net/http.Request",
		ShortName:     "Request",
	})
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "net/http.Response",
		ShortName:     "Response",
	})
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "net/http.Client",
		ShortName:     "Client",
	})
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "net/http.Server",
		ShortName:     "Server",
	})
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "net/http.Handler",
		ShortName:     "Handler",
		IsInterface:   true,
	})
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "net/http.HandlerFunc",
		ShortName:     "HandlerFunc",
	})
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "net/http.ServeMux",
		ShortName:     "ServeMux",
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "net/http.NewRequest",
		ShortName:     "NewRequest",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Pointer(typresolve.Named("net/http.Request")), typresolve.Builtin("error")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "net/http.Get",
		ShortName:     "Get",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Pointer(typresolve.Named("net/http.Response")), typresolve.Builtin("error")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "net/http.Post",
		ShortName:     "Post",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Pointer(typresolve.Named("net/http.Response")), typresolve.Builtin("error")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "net/http.ListenAndServe",
		ShortName:     "ListenAndServe",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("error")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "net/http.Handle",
		ShortName:     "Handle",
		Signature:     typresolve.Func(nil, nil),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "net/http.HandleFunc",
		ShortName:     "HandleFunc",
		Signature:     typresolve.Func(nil, nil),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "net/http.NewServeMux",
		ShortName:     "NewServeMux",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Pointer(typresolve.Named("net/http.ServeMux"))}),
	})
	// Client methods
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "net/http.Client.Do",
		ShortName:     "Do",
		ReceiverType:  "net/http.Client",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Pointer(typresolve.Named("net/http.Response")), typresolve.Builtin("error")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "net/http.Client.Get",
		ShortName:     "Get",
		ReceiverType:  "net/http.Client",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Pointer(typresolve.Named("net/http.Response")), typresolve.Builtin("error")}),
	})
	// Response methods
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "net/http.Response.Body.Close",
		ShortName:     "Close",
		ReceiverType:  "net/http.Response",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("error")}),
	})

	// --- sync ---
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "sync.WaitGroup",
		ShortName:     "WaitGroup",
	})
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "sync.Mutex",
		ShortName:     "Mutex",
	})
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "sync.RWMutex",
		ShortName:     "RWMutex",
	})
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "sync.Once",
		ShortName:     "Once",
	})
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "sync.Map",
		ShortName:     "Map",
	})
	// WaitGroup methods
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "sync.WaitGroup.Add",
		ShortName:     "Add",
		ReceiverType:  "sync.WaitGroup",
		Signature:     typresolve.Func(nil, nil),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "sync.WaitGroup.Done",
		ShortName:     "Done",
		ReceiverType:  "sync.WaitGroup",
		Signature:     typresolve.Func(nil, nil),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "sync.WaitGroup.Wait",
		ShortName:     "Wait",
		ReceiverType:  "sync.WaitGroup",
		Signature:     typresolve.Func(nil, nil),
	})
	// Mutex methods
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "sync.Mutex.Lock",
		ShortName:     "Lock",
		ReceiverType:  "sync.Mutex",
		Signature:     typresolve.Func(nil, nil),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "sync.Mutex.Unlock",
		ShortName:     "Unlock",
		ReceiverType:  "sync.Mutex",
		Signature:     typresolve.Func(nil, nil),
	})
	// RWMutex methods
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "sync.RWMutex.RLock",
		ShortName:     "RLock",
		ReceiverType:  "sync.RWMutex",
		Signature:     typresolve.Func(nil, nil),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "sync.RWMutex.RUnlock",
		ShortName:     "RUnlock",
		ReceiverType:  "sync.RWMutex",
		Signature:     typresolve.Func(nil, nil),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "sync.RWMutex.Lock",
		ShortName:     "Lock",
		ReceiverType:  "sync.RWMutex",
		Signature:     typresolve.Func(nil, nil),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "sync.RWMutex.Unlock",
		ShortName:     "Unlock",
		ReceiverType:  "sync.RWMutex",
		Signature:     typresolve.Func(nil, nil),
	})
	// Once methods
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "sync.Once.Do",
		ShortName:     "Do",
		ReceiverType:  "sync.Once",
		Signature:     typresolve.Func(nil, nil),
	})

	// --- errors ---
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "errors.New",
		ShortName:     "New",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("error")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "errors.Is",
		ShortName:     "Is",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("bool")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "errors.As",
		ShortName:     "As",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("bool")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "errors.Unwrap",
		ShortName:     "Unwrap",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("error")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "errors.Join",
		ShortName:     "Join",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("error")}),
	})

	// --- encoding/json ---
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "encoding/json.Decoder",
		ShortName:     "Decoder",
	})
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "encoding/json.Encoder",
		ShortName:     "Encoder",
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "encoding/json.Marshal",
		ShortName:     "Marshal",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Slice(typresolve.Builtin("byte")), typresolve.Builtin("error")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "encoding/json.MarshalIndent",
		ShortName:     "MarshalIndent",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Slice(typresolve.Builtin("byte")), typresolve.Builtin("error")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "encoding/json.Unmarshal",
		ShortName:     "Unmarshal",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("error")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "encoding/json.NewDecoder",
		ShortName:     "NewDecoder",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Pointer(typresolve.Named("encoding/json.Decoder"))}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "encoding/json.NewEncoder",
		ShortName:     "NewEncoder",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Pointer(typresolve.Named("encoding/json.Encoder"))}),
	})
	// Decoder methods
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "encoding/json.Decoder.Decode",
		ShortName:     "Decode",
		ReceiverType:  "encoding/json.Decoder",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("error")}),
	})
	// Encoder methods
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "encoding/json.Encoder.Encode",
		ShortName:     "Encode",
		ReceiverType:  "encoding/json.Encoder",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("error")}),
	})

	// --- path/filepath ---
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "path/filepath.Join",
		ShortName:     "Join",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("string")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "path/filepath.Dir",
		ShortName:     "Dir",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("string")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "path/filepath.Base",
		ShortName:     "Base",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("string")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "path/filepath.Ext",
		ShortName:     "Ext",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("string")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "path/filepath.Abs",
		ShortName:     "Abs",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("string"), typresolve.Builtin("error")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "path/filepath.Rel",
		ShortName:     "Rel",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("string"), typresolve.Builtin("error")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "path/filepath.Walk",
		ShortName:     "Walk",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("error")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "path/filepath.WalkDir",
		ShortName:     "WalkDir",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("error")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "path/filepath.Glob",
		ShortName:     "Glob",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Slice(typresolve.Builtin("string")), typresolve.Builtin("error")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "path/filepath.Match",
		ShortName:     "Match",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("bool"), typresolve.Builtin("error")}),
	})

	// --- time ---
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "time.Time",
		ShortName:     "Time",
	})
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "time.Duration",
		ShortName:     "Duration",
	})
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "time.Timer",
		ShortName:     "Timer",
	})
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "time.Ticker",
		ShortName:     "Ticker",
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "time.Now",
		ShortName:     "Now",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Named("time.Time")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "time.Since",
		ShortName:     "Since",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Named("time.Duration")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "time.NewTimer",
		ShortName:     "NewTimer",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Pointer(typresolve.Named("time.Timer"))}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "time.NewTicker",
		ShortName:     "NewTicker",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Pointer(typresolve.Named("time.Ticker"))}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "time.After",
		ShortName:     "After",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Channel(typresolve.Named("time.Time"), typresolve.ChanRecv)}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "time.Sleep",
		ShortName:     "Sleep",
		Signature:     typresolve.Func(nil, nil),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "time.Parse",
		ShortName:     "Parse",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Named("time.Time"), typresolve.Builtin("error")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "time.ParseDuration",
		ShortName:     "ParseDuration",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Named("time.Duration"), typresolve.Builtin("error")}),
	})

	// --- regexp ---
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "regexp.Regexp",
		ShortName:     "Regexp",
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "regexp.Compile",
		ShortName:     "Compile",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Pointer(typresolve.Named("regexp.Regexp")), typresolve.Builtin("error")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "regexp.MustCompile",
		ShortName:     "MustCompile",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Pointer(typresolve.Named("regexp.Regexp"))}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "regexp.MatchString",
		ShortName:     "MatchString",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("bool"), typresolve.Builtin("error")}),
	})
	// Regexp methods
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "regexp.Regexp.FindString",
		ShortName:     "FindString",
		ReceiverType:  "regexp.Regexp",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("string")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "regexp.Regexp.FindStringSubmatch",
		ShortName:     "FindStringSubmatch",
		ReceiverType:  "regexp.Regexp",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Slice(typresolve.Builtin("string"))}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "regexp.Regexp.MatchString",
		ShortName:     "MatchString",
		ReceiverType:  "regexp.Regexp",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("bool")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "regexp.Regexp.ReplaceAllString",
		ShortName:     "ReplaceAllString",
		ReceiverType:  "regexp.Regexp",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("string")}),
	})

	// --- sort ---
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "sort.Slice",
		ShortName:     "Slice",
		Signature:     typresolve.Func(nil, nil),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "sort.Sort",
		ShortName:     "Sort",
		Signature:     typresolve.Func(nil, nil),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "sort.Search",
		ShortName:     "Search",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("int")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "sort.Strings",
		ShortName:     "Strings",
		Signature:     typresolve.Func(nil, nil),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "sort.Ints",
		ShortName:     "Ints",
		Signature:     typresolve.Func(nil, nil),
	})

	// --- bytes ---
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "bytes.Buffer",
		ShortName:     "Buffer",
	})
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "bytes.Reader",
		ShortName:     "Reader",
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "bytes.NewReader",
		ShortName:     "NewReader",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Pointer(typresolve.Named("bytes.Reader"))}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "bytes.NewBuffer",
		ShortName:     "NewBuffer",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Pointer(typresolve.Named("bytes.Buffer"))}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "bytes.Contains",
		ShortName:     "Contains",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("bool")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "bytes.Equal",
		ShortName:     "Equal",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("bool")}),
	})
	// Buffer methods
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "bytes.Buffer.Write",
		ShortName:     "Write",
		ReceiverType:  "bytes.Buffer",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("int"), typresolve.Builtin("error")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "bytes.Buffer.WriteString",
		ShortName:     "WriteString",
		ReceiverType:  "bytes.Buffer",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("int"), typresolve.Builtin("error")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "bytes.Buffer.String",
		ShortName:     "String",
		ReceiverType:  "bytes.Buffer",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("string")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "bytes.Buffer.Bytes",
		ShortName:     "Bytes",
		ReceiverType:  "bytes.Buffer",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Slice(typresolve.Builtin("byte"))}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "bytes.Buffer.Len",
		ShortName:     "Len",
		ReceiverType:  "bytes.Buffer",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("int")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "bytes.Buffer.Reset",
		ShortName:     "Reset",
		ReceiverType:  "bytes.Buffer",
		Signature:     typresolve.Func(nil, nil),
	})

	// --- log ---
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "log.Logger",
		ShortName:     "Logger",
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "log.Printf",
		ShortName:     "Printf",
		Signature:     typresolve.Func(nil, nil),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "log.Println",
		ShortName:     "Println",
		Signature:     typresolve.Func(nil, nil),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "log.Fatal",
		ShortName:     "Fatal",
		Signature:     typresolve.Func(nil, nil),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "log.Fatalf",
		ShortName:     "Fatalf",
		Signature:     typresolve.Func(nil, nil),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "log.New",
		ShortName:     "New",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Pointer(typresolve.Named("log.Logger"))}),
	})

	// --- strconv ---
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "strconv.Itoa",
		ShortName:     "Itoa",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("string")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "strconv.Atoi",
		ShortName:     "Atoi",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("int"), typresolve.Builtin("error")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "strconv.FormatInt",
		ShortName:     "FormatInt",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("string")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "strconv.ParseInt",
		ShortName:     "ParseInt",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("int64"), typresolve.Builtin("error")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "strconv.ParseFloat",
		ShortName:     "ParseFloat",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("float64"), typresolve.Builtin("error")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "strconv.FormatBool",
		ShortName:     "FormatBool",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("string")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "strconv.ParseBool",
		ShortName:     "ParseBool",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("bool"), typresolve.Builtin("error")}),
	})

	// --- bufio ---
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "bufio.Scanner",
		ShortName:     "Scanner",
	})
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "bufio.Reader",
		ShortName:     "Reader",
	})
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "bufio.Writer",
		ShortName:     "Writer",
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "bufio.NewScanner",
		ShortName:     "NewScanner",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Pointer(typresolve.Named("bufio.Scanner"))}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "bufio.NewReader",
		ShortName:     "NewReader",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Pointer(typresolve.Named("bufio.Reader"))}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "bufio.NewWriter",
		ShortName:     "NewWriter",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Pointer(typresolve.Named("bufio.Writer"))}),
	})
	// Scanner methods
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "bufio.Scanner.Scan",
		ShortName:     "Scan",
		ReceiverType:  "bufio.Scanner",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("bool")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "bufio.Scanner.Text",
		ShortName:     "Text",
		ReceiverType:  "bufio.Scanner",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("string")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "bufio.Scanner.Err",
		ShortName:     "Err",
		ReceiverType:  "bufio.Scanner",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("error")}),
	})

	// --- path ---
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "path.Join",
		ShortName:     "Join",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("string")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "path.Base",
		ShortName:     "Base",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("string")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "path.Dir",
		ShortName:     "Dir",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("string")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "path.Ext",
		ShortName:     "Ext",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("string")}),
	})

	// --- database/sql ---
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "database/sql.DB",
		ShortName:     "DB",
	})
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "database/sql.Tx",
		ShortName:     "Tx",
	})
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "database/sql.Row",
		ShortName:     "Row",
	})
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: "database/sql.Rows",
		ShortName:     "Rows",
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "database/sql.Open",
		ShortName:     "Open",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Pointer(typresolve.Named("database/sql.DB")), typresolve.Builtin("error")}),
	})
	// DB methods
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "database/sql.DB.Query",
		ShortName:     "Query",
		ReceiverType:  "database/sql.DB",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Pointer(typresolve.Named("database/sql.Rows")), typresolve.Builtin("error")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "database/sql.DB.QueryRow",
		ShortName:     "QueryRow",
		ReceiverType:  "database/sql.DB",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Pointer(typresolve.Named("database/sql.Row"))}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "database/sql.DB.Exec",
		ShortName:     "Exec",
		ReceiverType:  "database/sql.DB",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Named("database/sql.Result"), typresolve.Builtin("error")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "database/sql.DB.Begin",
		ShortName:     "Begin",
		ReceiverType:  "database/sql.DB",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Pointer(typresolve.Named("database/sql.Tx")), typresolve.Builtin("error")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "database/sql.DB.Close",
		ShortName:     "Close",
		ReceiverType:  "database/sql.DB",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("error")}),
	})
	// Rows methods
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "database/sql.Rows.Next",
		ShortName:     "Next",
		ReceiverType:  "database/sql.Rows",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("bool")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "database/sql.Rows.Scan",
		ShortName:     "Scan",
		ReceiverType:  "database/sql.Rows",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("error")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "database/sql.Rows.Close",
		ShortName:     "Close",
		ReceiverType:  "database/sql.Rows",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("error")}),
	})
	// Row methods
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "database/sql.Row.Scan",
		ShortName:     "Scan",
		ReceiverType:  "database/sql.Row",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("error")}),
	})

	// --- ioutil (deprecated but still seen) ---
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "io/ioutil.ReadAll",
		ShortName:     "ReadAll",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Slice(typresolve.Builtin("byte")), typresolve.Builtin("error")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "io/ioutil.ReadFile",
		ShortName:     "ReadFile",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Slice(typresolve.Builtin("byte")), typresolve.Builtin("error")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "io/ioutil.WriteFile",
		ShortName:     "WriteFile",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("error")}),
	})
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: "io/ioutil.TempDir",
		ShortName:     "TempDir",
		Signature:     typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("string"), typresolve.Builtin("error")}),
	})
}

package tsresolve

import "github.com/blackwell-systems/knowing/internal/typresolve"

// RegisterStdlib populates a Registry with TypeScript/JavaScript standard
// library types and their methods. Mirrors cbm_ts_stdlib_register from the C
// reference implementation: Array (with typed callbacks), Promise, Map, Set,
// String, Number, Boolean, Date, RegExp, Error, Math, Object, JSON, console,
// and DOM essentials (EventTarget, Node, Element, HTMLElement, Document, Event,
// Response).
func RegisterStdlib(reg *typresolve.Registry) {
	if reg == nil {
		return
	}

	tString := typresolve.Builtin("string")
	tNumber := typresolve.Builtin("number")
	tBoolean := typresolve.Builtin("boolean")
	tVoid := typresolve.Builtin("void")
	tUnknown := typresolve.Unknown()

	// ── Array<T> ─────────────────────────────────────────────────────────
	// Type param T used in callback signatures for contextual typing.
	tParamT := typresolve.TypeParamType("T")
	tParamU := typresolve.TypeParamType("U")
	arrT := typresolve.Slice(tParamT)

	regType(reg, "Array", nil)

	// Simple Array methods (non-callback).
	regMethod(reg, "Array", "push", typresolve.Func(
		[]typresolve.Param{{Name: "items", Type: tParamT}},
		[]*typresolve.Type{tNumber},
	))
	regMethod(reg, "Array", "pop", typresolve.Func(nil, []*typresolve.Type{tParamT}))
	regMethod(reg, "Array", "shift", typresolve.Func(nil, []*typresolve.Type{tParamT}))
	regMethod(reg, "Array", "unshift", typresolve.Func(
		[]typresolve.Param{{Name: "items", Type: tParamT}},
		[]*typresolve.Type{tNumber},
	))
	regMethod(reg, "Array", "slice", typresolve.Func(nil, []*typresolve.Type{arrT}))
	regMethod(reg, "Array", "splice", typresolve.Func(nil, []*typresolve.Type{arrT}))
	regMethod(reg, "Array", "concat", typresolve.Func(nil, []*typresolve.Type{arrT}))
	regMethod(reg, "Array", "join", typresolve.Func(nil, []*typresolve.Type{tString}))
	regMethod(reg, "Array", "indexOf", typresolve.Func(nil, []*typresolve.Type{tNumber}))
	regMethod(reg, "Array", "lastIndexOf", typresolve.Func(nil, []*typresolve.Type{tNumber}))
	regMethod(reg, "Array", "includes", typresolve.Func(nil, []*typresolve.Type{tBoolean}))
	regMethod(reg, "Array", "reverse", typresolve.Func(nil, []*typresolve.Type{arrT}))
	regMethod(reg, "Array", "at", typresolve.Func(nil, []*typresolve.Type{tParamT}))
	regMethod(reg, "Array", "flat", typresolve.Func(nil, []*typresolve.Type{arrT}))
	regMethod(reg, "Array", "fill", typresolve.Func(nil, []*typresolve.Type{arrT}))
	regMethod(reg, "Array", "copyWithin", typresolve.Func(nil, []*typresolve.Type{arrT}))
	regMethod(reg, "Array", "entries", typresolve.Func(nil, []*typresolve.Type{tUnknown}))
	regMethod(reg, "Array", "keys", typresolve.Func(nil, []*typresolve.Type{tUnknown}))
	regMethod(reg, "Array", "values", typresolve.Func(nil, []*typresolve.Type{tUnknown}))
	regMethod(reg, "Array", "length", nil) // property, not method

	// Callback methods: signature carries typed callback param for contextual typing.
	// forEach(callback: (x: T) => void): void
	cbForEach := typresolve.Func(
		[]typresolve.Param{{Name: "x", Type: tParamT}},
		[]*typresolve.Type{tVoid},
	)
	regMethod(reg, "Array", "forEach", typresolve.Func(
		[]typresolve.Param{{Name: "callback", Type: cbForEach}},
		[]*typresolve.Type{tVoid},
	))

	// map(callback: (x: T) => U): U[]
	cbMap := typresolve.Func(
		[]typresolve.Param{{Name: "x", Type: tParamT}},
		[]*typresolve.Type{tParamU},
	)
	regMethod(reg, "Array", "map", typresolve.Func(
		[]typresolve.Param{{Name: "callback", Type: cbMap}},
		[]*typresolve.Type{typresolve.Slice(tParamU)},
	))

	// filter(callback: (x: T) => boolean): T[]
	cbFilter := typresolve.Func(
		[]typresolve.Param{{Name: "x", Type: tParamT}},
		[]*typresolve.Type{tBoolean},
	)
	regMethod(reg, "Array", "filter", typresolve.Func(
		[]typresolve.Param{{Name: "callback", Type: cbFilter}},
		[]*typresolve.Type{arrT},
	))

	// find(callback: (x: T) => boolean): T
	regMethod(reg, "Array", "find", typresolve.Func(
		[]typresolve.Param{{Name: "callback", Type: cbFilter}},
		[]*typresolve.Type{tParamT},
	))

	// findIndex(callback: (x: T) => boolean): number
	regMethod(reg, "Array", "findIndex", typresolve.Func(
		[]typresolve.Param{{Name: "callback", Type: cbFilter}},
		[]*typresolve.Type{tNumber},
	))

	// some(callback: (x: T) => boolean): boolean
	regMethod(reg, "Array", "some", typresolve.Func(
		[]typresolve.Param{{Name: "callback", Type: cbFilter}},
		[]*typresolve.Type{tBoolean},
	))

	// every(callback: (x: T) => boolean): boolean
	regMethod(reg, "Array", "every", typresolve.Func(
		[]typresolve.Param{{Name: "callback", Type: cbFilter}},
		[]*typresolve.Type{tBoolean},
	))

	// sort(callback: (a: T, b: T) => number): T[]
	cbSort := typresolve.Func(
		[]typresolve.Param{{Name: "a", Type: tParamT}, {Name: "b", Type: tParamT}},
		[]*typresolve.Type{tNumber},
	)
	regMethod(reg, "Array", "sort", typresolve.Func(
		[]typresolve.Param{{Name: "compareFn", Type: cbSort}},
		[]*typresolve.Type{arrT},
	))

	// reduce(callback: (acc: U, x: T) => U, initial: U): U
	cbReduce := typresolve.Func(
		[]typresolve.Param{{Name: "acc", Type: tParamU}, {Name: "x", Type: tParamT}},
		[]*typresolve.Type{tParamU},
	)
	regMethod(reg, "Array", "reduce", typresolve.Func(
		[]typresolve.Param{{Name: "callback", Type: cbReduce}, {Name: "initialValue", Type: tParamU}},
		[]*typresolve.Type{tParamU},
	))

	// flatMap(callback: (x: T) => U[]): U[]
	cbFlatMap := typresolve.Func(
		[]typresolve.Param{{Name: "x", Type: tParamT}},
		[]*typresolve.Type{typresolve.Slice(tParamU)},
	)
	regMethod(reg, "Array", "flatMap", typresolve.Func(
		[]typresolve.Param{{Name: "callback", Type: cbFlatMap}},
		[]*typresolve.Type{typresolve.Slice(tParamU)},
	))

	// Static methods.
	regMethod(reg, "Array", "from", typresolve.Func(nil, []*typresolve.Type{arrT}))
	regMethod(reg, "Array", "isArray", typresolve.Func(nil, []*typresolve.Type{tBoolean}))
	regMethod(reg, "Array", "of", typresolve.Func(nil, []*typresolve.Type{arrT}))

	// ── Promise<T> ───────────────────────────────────────────────────────
	promT := typresolve.Named("Promise")
	promT.TypeParams = []typresolve.TypeParam{{Name: "T", Constraint: tParamT}}

	regType(reg, "Promise", nil)

	// then(callback: (value: T) => U): Promise<U>
	cbThen := typresolve.Func(
		[]typresolve.Param{{Name: "value", Type: tParamT}},
		[]*typresolve.Type{tParamU},
	)
	promRet := typresolve.Named("Promise")
	promRet.TypeParams = []typresolve.TypeParam{{Name: "T", Constraint: tParamU}}
	regMethod(reg, "Promise", "then", typresolve.Func(
		[]typresolve.Param{{Name: "onfulfilled", Type: cbThen}},
		[]*typresolve.Type{promRet},
	))

	// catch(callback: (reason: any) => U): Promise<T>
	promSelf := typresolve.Named("Promise")
	regMethod(reg, "Promise", "catch", typresolve.Func(nil, []*typresolve.Type{promSelf}))
	regMethod(reg, "Promise", "finally", typresolve.Func(nil, []*typresolve.Type{promSelf}))

	// Static methods.
	regMethod(reg, "Promise", "resolve", typresolve.Func(nil, []*typresolve.Type{promSelf}))
	regMethod(reg, "Promise", "reject", typresolve.Func(nil, []*typresolve.Type{promSelf}))
	regMethod(reg, "Promise", "all", typresolve.Func(nil, []*typresolve.Type{promSelf}))
	regMethod(reg, "Promise", "allSettled", typresolve.Func(nil, []*typresolve.Type{promSelf}))
	regMethod(reg, "Promise", "race", typresolve.Func(nil, []*typresolve.Type{promSelf}))
	regMethod(reg, "Promise", "any", typresolve.Func(nil, []*typresolve.Type{promSelf}))

	// ── Map<K,V> ─────────────────────────────────────────────────────────
	tParamK := typresolve.TypeParamType("K")
	tParamV := typresolve.TypeParamType("V")
	mapSelf := typresolve.Named("Map")

	regType(reg, "Map", nil)
	regMethod(reg, "Map", "get", typresolve.Func(
		[]typresolve.Param{{Name: "key", Type: tParamK}},
		[]*typresolve.Type{tParamV},
	))
	regMethod(reg, "Map", "set", typresolve.Func(
		[]typresolve.Param{{Name: "key", Type: tParamK}, {Name: "value", Type: tParamV}},
		[]*typresolve.Type{mapSelf},
	))
	regMethod(reg, "Map", "has", typresolve.Func(nil, []*typresolve.Type{tBoolean}))
	regMethod(reg, "Map", "delete", typresolve.Func(nil, []*typresolve.Type{tBoolean}))
	regMethod(reg, "Map", "clear", typresolve.Func(nil, []*typresolve.Type{tVoid}))
	regMethod(reg, "Map", "forEach", typresolve.Func(nil, []*typresolve.Type{tVoid}))
	regMethod(reg, "Map", "keys", typresolve.Func(nil, []*typresolve.Type{tUnknown}))
	regMethod(reg, "Map", "values", typresolve.Func(nil, []*typresolve.Type{tUnknown}))
	regMethod(reg, "Map", "entries", typresolve.Func(nil, []*typresolve.Type{tUnknown}))
	regMethod(reg, "Map", "size", nil) // property

	// ── Set<T> ───────────────────────────────────────────────────────────
	setSelf := typresolve.Named("Set")
	regType(reg, "Set", nil)
	regMethod(reg, "Set", "add", typresolve.Func(nil, []*typresolve.Type{setSelf}))
	regMethod(reg, "Set", "has", typresolve.Func(nil, []*typresolve.Type{tBoolean}))
	regMethod(reg, "Set", "delete", typresolve.Func(nil, []*typresolve.Type{tBoolean}))
	regMethod(reg, "Set", "clear", typresolve.Func(nil, []*typresolve.Type{tVoid}))
	regMethod(reg, "Set", "forEach", typresolve.Func(nil, []*typresolve.Type{tVoid}))
	regMethod(reg, "Set", "keys", typresolve.Func(nil, []*typresolve.Type{tUnknown}))
	regMethod(reg, "Set", "values", typresolve.Func(nil, []*typresolve.Type{tUnknown}))
	regMethod(reg, "Set", "entries", typresolve.Func(nil, []*typresolve.Type{tUnknown}))
	regMethod(reg, "Set", "size", nil) // property

	// ── WeakMap<K,V> / WeakSet<T> ────────────────────────────────────────
	regType(reg, "WeakMap", nil)
	regMethod(reg, "WeakMap", "get", typresolve.Func(nil, []*typresolve.Type{tUnknown}))
	regMethod(reg, "WeakMap", "set", typresolve.Func(nil, []*typresolve.Type{typresolve.Named("WeakMap")}))
	regMethod(reg, "WeakMap", "has", typresolve.Func(nil, []*typresolve.Type{tBoolean}))
	regMethod(reg, "WeakMap", "delete", typresolve.Func(nil, []*typresolve.Type{tBoolean}))

	regType(reg, "WeakSet", nil)
	regMethod(reg, "WeakSet", "add", typresolve.Func(nil, []*typresolve.Type{typresolve.Named("WeakSet")}))
	regMethod(reg, "WeakSet", "has", typresolve.Func(nil, []*typresolve.Type{tBoolean}))
	regMethod(reg, "WeakSet", "delete", typresolve.Func(nil, []*typresolve.Type{tBoolean}))

	// ── String ───────────────────────────────────────────────────────────
	regType(reg, "String", nil)
	regMethod(reg, "String", "charAt", typresolve.Func(nil, []*typresolve.Type{tString}))
	regMethod(reg, "String", "charCodeAt", typresolve.Func(nil, []*typresolve.Type{tNumber}))
	regMethod(reg, "String", "codePointAt", typresolve.Func(nil, []*typresolve.Type{tNumber}))
	regMethod(reg, "String", "concat", typresolve.Func(nil, []*typresolve.Type{tString}))
	regMethod(reg, "String", "includes", typresolve.Func(nil, []*typresolve.Type{tBoolean}))
	regMethod(reg, "String", "endsWith", typresolve.Func(nil, []*typresolve.Type{tBoolean}))
	regMethod(reg, "String", "startsWith", typresolve.Func(nil, []*typresolve.Type{tBoolean}))
	regMethod(reg, "String", "indexOf", typresolve.Func(nil, []*typresolve.Type{tNumber}))
	regMethod(reg, "String", "lastIndexOf", typresolve.Func(nil, []*typresolve.Type{tNumber}))
	regMethod(reg, "String", "match", typresolve.Func(nil, []*typresolve.Type{tUnknown}))
	regMethod(reg, "String", "matchAll", typresolve.Func(nil, []*typresolve.Type{tUnknown}))
	regMethod(reg, "String", "normalize", typresolve.Func(nil, []*typresolve.Type{tString}))
	regMethod(reg, "String", "padStart", typresolve.Func(nil, []*typresolve.Type{tString}))
	regMethod(reg, "String", "padEnd", typresolve.Func(nil, []*typresolve.Type{tString}))
	regMethod(reg, "String", "repeat", typresolve.Func(nil, []*typresolve.Type{tString}))
	regMethod(reg, "String", "replace", typresolve.Func(nil, []*typresolve.Type{tString}))
	regMethod(reg, "String", "replaceAll", typresolve.Func(nil, []*typresolve.Type{tString}))
	regMethod(reg, "String", "search", typresolve.Func(nil, []*typresolve.Type{tNumber}))
	regMethod(reg, "String", "slice", typresolve.Func(nil, []*typresolve.Type{tString}))
	regMethod(reg, "String", "split", typresolve.Func(nil, []*typresolve.Type{typresolve.Slice(tString)}))
	regMethod(reg, "String", "substring", typresolve.Func(nil, []*typresolve.Type{tString}))
	regMethod(reg, "String", "toLowerCase", typresolve.Func(nil, []*typresolve.Type{tString}))
	regMethod(reg, "String", "toUpperCase", typresolve.Func(nil, []*typresolve.Type{tString}))
	regMethod(reg, "String", "toLocaleLowerCase", typresolve.Func(nil, []*typresolve.Type{tString}))
	regMethod(reg, "String", "toLocaleUpperCase", typresolve.Func(nil, []*typresolve.Type{tString}))
	regMethod(reg, "String", "trim", typresolve.Func(nil, []*typresolve.Type{tString}))
	regMethod(reg, "String", "trimStart", typresolve.Func(nil, []*typresolve.Type{tString}))
	regMethod(reg, "String", "trimEnd", typresolve.Func(nil, []*typresolve.Type{tString}))
	regMethod(reg, "String", "at", typresolve.Func(nil, []*typresolve.Type{tString}))
	regMethod(reg, "String", "length", nil) // property

	// ── Number ───────────────────────────────────────────────────────────
	regType(reg, "Number", nil)
	regMethod(reg, "Number", "toString", typresolve.Func(nil, []*typresolve.Type{tString}))
	regMethod(reg, "Number", "toFixed", typresolve.Func(nil, []*typresolve.Type{tString}))
	regMethod(reg, "Number", "toExponential", typresolve.Func(nil, []*typresolve.Type{tString}))
	regMethod(reg, "Number", "toPrecision", typresolve.Func(nil, []*typresolve.Type{tString}))
	regMethod(reg, "Number", "valueOf", typresolve.Func(nil, []*typresolve.Type{tNumber}))
	regMethod(reg, "Number", "toLocaleString", typresolve.Func(nil, []*typresolve.Type{tString}))

	// ── Boolean ──────────────────────────────────────────────────────────
	regType(reg, "Boolean", nil)
	regMethod(reg, "Boolean", "toString", typresolve.Func(nil, []*typresolve.Type{tString}))
	regMethod(reg, "Boolean", "valueOf", typresolve.Func(nil, []*typresolve.Type{tBoolean}))

	// ── Date ─────────────────────────────────────────────────────────────
	regType(reg, "Date", nil)
	regMethod(reg, "Date", "toISOString", typresolve.Func(nil, []*typresolve.Type{tString}))
	regMethod(reg, "Date", "toString", typresolve.Func(nil, []*typresolve.Type{tString}))
	regMethod(reg, "Date", "toJSON", typresolve.Func(nil, []*typresolve.Type{tString}))
	regMethod(reg, "Date", "toDateString", typresolve.Func(nil, []*typresolve.Type{tString}))
	regMethod(reg, "Date", "toTimeString", typresolve.Func(nil, []*typresolve.Type{tString}))
	regMethod(reg, "Date", "toLocaleDateString", typresolve.Func(nil, []*typresolve.Type{tString}))
	regMethod(reg, "Date", "toLocaleTimeString", typresolve.Func(nil, []*typresolve.Type{tString}))
	regMethod(reg, "Date", "toLocaleString", typresolve.Func(nil, []*typresolve.Type{tString}))
	regMethod(reg, "Date", "getTime", typresolve.Func(nil, []*typresolve.Type{tNumber}))
	regMethod(reg, "Date", "valueOf", typresolve.Func(nil, []*typresolve.Type{tNumber}))
	regMethod(reg, "Date", "getFullYear", typresolve.Func(nil, []*typresolve.Type{tNumber}))
	regMethod(reg, "Date", "getMonth", typresolve.Func(nil, []*typresolve.Type{tNumber}))
	regMethod(reg, "Date", "getDate", typresolve.Func(nil, []*typresolve.Type{tNumber}))
	regMethod(reg, "Date", "getDay", typresolve.Func(nil, []*typresolve.Type{tNumber}))
	regMethod(reg, "Date", "getHours", typresolve.Func(nil, []*typresolve.Type{tNumber}))
	regMethod(reg, "Date", "getMinutes", typresolve.Func(nil, []*typresolve.Type{tNumber}))
	regMethod(reg, "Date", "getSeconds", typresolve.Func(nil, []*typresolve.Type{tNumber}))
	regMethod(reg, "Date", "getMilliseconds", typresolve.Func(nil, []*typresolve.Type{tNumber}))
	regMethod(reg, "Date", "setFullYear", typresolve.Func(nil, []*typresolve.Type{tNumber}))
	regMethod(reg, "Date", "setMonth", typresolve.Func(nil, []*typresolve.Type{tNumber}))
	regMethod(reg, "Date", "setDate", typresolve.Func(nil, []*typresolve.Type{tNumber}))
	regMethod(reg, "Date", "setHours", typresolve.Func(nil, []*typresolve.Type{tNumber}))
	regMethod(reg, "Date", "setMinutes", typresolve.Func(nil, []*typresolve.Type{tNumber}))
	regMethod(reg, "Date", "setSeconds", typresolve.Func(nil, []*typresolve.Type{tNumber}))
	regMethod(reg, "Date", "setMilliseconds", typresolve.Func(nil, []*typresolve.Type{tNumber}))
	regMethod(reg, "Date", "now", typresolve.Func(nil, []*typresolve.Type{tNumber}))   // static
	regMethod(reg, "Date", "parse", typresolve.Func(nil, []*typresolve.Type{tNumber})) // static

	// ── RegExp ───────────────────────────────────────────────────────────
	regType(reg, "RegExp", nil)
	regMethod(reg, "RegExp", "test", typresolve.Func(nil, []*typresolve.Type{tBoolean}))
	regMethod(reg, "RegExp", "exec", typresolve.Func(nil, []*typresolve.Type{tUnknown}))
	regMethod(reg, "RegExp", "toString", typresolve.Func(nil, []*typresolve.Type{tString}))
	regMethod(reg, "RegExp", "compile", typresolve.Func(nil, []*typresolve.Type{typresolve.Named("RegExp")}))

	// ── Error ────────────────────────────────────────────────────────────
	regType(reg, "Error", nil)
	regMethod(reg, "Error", "toString", typresolve.Func(nil, []*typresolve.Type{tString}))

	// ── Math ─────────────────────────────────────────────────────────────
	regType(reg, "Math", nil)
	mathMethods := []string{
		"abs", "acos", "acosh", "asin", "asinh", "atan", "atanh", "atan2",
		"cbrt", "ceil", "clz32", "cos", "cosh", "exp", "expm1", "floor",
		"fround", "hypot", "imul", "log", "log1p", "log10", "log2", "max",
		"min", "pow", "random", "round", "sign", "sin", "sinh", "sqrt",
		"tan", "tanh", "trunc",
	}
	for _, m := range mathMethods {
		regMethod(reg, "Math", m, typresolve.Func(nil, []*typresolve.Type{tNumber}))
	}

	// ── Object ───────────────────────────────────────────────────────────
	regType(reg, "Object", nil)
	regMethod(reg, "Object", "keys", typresolve.Func(nil, []*typresolve.Type{typresolve.Slice(tString)}))
	regMethod(reg, "Object", "values", typresolve.Func(nil, []*typresolve.Type{typresolve.Slice(tUnknown)}))
	regMethod(reg, "Object", "entries", typresolve.Func(nil, []*typresolve.Type{typresolve.Slice(tUnknown)}))
	regMethod(reg, "Object", "assign", typresolve.Func(nil, []*typresolve.Type{tUnknown}))
	regMethod(reg, "Object", "freeze", typresolve.Func(nil, []*typresolve.Type{tUnknown}))
	regMethod(reg, "Object", "isFrozen", typresolve.Func(nil, []*typresolve.Type{tBoolean}))
	regMethod(reg, "Object", "create", typresolve.Func(nil, []*typresolve.Type{tUnknown}))
	regMethod(reg, "Object", "getPrototypeOf", typresolve.Func(nil, []*typresolve.Type{tUnknown}))
	regMethod(reg, "Object", "defineProperty", typresolve.Func(nil, []*typresolve.Type{tUnknown}))
	regMethod(reg, "Object", "defineProperties", typresolve.Func(nil, []*typresolve.Type{tUnknown}))
	regMethod(reg, "Object", "getOwnPropertyNames", typresolve.Func(nil, []*typresolve.Type{typresolve.Slice(tString)}))
	regMethod(reg, "Object", "hasOwnProperty", typresolve.Func(nil, []*typresolve.Type{tBoolean}))
	regMethod(reg, "Object", "is", typresolve.Func(nil, []*typresolve.Type{tBoolean}))

	// ── JSON ─────────────────────────────────────────────────────────────
	regType(reg, "JSON", nil)
	regMethod(reg, "JSON", "parse", typresolve.Func(nil, []*typresolve.Type{tUnknown}))
	regMethod(reg, "JSON", "stringify", typresolve.Func(nil, []*typresolve.Type{tString}))

	// ── console ──────────────────────────────────────────────────────────
	regType(reg, "console", nil)
	consoleMethods := []string{"log", "error", "warn", "info", "debug", "trace", "dir", "table", "time", "timeEnd", "timeLog", "clear", "count", "countReset", "group", "groupEnd", "assert"}
	for _, m := range consoleMethods {
		regMethod(reg, "console", m, typresolve.Func(nil, []*typresolve.Type{tVoid}))
	}

	// ── Function ─────────────────────────────────────────────────────────
	regType(reg, "Function", nil)
	regMethod(reg, "Function", "call", typresolve.Func(nil, []*typresolve.Type{tUnknown}))
	regMethod(reg, "Function", "apply", typresolve.Func(nil, []*typresolve.Type{tUnknown}))
	regMethod(reg, "Function", "bind", typresolve.Func(nil, []*typresolve.Type{typresolve.Named("Function")}))

	// ── Symbol ───────────────────────────────────────────────────────────
	regType(reg, "Symbol", nil)
	regMethod(reg, "Symbol", "toString", typresolve.Func(nil, []*typresolve.Type{tString}))
	regMethod(reg, "Symbol", "valueOf", typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("symbol")}))
	regMethod(reg, "Symbol", "description", nil) // property

	// ── BigInt ───────────────────────────────────────────────────────────
	regType(reg, "BigInt", nil)
	regMethod(reg, "BigInt", "toString", typresolve.Func(nil, []*typresolve.Type{tString}))
	regMethod(reg, "BigInt", "valueOf", typresolve.Func(nil, []*typresolve.Type{typresolve.Builtin("bigint")}))
	regMethod(reg, "BigInt", "toLocaleString", typresolve.Func(nil, []*typresolve.Type{tString}))

	// ── DOM: EventTarget ─────────────────────────────────────────────────
	regType(reg, "EventTarget", nil)
	regMethod(reg, "EventTarget", "addEventListener", typresolve.Func(nil, []*typresolve.Type{tVoid}))
	regMethod(reg, "EventTarget", "removeEventListener", typresolve.Func(nil, []*typresolve.Type{tVoid}))
	regMethod(reg, "EventTarget", "dispatchEvent", typresolve.Func(nil, []*typresolve.Type{tBoolean}))

	// ── DOM: Node ────────────────────────────────────────────────────────
	nodeT := typresolve.Named("Node")
	regType(reg, "Node", []string{"EventTarget"})
	regMethod(reg, "Node", "appendChild", typresolve.Func(nil, []*typresolve.Type{nodeT}))
	regMethod(reg, "Node", "removeChild", typresolve.Func(nil, []*typresolve.Type{nodeT}))
	regMethod(reg, "Node", "replaceChild", typresolve.Func(nil, []*typresolve.Type{nodeT}))
	regMethod(reg, "Node", "cloneNode", typresolve.Func(nil, []*typresolve.Type{nodeT}))
	regMethod(reg, "Node", "contains", typresolve.Func(nil, []*typresolve.Type{tBoolean}))
	regMethod(reg, "Node", "hasChildNodes", typresolve.Func(nil, []*typresolve.Type{tBoolean}))

	// ── DOM: Element ─────────────────────────────────────────────────────
	elemT := typresolve.Named("Element")
	regType(reg, "Element", []string{"Node"})
	regMethod(reg, "Element", "getAttribute", typresolve.Func(nil, []*typresolve.Type{tString}))
	regMethod(reg, "Element", "setAttribute", typresolve.Func(nil, []*typresolve.Type{tVoid}))
	regMethod(reg, "Element", "removeAttribute", typresolve.Func(nil, []*typresolve.Type{tVoid}))
	regMethod(reg, "Element", "hasAttribute", typresolve.Func(nil, []*typresolve.Type{tBoolean}))
	regMethod(reg, "Element", "querySelector", typresolve.Func(nil, []*typresolve.Type{elemT}))
	regMethod(reg, "Element", "querySelectorAll", typresolve.Func(nil, []*typresolve.Type{tUnknown}))
	regMethod(reg, "Element", "closest", typresolve.Func(nil, []*typresolve.Type{elemT}))
	regMethod(reg, "Element", "matches", typresolve.Func(nil, []*typresolve.Type{tBoolean}))
	regMethod(reg, "Element", "getBoundingClientRect", typresolve.Func(nil, []*typresolve.Type{tUnknown}))
	regMethod(reg, "Element", "scrollIntoView", typresolve.Func(nil, []*typresolve.Type{tVoid}))
	regMethod(reg, "Element", "remove", typresolve.Func(nil, []*typresolve.Type{tVoid}))
	regMethod(reg, "Element", "focus", typresolve.Func(nil, []*typresolve.Type{tVoid}))
	regMethod(reg, "Element", "blur", typresolve.Func(nil, []*typresolve.Type{tVoid}))
	regMethod(reg, "Element", "click", typresolve.Func(nil, []*typresolve.Type{tVoid}))

	// ── DOM: HTMLElement ──────────────────────────────────────────────────
	htmlT := typresolve.Named("HTMLElement")
	regType(reg, "HTMLElement", []string{"Element"})
	regMethod(reg, "HTMLElement", "querySelector", typresolve.Func(nil, []*typresolve.Type{htmlT}))
	regMethod(reg, "HTMLElement", "querySelectorAll", typresolve.Func(nil, []*typresolve.Type{tUnknown}))
	regMethod(reg, "HTMLElement", "closest", typresolve.Func(nil, []*typresolve.Type{htmlT}))
	regMethod(reg, "HTMLElement", "click", typresolve.Func(nil, []*typresolve.Type{tVoid}))
	regMethod(reg, "HTMLElement", "focus", typresolve.Func(nil, []*typresolve.Type{tVoid}))
	regMethod(reg, "HTMLElement", "blur", typresolve.Func(nil, []*typresolve.Type{tVoid}))
	regMethod(reg, "HTMLElement", "remove", typresolve.Func(nil, []*typresolve.Type{tVoid}))

	// ── DOM: Document ────────────────────────────────────────────────────
	regType(reg, "Document", []string{"Node"})
	regMethod(reg, "Document", "getElementById", typresolve.Func(nil, []*typresolve.Type{htmlT}))
	regMethod(reg, "Document", "querySelector", typresolve.Func(nil, []*typresolve.Type{htmlT}))
	regMethod(reg, "Document", "querySelectorAll", typresolve.Func(nil, []*typresolve.Type{tUnknown}))
	regMethod(reg, "Document", "getElementsByClassName", typresolve.Func(nil, []*typresolve.Type{tUnknown}))
	regMethod(reg, "Document", "getElementsByTagName", typresolve.Func(nil, []*typresolve.Type{tUnknown}))
	regMethod(reg, "Document", "createElement", typresolve.Func(nil, []*typresolve.Type{htmlT}))
	regMethod(reg, "Document", "createTextNode", typresolve.Func(nil, []*typresolve.Type{nodeT}))
	regMethod(reg, "Document", "createDocumentFragment", typresolve.Func(nil, []*typresolve.Type{nodeT}))
	regMethod(reg, "Document", "addEventListener", typresolve.Func(nil, []*typresolve.Type{tVoid}))
	regMethod(reg, "Document", "removeEventListener", typresolve.Func(nil, []*typresolve.Type{tVoid}))

	// ── DOM: Window ──────────────────────────────────────────────────────
	regType(reg, "Window", []string{"EventTarget"})
	regMethod(reg, "Window", "addEventListener", typresolve.Func(nil, []*typresolve.Type{tVoid}))
	regMethod(reg, "Window", "removeEventListener", typresolve.Func(nil, []*typresolve.Type{tVoid}))
	regMethod(reg, "Window", "setTimeout", typresolve.Func(nil, []*typresolve.Type{tNumber}))
	regMethod(reg, "Window", "setInterval", typresolve.Func(nil, []*typresolve.Type{tNumber}))
	regMethod(reg, "Window", "clearTimeout", typresolve.Func(nil, []*typresolve.Type{tVoid}))
	regMethod(reg, "Window", "clearInterval", typresolve.Func(nil, []*typresolve.Type{tVoid}))
	regMethod(reg, "Window", "fetch", typresolve.Func(nil, []*typresolve.Type{typresolve.Named("Promise")}))
	regMethod(reg, "Window", "alert", typresolve.Func(nil, []*typresolve.Type{tVoid}))

	// ── DOM: Event ───────────────────────────────────────────────────────
	regType(reg, "Event", nil)
	regMethod(reg, "Event", "preventDefault", typresolve.Func(nil, []*typresolve.Type{tVoid}))
	regMethod(reg, "Event", "stopPropagation", typresolve.Func(nil, []*typresolve.Type{tVoid}))
	regMethod(reg, "Event", "stopImmediatePropagation", typresolve.Func(nil, []*typresolve.Type{tVoid}))

	// ── Fetch: Response ──────────────────────────────────────────────────
	respT := typresolve.Named("Response")
	regType(reg, "Response", nil)
	promUnknown := typresolve.Named("Promise")
	promString := typresolve.Named("Promise")
	promString.Elem = tString
	promUnknown.Elem = tUnknown
	regMethod(reg, "Response", "json", typresolve.Func(nil, []*typresolve.Type{promUnknown}))
	regMethod(reg, "Response", "text", typresolve.Func(nil, []*typresolve.Type{promString}))
	regMethod(reg, "Response", "blob", typresolve.Func(nil, []*typresolve.Type{promUnknown}))
	regMethod(reg, "Response", "arrayBuffer", typresolve.Func(nil, []*typresolve.Type{promUnknown}))
	regMethod(reg, "Response", "formData", typresolve.Func(nil, []*typresolve.Type{promUnknown}))
	regMethod(reg, "Response", "clone", typresolve.Func(nil, []*typresolve.Type{respT}))

	// ── Fetch: Request, Headers, URL, URLSearchParams ────────────────────
	regType(reg, "Request", nil)
	regType(reg, "Headers", nil)
	regMethod(reg, "Headers", "get", typresolve.Func(nil, []*typresolve.Type{tString}))
	regMethod(reg, "Headers", "set", typresolve.Func(nil, []*typresolve.Type{tVoid}))
	regMethod(reg, "Headers", "has", typresolve.Func(nil, []*typresolve.Type{tBoolean}))
	regMethod(reg, "Headers", "delete", typresolve.Func(nil, []*typresolve.Type{tVoid}))
	regMethod(reg, "Headers", "append", typresolve.Func(nil, []*typresolve.Type{tVoid}))
	regMethod(reg, "Headers", "forEach", typresolve.Func(nil, []*typresolve.Type{tVoid}))

	regType(reg, "URL", nil)
	regMethod(reg, "URL", "toString", typresolve.Func(nil, []*typresolve.Type{tString}))

	regType(reg, "URLSearchParams", nil)
	regMethod(reg, "URLSearchParams", "get", typresolve.Func(nil, []*typresolve.Type{tString}))
	regMethod(reg, "URLSearchParams", "set", typresolve.Func(nil, []*typresolve.Type{tVoid}))
	regMethod(reg, "URLSearchParams", "has", typresolve.Func(nil, []*typresolve.Type{tBoolean}))
	regMethod(reg, "URLSearchParams", "delete", typresolve.Func(nil, []*typresolve.Type{tVoid}))
	regMethod(reg, "URLSearchParams", "append", typresolve.Func(nil, []*typresolve.Type{tVoid}))
	regMethod(reg, "URLSearchParams", "toString", typresolve.Func(nil, []*typresolve.Type{tString}))

	// ── FormData, Blob, File ─────────────────────────────────────────────
	regType(reg, "FormData", nil)
	regMethod(reg, "FormData", "get", typresolve.Func(nil, []*typresolve.Type{tUnknown}))
	regMethod(reg, "FormData", "set", typresolve.Func(nil, []*typresolve.Type{tVoid}))
	regMethod(reg, "FormData", "has", typresolve.Func(nil, []*typresolve.Type{tBoolean}))
	regMethod(reg, "FormData", "delete", typresolve.Func(nil, []*typresolve.Type{tVoid}))
	regMethod(reg, "FormData", "append", typresolve.Func(nil, []*typresolve.Type{tVoid}))

	regType(reg, "Blob", nil)
	regMethod(reg, "Blob", "text", typresolve.Func(nil, []*typresolve.Type{promString}))
	regMethod(reg, "Blob", "arrayBuffer", typresolve.Func(nil, []*typresolve.Type{promUnknown}))
	regMethod(reg, "Blob", "slice", typresolve.Func(nil, []*typresolve.Type{typresolve.Named("Blob")}))

	regType(reg, "File", []string{"Blob"})

	// ── Iterator/Generator ───────────────────────────────────────────────
	regType(reg, "Iterator", nil)
	regMethod(reg, "Iterator", "next", typresolve.Func(nil, []*typresolve.Type{tUnknown}))
	regMethod(reg, "Iterator", "return", typresolve.Func(nil, []*typresolve.Type{tUnknown}))
	regMethod(reg, "Iterator", "throw", typresolve.Func(nil, []*typresolve.Type{tUnknown}))

	regType(reg, "Iterable", nil)
	regType(reg, "AsyncIterator", nil)
	regMethod(reg, "AsyncIterator", "next", typresolve.Func(nil, []*typresolve.Type{promUnknown}))
	regType(reg, "AsyncIterable", nil)

	regType(reg, "Generator", []string{"Iterator"})
	regMethod(reg, "Generator", "next", typresolve.Func(nil, []*typresolve.Type{tUnknown}))
	regMethod(reg, "Generator", "return", typresolve.Func(nil, []*typresolve.Type{tUnknown}))
	regMethod(reg, "Generator", "throw", typresolve.Func(nil, []*typresolve.Type{tUnknown}))

	regType(reg, "AsyncGenerator", []string{"AsyncIterator"})
	regMethod(reg, "AsyncGenerator", "next", typresolve.Func(nil, []*typresolve.Type{promUnknown}))
	regMethod(reg, "AsyncGenerator", "return", typresolve.Func(nil, []*typresolve.Type{promUnknown}))
	regMethod(reg, "AsyncGenerator", "throw", typresolve.Func(nil, []*typresolve.Type{promUnknown}))
}

// regType is a helper to register a type in the registry.
func regType(reg *typresolve.Registry, name string, embeddedTypes []string) {
	reg.AddType(typresolve.RegisteredType{
		QualifiedName: name,
		ShortName:     name,
		EmbeddedTypes: embeddedTypes,
	})
}

// regMethod is a helper to register a method in the registry.
// sig is the method's Func signature (nil for property-style entries that
// only need to exist for member lookup but have no meaningful type).
func regMethod(reg *typresolve.Registry, receiverQN string, methodName string, sig *typresolve.Type) {
	qn := receiverQN + "." + methodName
	reg.AddFunc(typresolve.RegisteredFunc{
		QualifiedName: qn,
		ShortName:     methodName,
		ReceiverType:  receiverQN,
		Signature:     sig,
		MinParams:     -1,
	})
}

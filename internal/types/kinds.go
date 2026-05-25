package types

// Node kind constants. Use these instead of raw string literals.
const (
	KindFunction  = "function"
	KindMethod    = "method"
	KindType      = "type"
	KindInterface = "interface"
	KindConst     = "const"
	KindVar       = "var"
	KindService   = "service"
	KindRoute     = "route_handler"
	KindExternal  = "external"
	KindFile      = "file"
	KindPackage   = "package"
	KindField     = "field"
	KindEnvVar    = "env_var"
	KindProcess   = "process"
)

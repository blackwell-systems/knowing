package context

// universalEquivalenceClasses returns concepts common to virtually all software
// projects. These work on any Go/TS/Python/Rust/Java codebase without
// repo-specific knowledge. Separate from knowing-specific seeds so they can
// ship as the default for any repo.
func universalEquivalenceClasses() []EquivalenceClass {
	seeds := []EquivalenceClass{
		// Architecture / structure
		{
			Concept:    "ENTRY_POINT",
			Phrases:    []string{"entry point", "main function", "program start", "application bootstrap", "startup", "initialization"},
			Targets:    []string{"main", "Main", "init", "Init", "Start", "Run", "Bootstrap", "Setup"},
			TargetType: "symbol",
			Weight:     0.8,
			Source:     "universal",
		},
		{
			Concept:    "CONFIGURATION",
			Phrases:    []string{"configuration", "config", "settings", "options", "environment variables", "env vars", "flags", "parameters"},
			Targets:    []string{"Config", "Configuration", "Settings", "Options", "NewConfig", "LoadConfig", "ParseFlags"},
			TargetType: "symbol",
			Weight:     0.8,
			Source:     "universal",
		},
		{
			Concept:    "ERROR_HANDLING",
			Phrases:    []string{"error handling", "error types", "error wrapping", "custom errors", "error codes", "panic recovery"},
			Targets:    []string{"Error", "NewError", "Wrap", "Unwrap", "HandleError", "ErrorHandler", "Recovery"},
			TargetType: "symbol",
			Weight:     0.8,
			Source:     "universal",
		},
		{
			Concept:    "LOGGING",
			Phrases:    []string{"logging", "log output", "structured logging", "log levels", "debug logging", "trace logging"},
			Targets:    []string{"Logger", "Log", "NewLogger", "Debug", "Info", "Warn", "Error", "Printf"},
			TargetType: "symbol",
			Weight:     0.8,
			Source:     "universal",
		},
		{
			Concept:    "MIDDLEWARE",
			Phrases:    []string{"middleware", "interceptor", "handler chain", "request pipeline", "pre-processing", "post-processing"},
			Targets:    []string{"Middleware", "Interceptor", "Handler", "Chain", "Wrap", "Use"},
			TargetType: "symbol",
			Weight:     0.8,
			Source:     "universal",
		},

		// Data / storage
		{
			Concept:    "DATABASE",
			Phrases:    []string{"database", "db connection", "query", "sql", "data store", "persistence", "repository", "data access"},
			Targets:    []string{"DB", "Database", "Store", "Repository", "Query", "Connect", "Open", "Close", "Migrate"},
			TargetType: "symbol",
			Weight:     0.8,
			Source:     "universal",
		},
		{
			Concept:    "CACHING",
			Phrases:    []string{"cache", "caching", "memoization", "lru", "ttl", "invalidation", "cache miss", "cache hit"},
			Targets:    []string{"Cache", "LRU", "TTL", "Get", "Set", "Invalidate", "Evict", "NewCache"},
			TargetType: "symbol",
			Weight:     0.8,
			Source:     "universal",
		},
		{
			Concept:    "SERIALIZATION",
			Phrases:    []string{"serialization", "marshal", "unmarshal", "encode", "decode", "json", "protobuf", "wire format"},
			Targets:    []string{"Marshal", "Unmarshal", "Encode", "Decode", "Serialize", "Deserialize", "Codec", "Payload"},
			TargetType: "symbol",
			Weight:     0.8,
			Source:     "universal",
		},

		// HTTP / networking
		{
			Concept:    "HTTP_SERVER",
			Phrases:    []string{"http server", "web server", "api server", "rest api", "listen and serve", "router", "routes"},
			Targets:    []string{"Server", "Router", "Mux", "ListenAndServe", "HandleFunc", "Route", "Endpoint"},
			TargetType: "symbol",
			Weight:     0.8,
			Source:     "universal",
		},
		{
			Concept:    "HTTP_CLIENT",
			Phrases:    []string{"http client", "api client", "fetch", "request", "make request", "call api", "http call"},
			Targets:    []string{"Client", "HTTPClient", "Request", "Do", "Get", "Post", "Fetch", "Call"},
			TargetType: "symbol",
			Weight:     0.8,
			Source:     "universal",
		},
		{
			Concept:    "AUTHENTICATION",
			Phrases:    []string{"authentication", "auth", "login", "credentials", "token", "jwt", "session", "identity"},
			Targets:    []string{"Auth", "Authenticate", "Login", "Token", "JWT", "Session", "Verify", "ValidateToken"},
			TargetType: "symbol",
			Weight:     0.8,
			Source:     "universal",
		},

		// Testing
		{
			Concept:    "TESTING",
			Phrases:    []string{"test", "unit test", "integration test", "test suite", "test helper", "test fixture", "assertion"},
			Targets:    []string{"Test", "Suite", "Assert", "Setup", "Teardown", "Mock", "Fixture", "Helper"},
			TargetType: "symbol",
			Weight:     0.8,
			Source:     "universal",
		},

		// Concurrency
		{
			Concept:    "CONCURRENCY",
			Phrases:    []string{"concurrency", "goroutine", "thread", "parallel", "async", "worker pool", "channel", "mutex", "synchronization"},
			Targets:    []string{"Worker", "Pool", "Mutex", "Lock", "Channel", "Wait", "WaitGroup", "Spawn", "Run"},
			TargetType: "symbol",
			Weight:     0.8,
			Source:     "universal",
		},

		// Code organization
		{
			Concept:    "INTERFACE_CONTRACT",
			Phrases:    []string{"interface", "contract", "abstraction", "protocol", "trait", "implements", "adapter", "wrapper"},
			Targets:    []string{"Interface", "Contract", "Adapter", "Wrapper", "Implements", "Provider"},
			TargetType: "symbol",
			Weight:     0.8,
			Source:     "universal",
		},
		{
			Concept:    "FACTORY",
			Phrases:    []string{"factory", "constructor", "builder", "create instance", "new instance", "instantiate"},
			Targets:    []string{"New", "Create", "Build", "Factory", "Builder", "Make"},
			TargetType: "symbol",
			Weight:     0.8,
			Source:     "universal",
		},
		{
			Concept:    "EVENT_SYSTEM",
			Phrases:    []string{"event", "pub sub", "publish subscribe", "emit", "listener", "observer", "handler", "callback", "hook"},
			Targets:    []string{"Event", "Emit", "Subscribe", "Publish", "Listen", "Observer", "Handler", "Hook", "Callback"},
			TargetType: "symbol",
			Weight:     0.8,
			Source:     "universal",
		},
		{
			Concept:    "VALIDATION",
			Phrases:    []string{"validation", "validate", "check", "verify", "sanitize", "constraint", "rules"},
			Targets:    []string{"Validate", "Check", "Verify", "Sanitize", "Rules", "Constraint", "Valid"},
			TargetType: "symbol",
			Weight:     0.8,
			Source:     "universal",
		},
		{
			Concept:    "MIGRATION",
			Phrases:    []string{"migration", "schema migration", "database migration", "upgrade", "versioning", "schema change"},
			Targets:    []string{"Migrate", "Migration", "Upgrade", "Schema", "Version"},
			TargetType: "symbol",
			Weight:     0.8,
			Source:     "universal",
		},
		{
			Concept:    "CLI",
			Phrases:    []string{"command line", "cli", "subcommand", "flags", "arguments", "usage", "help"},
			Targets:    []string{"Command", "Cmd", "Run", "Execute", "Parse", "Flag", "Args"},
			TargetType: "symbol",
			Weight:     0.8,
			Source:     "universal",
		},
		{
			Concept:    "FILE_IO",
			Phrases:    []string{"file io", "read file", "write file", "file system", "directory", "path", "glob", "walk"},
			Targets:    []string{"Read", "Write", "Open", "Close", "Walk", "Glob", "ReadFile", "WriteFile", "Path"},
			TargetType: "symbol",
			Weight:     0.8,
			Source:     "universal",
		},
	}

	// Expand with action verbs.
	for i := range seeds {
		seeds[i].Phrases = expandWithVerbs(seeds[i].Phrases)
	}

	return seeds
}

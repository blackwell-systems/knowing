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
			Phrases:    []string{"http client", "api client", "make request", "call api", "http call"},
			Targets:    []string{"Client", "HTTPClient", "Request", "Fetch"},
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

		// Security / authorization
		{
			Concept:    "AUTHORIZATION",
			Phrases:    []string{"authorization", "permissions", "access control", "rbac", "acl", "role based", "who can access", "allowed", "forbidden"},
			Targets:    []string{"Authorize", "Permission", "Role", "ACL", "RBAC", "Policy", "Allow", "Deny", "Can", "IsAllowed"},
			TargetType: "symbol",
			Weight:     0.8,
			Source:     "universal",
		},
		{
			Concept:    "ENCRYPTION",
			Phrases:    []string{"encryption", "encrypt", "decrypt", "cipher", "aes", "rsa", "hash password", "bcrypt", "crypto"},
			Targets:    []string{"Encrypt", "Decrypt", "Hash", "Sign", "Verify", "Cipher", "Key", "GenerateKey", "Bcrypt"},
			TargetType: "symbol",
			Weight:     0.8,
			Source:     "universal",
		},
		{
			Concept:    "SECRETS_MANAGEMENT",
			Phrases:    []string{"secrets", "secret management", "vault", "credentials", "api key", "env secret", "key rotation"},
			Targets:    []string{"Secret", "Vault", "Credential", "APIKey", "KeyStore", "GetSecret", "RotateKey"},
			TargetType: "symbol",
			Weight:     0.8,
			Source:     "universal",
		},
		{
			Concept:    "CORS",
			Phrases:    []string{"cors", "cross origin", "allowed origins", "preflight", "access control allow"},
			Targets:    []string{"CORS", "CORSMiddleware", "AllowOrigins", "Preflight", "CORSHandler"},
			TargetType: "symbol",
			Weight:     0.8,
			Source:     "universal",
		},
		{
			Concept:    "CSRF_PROTECTION",
			Phrases:    []string{"csrf", "cross site request forgery", "csrf token", "anti forgery", "request verification"},
			Targets:    []string{"CSRF", "CSRFToken", "CSRFMiddleware", "VerifyToken", "AntiForgery"},
			TargetType: "symbol",
			Weight:     0.8,
			Source:     "universal",
		},
		{
			Concept:    "INPUT_SANITIZATION",
			Phrases:    []string{"sanitize input", "escape html", "xss prevention", "sql injection", "input filtering", "safe string"},
			Targets:    []string{"Sanitize", "Escape", "EscapeHTML", "Clean", "Filter", "Strip", "SafeString"},
			TargetType: "symbol",
			Weight:     0.8,
			Source:     "universal",
		},

		// Monitoring / observability
		{
			Concept:    "METRICS",
			Phrases:    []string{"metrics", "prometheus", "counter", "histogram", "gauge", "metric recording", "instrumentation"},
			Targets:    []string{"Metric", "Counter", "Histogram", "Gauge", "Record", "Observe", "Increment", "MetricsHandler"},
			TargetType: "symbol",
			Weight:     0.8,
			Source:     "universal",
		},
		{
			Concept:    "TRACING",
			Phrases:    []string{"distributed tracing", "span", "trace context", "opentelemetry", "jaeger", "trace id", "propagation"},
			Targets:    []string{"Span", "Trace", "StartSpan", "EndSpan", "TraceID", "Propagator", "TracerProvider"},
			TargetType: "symbol",
			Weight:     0.8,
			Source:     "universal",
		},
		{
			Concept:    "HEALTH_CHECK",
			Phrases:    []string{"health check", "readiness probe", "liveness probe", "heartbeat", "status endpoint", "service health"},
			Targets:    []string{"Health", "HealthCheck", "Ready", "Readiness", "Liveness", "Ping", "Status", "Heartbeat"},
			TargetType: "symbol",
			Weight:     0.8,
			Source:     "universal",
		},
		{
			Concept:    "AUDIT_LOGGING",
			Phrases:    []string{"audit log", "audit trail", "activity log", "who did what", "compliance log", "access log"},
			Targets:    []string{"AuditLog", "Audit", "LogActivity", "RecordEvent", "Trail", "AuditEntry"},
			TargetType: "symbol",
			Weight:     0.8,
			Source:     "universal",
		},

		// Resilience / reliability
		{
			Concept:    "RETRY_LOGIC",
			Phrases:    []string{"retry", "backoff", "exponential backoff", "retry policy", "attempt", "retryable"},
			Targets:    []string{"Retry", "Backoff", "RetryPolicy", "WithRetry", "Attempt", "ExponentialBackoff"},
			TargetType: "symbol",
			Weight:     0.8,
			Source:     "universal",
		},
		{
			Concept:    "CIRCUIT_BREAKER",
			Phrases:    []string{"circuit breaker", "breaker", "fault tolerance", "fail fast", "half open", "trip"},
			Targets:    []string{"CircuitBreaker", "Breaker", "Trip", "HalfOpen", "Reset", "NewBreaker"},
			TargetType: "symbol",
			Weight:     0.8,
			Source:     "universal",
		},
		{
			Concept:    "RATE_LIMITING",
			Phrases:    []string{"rate limit", "throttle", "rate limiter", "requests per second", "quota", "token bucket", "leaky bucket"},
			Targets:    []string{"RateLimiter", "Throttle", "Limit", "TokenBucket", "Allow", "Wait", "Limiter"},
			TargetType: "symbol",
			Weight:     0.8,
			Source:     "universal",
		},
		{
			Concept:    "TIMEOUT",
			Phrases:    []string{"timeout", "deadline", "context timeout", "request timeout", "cancel", "context deadline"},
			Targets:    []string{"Timeout", "Deadline", "WithTimeout", "WithDeadline", "Cancel", "CancelFunc"},
			TargetType: "symbol",
			Weight:     0.8,
			Source:     "universal",
		},
		{
			Concept:    "GRACEFUL_SHUTDOWN",
			Phrases:    []string{"graceful shutdown", "signal handling", "sigterm", "cleanup", "drain connections", "shutdown hook"},
			Targets:    []string{"Shutdown", "GracefulStop", "Drain", "Cleanup", "OnShutdown", "Signal", "NotifyContext"},
			TargetType: "symbol",
			Weight:     0.8,
			Source:     "universal",
		},

		// API design
		{
			Concept:    "PAGINATION",
			Phrases:    []string{"pagination", "paginate", "page size", "cursor", "offset limit", "next page", "list results"},
			Targets:    []string{"Paginate", "Page", "PageSize", "Cursor", "Offset", "Limit", "NextPage", "PageToken"},
			TargetType: "symbol",
			Weight:     0.8,
			Source:     "universal",
		},
		{
			Concept:    "API_VERSIONING",
			Phrases:    []string{"api version", "versioning", "v1 v2", "api compatibility", "deprecation", "api migration"},
			Targets:    []string{"Version", "APIVersion", "Deprecated", "V1", "V2", "Versioned"},
			TargetType: "symbol",
			Weight:     0.8,
			Source:     "universal",
		},
		{
			Concept:    "WEBHOOK",
			Phrases:    []string{"webhook", "callback url", "event notification", "http callback", "webhook handler", "webhook delivery"},
			Targets:    []string{"Webhook", "WebhookHandler", "Deliver", "Callback", "Notify", "ProcessWebhook"},
			TargetType: "symbol",
			Weight:     0.8,
			Source:     "universal",
		},
		{
			Concept:    "WEBSOCKET",
			Phrases:    []string{"websocket", "realtime", "ws connection", "socket", "push notification", "bidirectional", "live update"},
			Targets:    []string{"WebSocket", "Socket", "Conn", "Hub", "Broadcast", "Upgrade", "OnMessage", "Send"},
			TargetType: "symbol",
			Weight:     0.8,
			Source:     "universal",
		},

		// Data processing
		{
			Concept:    "DATA_TRANSFORMATION",
			Phrases:    []string{"transform data", "etl", "pipeline", "map reduce", "data processing", "convert", "normalize"},
			Targets:    []string{"Transform", "Convert", "Normalize", "Pipeline", "Process", "Map", "Reduce", "Filter"},
			TargetType: "symbol",
			Weight:     0.8,
			Source:     "universal",
		},
		{
			Concept:    "QUEUE_PROCESSING",
			Phrases:    []string{"job queue", "worker", "background job", "task queue", "message consumer", "process queue", "dequeue"},
			Targets:    []string{"Worker", "Queue", "Job", "Enqueue", "Dequeue", "Process", "Consumer", "Dispatch"},
			TargetType: "symbol",
			Weight:     0.8,
			Source:     "universal",
		},
		{
			Concept:    "SCHEDULING",
			Phrases:    []string{"scheduler", "cron", "scheduled task", "periodic", "timer", "recurring job", "cron expression"},
			Targets:    []string{"Scheduler", "Cron", "Schedule", "Timer", "Tick", "Every", "AddJob", "CronJob"},
			TargetType: "symbol",
			Weight:     0.8,
			Source:     "universal",
		},
		{
			Concept:    "SEARCH",
			Phrases:    []string{"search", "full text search", "query builder", "filter", "index", "search engine", "elasticsearch"},
			Targets:    []string{"Search", "Query", "Index", "Find", "Lookup", "FullTextSearch", "Match", "SearchResult"},
			TargetType: "symbol",
			Weight:     0.8,
			Source:     "universal",
		},

		// Storage patterns
		{
			Concept:    "TRANSACTION",
			Phrases:    []string{"transaction", "begin commit", "rollback", "atomic operation", "transactional", "unit of work"},
			Targets:    []string{"Transaction", "Tx", "Begin", "Commit", "Rollback", "WithTx", "Atomic"},
			TargetType: "symbol",
			Weight:     0.8,
			Source:     "universal",
		},
		{
			Concept:    "ORM_MODEL",
			Phrases:    []string{"orm", "model", "entity", "active record", "data model", "schema definition", "table mapping"},
			Targets:    []string{"Model", "Entity", "Schema", "Table", "Column", "Field", "Migrate", "AutoMigrate"},
			TargetType: "symbol",
			Weight:     0.8,
			Source:     "universal",
		},
		{
			Concept:    "CONNECTION_POOL",
			Phrases:    []string{"connection pool", "pool", "max connections", "idle connections", "pool size", "acquire release"},
			Targets:    []string{"Pool", "ConnectionPool", "MaxOpen", "MaxIdle", "Acquire", "Release", "NewPool"},
			TargetType: "symbol",
			Weight:     0.8,
			Source:     "universal",
		},
		{
			Concept:    "FILE_UPLOAD",
			Phrases:    []string{"file upload", "multipart", "upload handler", "form file", "save file", "attachment"},
			Targets:    []string{"Upload", "MultipartForm", "SaveFile", "FormFile", "Attachment", "HandleUpload"},
			TargetType: "symbol",
			Weight:     0.8,
			Source:     "universal",
		},

		// Notifications
		{
			Concept:    "NOTIFICATION",
			Phrases:    []string{"notification", "notify", "alert", "email notification", "push notification", "send email", "sms"},
			Targets:    []string{"Notify", "Notification", "Alert", "SendEmail", "Send", "Push", "Notifier", "Mailer"},
			TargetType: "symbol",
			Weight:     0.8,
			Source:     "universal",
		},
		{
			Concept:    "TEMPLATING",
			Phrases:    []string{"template", "render", "html template", "template engine", "mustache", "handlebars", "view"},
			Targets:    []string{"Template", "Render", "Execute", "Parse", "View", "RenderHTML", "TemplateFuncs"},
			TargetType: "symbol",
			Weight:     0.8,
			Source:     "universal",
		},

		// Infrastructure / deployment
		{
			Concept:    "CI_CD",
			Phrases:    []string{"ci cd", "continuous integration", "deployment", "build pipeline", "github actions", "deploy"},
			Targets:    []string{"Deploy", "Build", "Pipeline", "Release", "Publish", "Stage", "Promote"},
			TargetType: "symbol",
			Weight:     0.8,
			Source:     "universal",
		},
		{
			Concept:    "CONTAINER",
			Phrases:    []string{"docker", "container", "dockerfile", "image build", "container runtime", "kubernetes pod"},
			Targets:    []string{"Container", "Image", "Dockerfile", "Build", "Run", "Pod", "Deployment"},
			TargetType: "symbol",
			Weight:     0.8,
			Source:     "universal",
		},
		{
			Concept:    "FEATURE_FLAG",
			Phrases:    []string{"feature flag", "feature toggle", "feature gate", "experiment", "a/b test", "rollout", "canary"},
			Targets:    []string{"FeatureFlag", "Toggle", "IsEnabled", "Gate", "Experiment", "Variant", "Rollout"},
			TargetType: "symbol",
			Weight:     0.8,
			Source:     "universal",
		},

		// Application patterns
		{
			Concept:    "DEPENDENCY_INJECTION",
			Phrases:    []string{"dependency injection", "di container", "inject", "wire", "provider", "service locator", "ioc"},
			Targets:    []string{"Inject", "Wire", "Provider", "Container", "Provide", "Resolve", "Register", "Bind"},
			TargetType: "symbol",
			Weight:     0.8,
			Source:     "universal",
		},
		{
			Concept:    "STATE_MANAGEMENT",
			Phrases:    []string{"state management", "state machine", "reducer", "store", "dispatch", "action", "fsm"},
			Targets:    []string{"State", "Store", "Dispatch", "Reducer", "Action", "Transition", "StateMachine", "FSM"},
			TargetType: "symbol",
			Weight:     0.8,
			Source:     "universal",
		},
		{
			Concept:    "PLUGIN_SYSTEM",
			Phrases:    []string{"plugin", "extension", "addon", "module system", "hook", "register plugin", "plugin loader"},
			Targets:    []string{"Plugin", "Extension", "Register", "Load", "Hook", "Addon", "PluginManager"},
			TargetType: "symbol",
			Weight:     0.8,
			Source:     "universal",
		},
		{
			Concept:    "ROUTING",
			Phrases:    []string{"routing", "route handler", "url pattern", "path parameter", "route matching", "dispatch request"},
			Targets:    []string{"Route", "Router", "Handle", "Match", "Dispatch", "PathParam", "Group", "Prefix"},
			TargetType: "symbol",
			Weight:     0.8,
			Source:     "universal",
		},
		{
			Concept:    "CONTEXT_PROPAGATION",
			Phrases:    []string{"context propagation", "request context", "trace context", "pass context", "context value", "with value"},
			Targets:    []string{"Context", "WithValue", "Value", "WithContext", "Background", "TODO", "Propagate"},
			TargetType: "symbol",
			Weight:     0.8,
			Source:     "universal",
		},
		{
			Concept:    "INTERNATIONALIZATION",
			Phrases:    []string{"internationalization", "i18n", "localization", "l10n", "translate", "locale", "language pack"},
			Targets:    []string{"Translate", "Locale", "I18n", "L10n", "Message", "Bundle", "Language", "GetText"},
			TargetType: "symbol",
			Weight:     0.8,
			Source:     "universal",
		},
		{
			Concept:    "COMPRESSION",
			Phrases:    []string{"compression", "gzip", "deflate", "compress", "decompress", "zip", "zlib"},
			Targets:    []string{"Compress", "Decompress", "Gzip", "Deflate", "NewReader", "NewWriter", "Zip", "Unzip"},
			TargetType: "symbol",
			Weight:     0.8,
			Source:     "universal",
		},
		{
			Concept:    "HOT_RELOAD",
			Phrases:    []string{"hot reload", "live reload", "watch mode", "auto restart", "file watch", "dev server"},
			Targets:    []string{"Watch", "Reload", "Restart", "HotReload", "LiveReload", "DevServer", "OnChange"},
			TargetType: "symbol",
			Weight:     0.8,
			Source:     "universal",
		},

		// Domain patterns
		{
			Concept:    "PAYMENT",
			Phrases:    []string{"payment", "checkout", "charge", "invoice", "billing", "subscription", "stripe"},
			Targets:    []string{"Payment", "Charge", "Invoice", "Checkout", "Subscribe", "Billing", "Refund"},
			TargetType: "symbol",
			Weight:     0.8,
			Source:     "universal",
		},
		{
			Concept:    "ANALYTICS",
			Phrases:    []string{"analytics", "tracking", "event tracking", "metrics collection", "usage stats", "telemetry data"},
			Targets:    []string{"Track", "Analytics", "Event", "Measure", "Report", "Collect", "Segment"},
			TargetType: "symbol",
			Weight:     0.8,
			Source:     "universal",
		},
		{
			Concept:    "GRAPH_TRAVERSAL",
			Phrases:    []string{"graph traversal", "bfs", "dfs", "tree walk", "breadth first", "depth first", "visit nodes"},
			Targets:    []string{"BFS", "DFS", "Walk", "Visit", "Traverse", "Neighbors", "Adjacent", "Queue", "Stack"},
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

package context

// webEquivalenceClasses returns equivalence classes for general web patterns.
func webEquivalenceClasses() []EquivalenceClass {
	return []EquivalenceClass{
		{
			Concept:    "WEB_CORS",
			Phrases:    []string{"cors", "cross origin", "allow origin", "preflight request"},
			Targets:    []string{"CORS", "CORSMiddleware", "AllowOrigins", "Preflight", "AccessControlAllowOrigin"},
			TargetType: "symbol",
			Weight:     0.9,
			Source:     "framework",
		},
		{
			Concept:    "WEB_WEBSOCKET",
			Phrases:    []string{"websocket connection", "websocket handler", "ws connection", "real-time connection"},
			Targets:    []string{"WebSocket", "WebSocketHandler", "Upgrader", "Conn", "OnMessage", "OnClose", "OnOpen"},
			TargetType: "symbol",
			Weight:     0.9,
			Source:     "framework",
		},
		{
			Concept:    "WEB_UPLOAD",
			Phrases:    []string{"file upload", "multipart upload", "upload file", "form file"},
			Targets:    []string{"UploadFile", "MultipartFile", "FormFile", "SaveFile", "ParseMultipartForm"},
			TargetType: "symbol",
			Weight:     0.9,
			Source:     "framework",
		},
		{
			Concept:    "WEB_CACHE",
			Phrases:    []string{"response cache", "http cache", "cache control", "etag", "cache middleware"},
			Targets:    []string{"Cache", "CacheControl", "ETag", "CacheMiddleware", "ResponseCache", "Invalidate"},
			TargetType: "symbol",
			Weight:     0.9,
			Source:     "framework",
		},
		{
			Concept:    "WEB_RATE_LIMIT",
			Phrases:    []string{"rate limit", "rate limiter", "throttle", "request limit", "token bucket"},
			Targets:    []string{"RateLimiter", "Throttle", "TokenBucket", "LeakyBucket", "RateLimit", "Limiter"},
			TargetType: "symbol",
			Weight:     0.9,
			Source:     "framework",
		},
		{
			Concept:    "WEB_LOGGING",
			Phrases:    []string{"request logging", "access log", "structured logging", "log middleware"},
			Targets:    []string{"Logger", "AccessLog", "RequestLogger", "LogMiddleware", "Slog", "Zap", "Logrus"},
			TargetType: "symbol",
			Weight:     0.9,
			Source:     "framework",
		},
	}
}

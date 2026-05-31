package context

// goEquivalenceClasses returns equivalence classes for go.
func goEquivalenceClasses() []EquivalenceClass {
	return []EquivalenceClass{
		{
		Concept:    "GO_HTTP_HANDLER",
		Phrases:    []string{"http handler", "request handler", "handler func", "serve http", "endpoint handler"},
		Targets:    []string{"Handler", "HandlerFunc", "ServeHTTP", "Handle", "HandleFunc", "ServeMux", "DefaultServeMux"},
		TargetType: "symbol",
		Weight:     0.9,
		Source:     "framework",
		Lang:       "go",
		},
		{
		Concept:    "GO_HTTP_MIDDLEWARE",
		Phrases:    []string{"http middleware", "middleware chain", "middleware handler", "wrap handler"},
		Targets:    []string{"Middleware", "Handler", "WrapHandler", "Chain", "Use", "With"},
		TargetType: "symbol",
		Weight:     0.9,
		Source:     "framework",
		Lang:       "go",
		},
		{
		Concept:    "GO_HTTP_SERVER",
		Phrases:    []string{"http server", "listen and serve", "tls server", "server config", "reverse proxy"},
		Targets:    []string{"Server", "ListenAndServe", "ListenAndServeTLS", "ReverseProxy", "Transport", "Client"},
		TargetType: "symbol",
		Weight:     0.9,
		Source:     "framework",
		Lang:       "go",
		},
		{
		Concept:    "GO_ROUTER",
		Phrases:    []string{"http router", "url routing", "path parameter", "route group", "mux router"},
		Targets:    []string{"Router", "Mux", "Route", "Group", "Param"},
		TargetType: "symbol",
		Weight:     0.9,
		Source:     "framework",
		Lang:       "go",
		},
	}
}

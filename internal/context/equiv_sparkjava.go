package context

// sparkjavaEquivalenceClasses returns equivalence classes for Spark Java web framework.
func sparkjavaEquivalenceClasses() []EquivalenceClass {
	return []EquivalenceClass{
		{
		Concept:    "SPARK_ROUTE",
		Phrases:    []string{"route handler", "url pattern", "path parameter", "route matching", "wildcard route"},
		Targets:    []string{"Routes", "Routes.find", "Routes.findTargetsForRequestedRoute", "RouteEntry", "RouteEntry.matches", "RouteMatch"},
		TargetType: "symbol",
		Weight:     0.9,
		Source:     "framework",
		Lang:       "java",
		},
		{
		Concept:    "SPARK_REQUEST",
		Phrases:    []string{"query parameter", "request parameter", "query string", "request body"},
		Targets:    []string{"Request.queryParams", "Request.queryParamsValues", "Request.queryMap", "QueryParamsMap", "Request.body"},
		TargetType: "symbol",
		Weight:     0.9,
		Source:     "framework",
		Lang:       "java",
		},
		{
		Concept:    "SPARK_RESPONSE",
		Phrases:    []string{"response transformer", "serialize response", "json response", "response body"},
		Targets:    []string{"ResponseTransformer", "ResponseTransformer.render", "ResponseTransformerRouteImpl", "Response.body"},
		TargetType: "symbol",
		Weight:     0.9,
		Source:     "framework",
		Lang:       "java",
		},
		{
		Concept:    "SPARK_HALT",
		Phrases:    []string{"halt request", "stop processing", "abort request", "halt exception"},
		Targets:    []string{"Service.halt", "HaltException", "HaltException.getStatusCode", "HaltException.getBody"},
		TargetType: "symbol",
		Weight:     0.9,
		Source:     "framework",
		Lang:       "java",
		},
		{
		Concept:    "SPARK_REDIRECT",
		Phrases:    []string{"url redirect", "redirect route", "redirect response"},
		Targets:    []string{"Redirect", "Redirect.get", "Redirect.any", "Response.redirect"},
		TargetType: "symbol",
		Weight:     0.9,
		Source:     "framework",
		Lang:       "java",
		},
		{
		Concept:    "SPARK_PIPELINE",
		Phrases:    []string{"request pipeline", "request flow", "jetty handler", "filter chain", "before filter", "after filter"},
		Targets:    []string{"JettyHandler", "MatcherFilter", "BeforeFilters", "Routes.execute", "AfterFilters", "AfterAfterFilters"},
		TargetType: "symbol",
		Weight:     0.9,
		Source:     "framework",
		Lang:       "java",
		},
		{
		Concept:    "SPARK_SSL",
		Phrases:    []string{"https ssl", "keystore", "truststore", "mutual tls", "secure connection"},
		Targets:    []string{"Service.secure", "SslStores", "SslStores.create", "SocketConnectorFactory", "EmbeddedJettyServer"},
		TargetType: "symbol",
		Weight:     0.9,
		Source:     "framework",
		Lang:       "java",
		},
		{
		Concept:    "SPARK_COOKIE",
		Phrases:    []string{"http cookie", "set cookie", "read cookie", "remove cookie"},
		Targets:    []string{"Request.cookies", "Request.cookie", "Response.cookie", "Response.removeCookie"},
		TargetType: "symbol",
		Weight:     0.9,
		Source:     "framework",
		Lang:       "java",
		},
	}
}

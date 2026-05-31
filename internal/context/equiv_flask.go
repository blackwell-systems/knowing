package context

// flaskEquivalenceClasses returns equivalence classes for flask.
func flaskEquivalenceClasses() []EquivalenceClass {
	return []EquivalenceClass{
		{
		Concept:    "FLASK_BLUEPRINT",
		Phrases:    []string{"flask blueprint", "blueprint registration", "register blueprint", "nested blueprint", "url prefix"},
		Targets:    []string{"Blueprint", "register_blueprint", "BlueprintSetupState", "add_url_rule"},
		TargetType: "symbol",
		Weight:     0.9,
		Source:     "framework",
		Lang:       "python",
		},
		{
		Concept:    "FLASK_HOOKS",
		Phrases:    []string{"before_request", "after_request", "request hook", "teardown", "before request hook"},
		Targets:    []string{"before_request", "after_request", "teardown_request", "teardown_appcontext", "before_app_request", "after_app_request"},
		TargetType: "symbol",
		Weight:     0.9,
		Source:     "framework",
		Lang:       "python",
		},
		{
		Concept:    "FLASK_EXTENSION",
		Phrases:    []string{"flask extension", "application factory", "app factory", "create_app", "init_app"},
		Targets:    []string{"create_app", "init_app", "Flask", "current_app", "app_context", "application_factory"},
		TargetType: "symbol",
		Weight:     0.9,
		Source:     "framework",
		Lang:       "python",
		},
		{
		Concept:    "FLASK_SESSION",
		Phrases:    []string{"flask session", "session backend", "session cookie", "server-side session"},
		Targets:    []string{"SecureCookieSession", "SessionInterface", "NullSession", "SecureCookieSessionInterface", "session"},
		TargetType: "symbol",
		Weight:     0.9,
		Source:     "framework",
		Lang:       "python",
		},
		{
		Concept:    "FLASK_JINJA",
		Phrases:    []string{"template filter", "jinja filter", "jinja2 filter", "custom filter", "template tag"},
		Targets:    []string{"template_filter", "add_template_filter", "Environment", "Template"},
		TargetType: "symbol",
		Weight:     0.9,
		Source:     "framework",
		Lang:       "python",
		},
		{
		Concept:    "FLASK_ERROR",
		Phrases:    []string{"error handler", "flask error", "custom error handler", "errorhandler", "exception handler"},
		Targets:    []string{"errorhandler", "app_errorhandler", "register_error_handler", "handle_exception", "HTTPException", "InternalServerError"},
		TargetType: "symbol",
		Weight:     0.9,
		Source:     "framework",
		Lang:       "python",
		},
		{
		Concept:    "FLASK_CLI",
		Phrases:    []string{"flask cli", "flask command", "click command", "cli command", "custom command"},
		Targets:    []string{"AppGroup", "FlaskGroup", "cli", "command", "with_appcontext"},
		TargetType: "symbol",
		Weight:     0.9,
		Source:     "framework",
		Lang:       "python",
		},
		{
		Concept:    "FLASK_SIGNALS",
		Phrases:    []string{"flask signal", "request_started", "request_finished", "request signal"},
		Targets:    []string{"request_started", "request_finished", "request_tearing_down", "appcontext_pushed", "appcontext_popped", "got_request_exception"},
		TargetType: "symbol",
		Weight:     0.9,
		Source:     "framework",
		Lang:       "python",
		},
	}
}

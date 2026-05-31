package context

// pythonEquivalenceClasses returns equivalence classes for python.
func pythonEquivalenceClasses() []EquivalenceClass {
	return []EquivalenceClass{
		{
		Concept:    "PY_ENTRY_POINT",
		Phrases:    []string{"entry point", "app factory", "application", "wsgi", "asgi", "startup"},
		Targets:    []string{"create_app", "app", "application", "wsgi_app", "asgi_app", "__main__", "main"},
		TargetType: "symbol",
		Weight:     0.8,
		Source:     "language",
		},
		{
		Concept:    "PY_ROUTING",
		Phrases:    []string{"route", "url", "endpoint", "view", "handler", "request handler", "url pattern"},
		Targets:    []string{"route", "urlpatterns", "path", "url_for", "add_url_rule", "blueprint", "Blueprint", "before_request", "after_request", "app_errorhandler"},
		TargetType: "symbol",
		Weight:     0.8,
		Source:     "language",
		},
		{
		Concept:    "PY_MIDDLEWARE",
		Phrases:    []string{"middleware", "request processing", "before request", "after request", "hook", "interceptor"},
		Targets:    []string{"before_request", "after_request", "before_app_request", "process_request", "process_response", "process_view", "process_exception", "teardown_request", "middleware"},
		TargetType: "symbol",
		Weight:     0.8,
		Source:     "language",
		},
		{
		Concept:    "PY_ORM",
		Phrases:    []string{"database", "query", "model", "orm", "queryset", "migration", "schema"},
		Targets:    []string{"QuerySet", "Manager", "Model", "objects", "filter", "exclude", "annotate", "aggregate", "get_queryset", "Meta", "ForeignKey", "ManyToManyField", "migration", "RunSQL", "RunPython"},
		TargetType: "symbol",
		Weight:     0.8,
		Source:     "language",
		},
		{
		Concept:    "PY_SERIALIZATION",
		Phrases:    []string{"serialization", "serializer", "schema", "validation", "form", "marshal"},
		Targets:    []string{"Serializer", "ModelSerializer", "Schema", "Form", "ModelForm", "Field", "validate", "clean", "to_representation", "to_internal_value"},
		TargetType: "symbol",
		Weight:     0.8,
		Source:     "language",
		},
		{
		Concept:    "PY_AUTH",
		Phrases:    []string{"authentication", "authorization", "login", "permission", "session", "user"},
		Targets:    []string{"authenticate", "login", "logout", "login_required", "permission_required", "is_authenticated", "get_user", "AnonymousUser", "AbstractUser", "PermissionMixin", "has_perm"},
		TargetType: "symbol",
		Weight:     0.8,
		Source:     "language",
		},
		{
		Concept:    "PY_TEMPLATE",
		Phrases:    []string{"template", "render", "view", "context", "response"},
		Targets:    []string{"render_template", "render", "render_to_response", "TemplateView", "get_context_data", "template_name", "get_template_names"},
		TargetType: "symbol",
		Weight:     0.8,
		Source:     "language",
		},
		{
		Concept:    "PY_ERROR",
		Phrases:    []string{"error", "exception", "error handling", "abort", "error handler"},
		Targets:    []string{"HTTPException", "ValidationError", "abort", "errorhandler", "handle_exception", "app_errorhandler", "Http404", "PermissionDenied", "SuspiciousOperation"},
		TargetType: "symbol",
		Weight:     0.8,
		Source:     "language",
		},
		{
		Concept:    "PY_CONFIG",
		Phrases:    []string{"configuration", "settings", "config", "environment"},
		Targets:    []string{"settings", "config", "from_object", "from_envvar", "INSTALLED_APPS", "MIDDLEWARE", "DATABASES", "SECRET_KEY", "DEBUG", "BaseSettings"},
		TargetType: "symbol",
		Weight:     0.8,
		Source:     "language",
		},
		{
		Concept:    "PY_TESTING",
		Phrases:    []string{"test", "testing", "fixture", "mock", "assert"},
		Targets:    []string{"TestCase", "SimpleTestCase", "TransactionTestCase", "setUp", "tearDown", "client", "RequestFactory", "pytest", "fixture", "parametrize", "mock", "patch"},
		TargetType: "symbol",
		Weight:     0.8,
		Source:     "language",
		},
	}
}

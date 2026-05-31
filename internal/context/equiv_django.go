package context

// djangoEquivalenceClasses returns equivalence classes for django.
func djangoEquivalenceClasses() []EquivalenceClass {
	return []EquivalenceClass{
		{
		Concept:    "DJANGO_VALIDATORS",
		Phrases:    []string{"field validator", "email validation", "validate email", "custom validator", "input validation", "form validation"},
		Targets:    []string{"EmailValidator", "BaseValidator", "RegexValidator", "URLValidator", "ValidationError", "MaxValueValidator", "MinValueValidator"},
		TargetType: "symbol",
		Weight:     0.9,
		Source:     "framework",
		Lang:       "python",
		},
		{
		Concept:    "DJANGO_TEMPLATE_TAGS",
		Phrases:    []string{"template filter", "template tag", "custom filter", "custom tag", "templatetags"},
		Targets:    []string{"Library", "Library.filter", "Library.simple_tag", "Library.inclusion_tag", "FilterExpression", "register"},
		TargetType: "symbol",
		Weight:     0.9,
		Source:     "framework",
		Lang:       "python",
		},
		{
		Concept:    "DJANGO_MIDDLEWARE",
		Phrases:    []string{"django middleware", "request middleware", "custom middleware", "middleware class", "middleware chain"},
		Targets:    []string{"MiddlewareMixin", "SecurityMiddleware", "BaseHandler", "get_response", "process_request", "process_response", "CsrfViewMiddleware", "SessionMiddleware"},
		TargetType: "symbol",
		Weight:     0.9,
		Source:     "framework",
		Lang:       "python",
		},
		{
		Concept:    "DJANGO_CUSTOM_FIELD",
		Phrases:    []string{"custom field", "model field", "custom model field", "field type", "database field"},
		Targets:    []string{"Field", "CharField", "Field.deconstruct", "Field.from_db_value", "Field.get_prep_value", "Field.contribute_to_class", "ModelState"},
		TargetType: "symbol",
		Weight:     0.9,
		Source:     "framework",
		Lang:       "python",
		},
		{
		Concept:    "DJANGO_MANAGEMENT_CMD",
		Phrases:    []string{"management command", "django command", "custom command", "cli command"},
		Targets:    []string{"BaseCommand", "BaseCommand.handle", "BaseCommand.add_arguments", "OutputWrapper", "call_command", "CommandParser"},
		TargetType: "symbol",
		Weight:     0.9,
		Source:     "framework",
		Lang:       "python",
		},
		{
		Concept:    "DJANGO_SIGNALS",
		Phrases:    []string{"signal", "django signal", "post save", "pre save", "signal handler", "receiver"},
		Targets:    []string{"Signal", "Signal.send", "Signal.connect", "receiver", "post_save", "pre_save", "post_delete", "pre_delete", "request_finished"},
		TargetType: "symbol",
		Weight:     0.9,
		Source:     "framework",
		Lang:       "python",
		},
		{
		Concept:    "DJANGO_DB_BACKEND",
		Phrases:    []string{"database backend", "database connection", "database wrapper", "connection pool", "database cursor"},
		Targets:    []string{"BaseDatabaseWrapper", "BaseDatabaseWrapper.cursor", "BaseDatabaseWrapper.ensure_connection", "ConnectionHandler", "ConnectionRouter", "BaseDatabaseCreation", "BaseDatabaseOperations"},
		TargetType: "symbol",
		Weight:     0.9,
		Source:     "framework",
		Lang:       "python",
		},
		{
		Concept:    "DJANGO_MIGRATIONS",
		Phrases:    []string{"migration", "schema migration", "database migration", "alter field", "add field", "migrate"},
		Targets:    []string{"Operation", "Operation.state_forwards", "Operation.database_forwards", "AlterField", "AddField", "ProjectState", "MigrationExecutor", "BaseDatabaseSchemaEditor"},
		TargetType: "symbol",
		Weight:     0.9,
		Source:     "framework",
		Lang:       "python",
		},
	}
}

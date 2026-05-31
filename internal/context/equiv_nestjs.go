package context

// nestjsEquivalenceClasses returns equivalence classes for nestjs.
func nestjsEquivalenceClasses() []EquivalenceClass {
	return []EquivalenceClass{
		{
		Concept:    "NESTJS_MODULE",
		Phrases:    []string{"nest module", "nestjs module", "module provider", "module import"},
		Targets:    []string{"Module", "DynamicModule", "ModuleRef", "forRoot", "forRootAsync", "forFeature"},
		TargetType: "symbol",
		Weight:     0.9,
		Source:     "framework",
		Lang:       "typescript",
		},
		{
		Concept:    "NESTJS_GUARD",
		Phrases:    []string{"guard", "auth guard", "role guard", "canActivate"},
		Targets:    []string{"CanActivate", "AuthGuard", "RolesGuard", "ExecutionContext", "Reflector"},
		TargetType: "symbol",
		Weight:     0.9,
		Source:     "framework",
		Lang:       "typescript",
		},
		{
		Concept:    "NESTJS_PIPE",
		Phrases:    []string{"pipe", "validation pipe", "transform pipe", "parse pipe"},
		Targets:    []string{"PipeTransform", "ValidationPipe", "ParseIntPipe", "ParseUUIDPipe", "UsePipes"},
		TargetType: "symbol",
		Weight:     0.9,
		Source:     "framework",
		Lang:       "typescript",
		},
		{
		Concept:    "NESTJS_INTERCEPTOR",
		Phrases:    []string{"interceptor", "nestjs interceptor", "logging interceptor", "cache interceptor"},
		Targets:    []string{"NestInterceptor", "CallHandler", "UseInterceptors", "CacheInterceptor"},
		TargetType: "symbol",
		Weight:     0.9,
		Source:     "framework",
		Lang:       "typescript",
		},
	}
}

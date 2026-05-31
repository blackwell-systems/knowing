package context

// errorsEquivalenceClasses returns equivalence classes for error handling patterns.
func errorsEquivalenceClasses() []EquivalenceClass {
	return []EquivalenceClass{
		{
			Concept:    "ERROR_CUSTOM",
			Phrases:    []string{"custom exception", "custom error", "error type", "define error", "error class"},
			Targets:    []string{"Error", "Exception", "CustomError", "AppError", "DomainError", "ErrorCode"},
			TargetType: "symbol",
			Weight:     0.9,
			Source:     "framework",
		},
		{
			Concept:    "ERROR_HANDLER",
			Phrases:    []string{"error handler", "exception handler", "error middleware", "global error", "catch all errors"},
			Targets:    []string{"ErrorHandler", "ExceptionHandler", "ExceptionFilter", "HandleError", "Recover", "ErrorBoundary"},
			TargetType: "symbol",
			Weight:     0.9,
			Source:     "framework",
		},
		{
			Concept:    "ERROR_RETRY",
			Phrases:    []string{"retry logic", "retry policy", "backoff", "exponential backoff", "circuit breaker"},
			Targets:    []string{"Retry", "RetryPolicy", "Backoff", "ExponentialBackoff", "CircuitBreaker", "WithRetry"},
			TargetType: "symbol",
			Weight:     0.9,
			Source:     "framework",
		},
	}
}

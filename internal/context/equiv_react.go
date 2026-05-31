package context

// reactEquivalenceClasses returns equivalence classes for react.
func reactEquivalenceClasses() []EquivalenceClass {
	return []EquivalenceClass{
		{
		Concept:    "REACT_HOOKS",
		Phrases:    []string{"custom hook", "react hook", "use hook", "hook composition"},
		Targets:    []string{"useState", "useEffect", "useCallback", "useMemo", "useRef", "useContext", "useReducer"},
		TargetType: "symbol",
		Weight:     0.9,
		Source:     "framework",
		Lang:       "typescript",
		},
		{
		Concept:    "REACT_CONTEXT",
		Phrases:    []string{"react context", "context provider", "useContext", "context api"},
		Targets:    []string{"createContext", "useContext", "Provider", "Consumer", "ContextType"},
		TargetType: "symbol",
		Weight:     0.9,
		Source:     "framework",
		Lang:       "typescript",
		},
		{
		Concept:    "REACT_SUSPENSE",
		Phrases:    []string{"suspense", "lazy loading", "code splitting", "error boundary"},
		Targets:    []string{"Suspense", "lazy", "ErrorBoundary", "componentDidCatch", "getDerivedStateFromError"},
		TargetType: "symbol",
		Weight:     0.9,
		Source:     "framework",
		Lang:       "typescript",
		},
	}
}

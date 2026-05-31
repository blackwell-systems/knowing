package context

// angularEquivalenceClasses returns equivalence classes for angular.
func angularEquivalenceClasses() []EquivalenceClass {
	return []EquivalenceClass{
		{
		Concept:    "ANGULAR_COMPONENT",
		Phrases:    []string{"angular component", "component lifecycle", "ngOnInit", "change detection"},
		Targets:    []string{"Component", "OnInit", "OnDestroy", "OnChanges", "ChangeDetectorRef", "ViewChild", "Input", "Output"},
		TargetType: "symbol",
		Weight:     0.9,
		Source:     "framework",
		Lang:       "typescript",
		},
		{
		Concept:    "ANGULAR_SERVICE",
		Phrases:    []string{"angular service", "injectable", "dependency injection", "service provider"},
		Targets:    []string{"Injectable", "Inject", "InjectionToken", "Provider", "Injector"},
		TargetType: "symbol",
		Weight:     0.9,
		Source:     "framework",
		Lang:       "typescript",
		},
		{
		Concept:    "ANGULAR_PIPE",
		Phrases:    []string{"angular pipe", "custom pipe", "transform pipe", "async pipe"},
		Targets:    []string{"PipeTransform", "Pipe", "AsyncPipe", "DatePipe"},
		TargetType: "symbol",
		Weight:     0.9,
		Source:     "framework",
		Lang:       "typescript",
		},
		{
		Concept:    "ANGULAR_ROUTING",
		Phrases:    []string{"angular routing", "route guard", "route resolver", "lazy loading"},
		Targets:    []string{"CanActivate", "CanDeactivate", "Resolve", "RouterModule", "ActivatedRoute", "Router"},
		TargetType: "symbol",
		Weight:     0.9,
		Source:     "framework",
		Lang:       "typescript",
		},
	}
}

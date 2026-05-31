package context

// jekyllEquivalenceClasses returns equivalence classes for jekyll.
func jekyllEquivalenceClasses() []EquivalenceClass {
	return []EquivalenceClass{
		{
		Concept:    "JEKYLL_PLUGIN",
		Phrases:    []string{"jekyll plugin", "generator", "converter", "liquid tag", "jekyll hook"},
		Targets:    []string{"Generator", "Converter", "Tag", "Filter", "Hook", "Plugin"},
		TargetType: "symbol",
		Weight:     0.9,
		Source:     "framework",
		Lang:       "ruby",
		},
		{
		Concept:    "JEKYLL_SITE",
		Phrases:    []string{"jekyll site", "site generation", "build site", "site config"},
		Targets:    []string{"Site", "Site.process", "Site.render", "Site.write", "Configuration", "Reader"},
		TargetType: "symbol",
		Weight:     0.9,
		Source:     "framework",
		Lang:       "ruby",
		},
	}
}

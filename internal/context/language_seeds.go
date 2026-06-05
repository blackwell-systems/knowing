package context

// languageEquivalenceClasses aggregates all language and framework
// equivalence classes from per-file definitions.
func languageEquivalenceClasses() []EquivalenceClass {
	var all []EquivalenceClass
	all = append(all, angularEquivalenceClasses()...)
	all = append(all, csharpEquivalenceClasses()...)
	all = append(all, djangoEquivalenceClasses()...)
	all = append(all, fastapiEquivalenceClasses()...)
	all = append(all, flaskEquivalenceClasses()...)
	all = append(all, goEquivalenceClasses()...)
	all = append(all, javaEquivalenceClasses()...)
	all = append(all, jekyllEquivalenceClasses()...)
	all = append(all, kubernetesEquivalenceClasses()...)
	all = append(all, nestjsEquivalenceClasses()...)
	all = append(all, nextjsEquivalenceClasses()...)
	all = append(all, pythonEquivalenceClasses()...)
	all = append(all, railsEquivalenceClasses()...)
	all = append(all, reactEquivalenceClasses()...)
	all = append(all, rustEquivalenceClasses()...)
	all = append(all, terraformEquivalenceClasses()...)
	all = append(all, typescriptEquivalenceClasses()...)
	all = append(all, vscodeEquivalenceClasses()...)
	all = append(all, caddyEquivalenceClasses()...)
	all = append(all, cargoEquivalenceClasses()...)
	all = append(all, saleorEquivalenceClasses()...)
	all = append(all, sparkjavaEquivalenceClasses()...)
	// Cross-cutting patterns (framework-agnostic).
	all = append(all, containersEquivalenceClasses()...)
	all = append(all, cryptoEquivalenceClasses()...)
	all = append(all, testingEquivalenceClasses()...)
	all = append(all, ormEquivalenceClasses()...)
	all = append(all, authEquivalenceClasses()...)
	all = append(all, cliEquivalenceClasses()...)
	all = append(all, configEquivalenceClasses()...)
	all = append(all, errorsEquivalenceClasses()...)
	all = append(all, webEquivalenceClasses()...)
	return all
}

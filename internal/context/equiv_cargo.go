package context

// cargoEquivalenceClasses returns equivalence classes for Cargo (Rust build system).
func cargoEquivalenceClasses() []EquivalenceClass {
	return []EquivalenceClass{
		{
		Concept:    "CARGO_MANIFEST",
		Phrases:    []string{"cargo.toml", "manifest", "toml manifest", "configuration field", "cargo config"},
		Targets:    []string{"TomlManifest", "TomlProfile", "BuildConfig", "GlobalContext"},
		TargetType: "symbol",
		Weight:     0.9,
		Source:     "framework",
		},
		{
		Concept:    "CARGO_RESOLVE",
		Phrases:    []string{"dependency resolution", "resolve dependencies", "version resolution", "feature unification", "lockfile"},
		Targets:    []string{"FeatureResolver", "RequestedFeatures", "RegistryQueryer", "resolve", "resolve_std", "generate_lockfile", "Summary"},
		TargetType: "symbol",
		Weight:     0.9,
		Source:     "framework",
		},
		{
		Concept:    "CARGO_BUILD",
		Phrases:    []string{"build plan", "compilation order", "job queue", "build context", "parallel build"},
		Targets:    []string{"JobQueue", "JobQueue.execute", "BuildContext", "BuildConfig", "compile"},
		TargetType: "symbol",
		Weight:     0.9,
		Source:     "framework",
		},
		{
		Concept:    "CARGO_BUILD_SCRIPT",
		Phrases:    []string{"build script", "build.rs", "build output", "build directive", "custom metadata"},
		Targets:    []string{"BuildOutput", "BuildOutput.parse", "BuildOutput.parse_file", "TargetInfo"},
		TargetType: "symbol",
		Weight:     0.9,
		Source:     "framework",
		},
		{
		Concept:    "CARGO_WORKSPACE",
		Phrases:    []string{"workspace", "workspace member", "workspace dependency", "cargo workspace"},
		Targets:    []string{"Workspace", "Workspace.members", "Package", "Package.dependencies", "VirtualManifest"},
		TargetType: "symbol",
		Weight:     0.9,
		Source:     "framework",
		},
		{
		Concept:    "CARGO_SOURCE",
		Phrases:    []string{"path dependency", "path source", "registry source", "crate source"},
		Targets:    []string{"PathSource", "PathSource.read_package", "RecursivePathSource", "RegistrySource", "SourceId"},
		TargetType: "symbol",
		Weight:     0.9,
		Source:     "framework",
		},
		{
		Concept:    "CARGO_ALIAS",
		Phrases:    []string{"command alias", "cargo alias", "expand alias"},
		Targets:    []string{"expand_aliases", "builtin_exec", "GlobalContext", "GlobalContext.get_list"},
		TargetType: "symbol",
		Weight:     0.9,
		Source:     "framework",
		},
		{
		Concept:    "CARGO_IMPORT",
		Phrases:    []string{"cargo import", "import command", "artifact dependency"},
		Targets:    []string{"ImportCommand", "ImportOpts", "Dependency", "Dependency.set_artifact", "Target", "compute_deps"},
		TargetType: "symbol",
		Weight:     0.9,
		Source:     "framework",
		},
		{
		Concept:    "CARGO_LINT",
		Phrases:    []string{"cargo lint", "cargo warning", "workspace lint"},
		Targets:    []string{"analyze_cargo_lints", "DiagnosticPrinter"},
		TargetType: "symbol",
		Weight:     0.9,
		Source:     "framework",
		},
	}
}

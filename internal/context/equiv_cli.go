package context

// cliEquivalenceClasses returns equivalence classes for CLI/command-line patterns.
func cliEquivalenceClasses() []EquivalenceClass {
	return []EquivalenceClass{
		{
			Concept:    "CLI_ARGS",
			Phrases:    []string{"cli argument", "command line argument", "flag parsing", "parse flags", "argument parser"},
			Targets:    []string{"ArgParser", "FlagSet", "Parser", "add_argument", "flag", "Arg", "Command", "cobra"},
			TargetType: "symbol",
			Weight:     0.9,
			Source:     "framework",
		},
		{
			Concept:    "CLI_SUBCOMMAND",
			Phrases:    []string{"subcommand", "cli subcommand", "command group", "register command"},
			Targets:    []string{"Command", "Subcommand", "AddCommand", "register", "CommandFactory", "App"},
			TargetType: "symbol",
			Weight:     0.9,
			Source:     "framework",
		},
		{
			Concept:    "CLI_OUTPUT",
			Phrases:    []string{"cli output", "terminal output", "console output", "print formatted", "colored output"},
			Targets:    []string{"Printer", "Writer", "Formatter", "Printf", "Println", "Stderr", "Stdout", "ColorWriter"},
			TargetType: "symbol",
			Weight:     0.9,
			Source:     "framework",
		},
	}
}

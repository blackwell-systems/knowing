package context

// configEquivalenceClasses returns equivalence classes for configuration patterns.
func configEquivalenceClasses() []EquivalenceClass {
	return []EquivalenceClass{
		{
			Concept:    "CONFIG_ENV",
			Phrases:    []string{"environment variable", "env var", "dotenv", "env config", "getenv"},
			Targets:    []string{"Getenv", "LookupEnv", "dotenv", "load_dotenv", "environ", "EnvConfig", "FromEnv"},
			TargetType: "symbol",
			Weight:     0.9,
			Source:     "framework",
		},
		{
			Concept:    "CONFIG_FILE",
			Phrases:    []string{"config file", "yaml config", "json config", "toml config", "load config"},
			Targets:    []string{"Config", "LoadConfig", "ReadConfig", "ParseConfig", "ConfigFile", "Viper", "from_yaml"},
			TargetType: "symbol",
			Weight:     0.9,
			Source:     "framework",
		},
		{
			Concept:    "CONFIG_FEATURE_FLAG",
			Phrases:    []string{"feature flag", "feature toggle", "feature gate", "feature switch"},
			Targets:    []string{"FeatureFlag", "FeatureGate", "Toggle", "IsEnabled", "Enabled", "Variation"},
			TargetType: "symbol",
			Weight:     0.9,
			Source:     "framework",
		},
	}
}

package context

import "testing"

func TestIsStrongEquivMatch(t *testing.T) {
	tests := []struct {
		name   string
		match  equivalenceMatch
		strong bool
	}{
		{
			name: "single word phrase - not strong",
			match: equivalenceMatch{
				phrase:      "command",
				phraseCount: 1,
			},
			strong: false,
		},
		{
			name: "multi-word phrase - strong",
			match: equivalenceMatch{
				phrase:      "command palette",
				phraseCount: 1,
			},
			strong: true,
		},
		{
			name: "two phrases matched - strong",
			match: equivalenceMatch{
				phrase:      "command",
				phraseCount: 2,
			},
			strong: true,
		},
		{
			name: "three phrases matched - strong",
			match: equivalenceMatch{
				phrase:      "keybinding",
				phraseCount: 3,
			},
			strong: true,
		},
		{
			name: "single word action - not strong",
			match: equivalenceMatch{
				phrase:      "action",
				phraseCount: 1,
			},
			strong: false,
		},
		{
			name: "multi-word keyboard shortcut - strong",
			match: equivalenceMatch{
				phrase:      "keyboard shortcut",
				phraseCount: 1,
			},
			strong: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isStrongEquivMatch(tt.match)
			if got != tt.strong {
				t.Errorf("isStrongEquivMatch(%q, count=%d) = %v, want %v",
					tt.match.phrase, tt.match.phraseCount, got, tt.strong)
			}
		})
	}
}

func TestMatchEquivalenceClasses_PhraseCount(t *testing.T) {
	classes := []EquivalenceClass{
		{
			Concept: "TEST_COMMAND",
			Phrases: []string{"command", "keybinding", "command palette"},
			Targets: []string{"registerCommand"},
			Weight:  0.9,
			Source:  "framework",
		},
	}

	// Query with only "command" - should match 1 phrase.
	matches := matchEquivalenceClasses("implement the sort command", classes)
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	if matches[0].phraseCount != 1 {
		t.Errorf("expected phraseCount=1 for single word match, got %d", matches[0].phraseCount)
	}
	if isStrongEquivMatch(matches[0]) {
		t.Error("single word 'command' should not be a strong match")
	}

	// Query with "command palette" - should match 2 phrases (both "command" and "command palette").
	matches = matchEquivalenceClasses("open the command palette", classes)
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	if matches[0].phraseCount != 2 {
		t.Errorf("expected phraseCount=2 for 'command palette' (matches 'command' + 'command palette'), got %d", matches[0].phraseCount)
	}
	if !isStrongEquivMatch(matches[0]) {
		t.Error("'command palette' (2 phrases) should be a strong match")
	}

	// Query with "keybinding" only - 1 single-word phrase, not strong.
	matches = matchEquivalenceClasses("configure keybinding", classes)
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	if matches[0].phraseCount != 1 {
		t.Errorf("expected phraseCount=1, got %d", matches[0].phraseCount)
	}
	if isStrongEquivMatch(matches[0]) {
		t.Error("single word 'keybinding' should not be a strong match")
	}

	// Query with both "command" and "keybinding" - 2 phrases, strong.
	matches = matchEquivalenceClasses("command keybinding resolution", classes)
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	if matches[0].phraseCount != 2 {
		t.Errorf("expected phraseCount=2, got %d", matches[0].phraseCount)
	}
	if !isStrongEquivMatch(matches[0]) {
		t.Error("'command' + 'keybinding' (2 phrases) should be a strong match")
	}
}

func TestIsTestFilePath(t *testing.T) {
	tests := []struct {
		qn   string
		want bool
	}{
		// Go
		{"repo://internal/context/context_test.go.TestFoo", true},
		{"repo://internal/context/context.go.ForTask", false},

		// Python
		{"repo://django/tests/test_models.py.TestModel", true},
		{"repo://tests/test_admin.py.TestAdmin", true},
		{"repo://django/contrib/auth/models.py.User", false},

		// TypeScript/JavaScript
		{"repo://src/components/Button.test.tsx.ButtonTest", true},
		{"repo://src/components/Button.spec.ts.describe", true},
		{"repo://src/__tests__/utils.ts.TestUtil", true},
		{"repo://src/components/Button.tsx.Button", false},

		// Rust
		{"repo://src/tests/integration.rs.test_foo", true},

		// Ruby (Rails convention: test/ directory with .rb files)
		{"repo://activerecord/test/models/developer.ThreadsafeDeveloper", true},
		{"repo://activerecord/test/cases/scoping/named_scoping_test.rb.ScopeTest", true},
		{"repo://actioncable/test/channel/base_test.rb.BaseTest", true},
		{"repo://activerecord/lib/active_record/scoping/named.rb.scope", false},
		{"repo://activerecord/lib/active_record/test_case.rb.TestCase", false},    // lib file named test_case, not in test/
		{"repo://activerecord/lib/active_record/test_fixtures.rb.TestFixtures", false}, // lib file with "test" in name

		// Java (Maven/Gradle: src/test/java/)
		{"repo://streams/src/test/java/org/apache/kafka/streams/KafkaStreamsTest.java.KafkaStreamsTest", true},
		{"repo://clients/src/main/java/org/apache/kafka/clients/consumer/Consumer.java.Consumer", false},

		// C# (*.Tests/ project directories)
		{"repo://test/Ocelot.UnitTests/Configuration/ConfigurationTest.cs.ConfigTest", true},
		{"repo://src/Ocelot.AcceptanceTests/Steps.cs.Steps", true},
		{"repo://src/Ocelot/Configuration/Creator.cs.Creator", false},
	}

	for _, tt := range tests {
		t.Run(tt.qn, func(t *testing.T) {
			got := isTestFilePath(tt.qn)
			if got != tt.want {
				t.Errorf("isTestFilePath(%q) = %v, want %v", tt.qn, got, tt.want)
			}
		})
	}
}

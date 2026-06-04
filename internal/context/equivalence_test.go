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

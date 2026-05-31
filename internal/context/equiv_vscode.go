package context

// vscodeEquivalenceClasses returns equivalence classes for vscode.
func vscodeEquivalenceClasses() []EquivalenceClass {
	return []EquivalenceClass{
		{
		Concept:    "VSCODE_EDITOR",
		Phrases:    []string{"editor model", "text model", "piece tree", "text buffer", "document model"},
		Targets:    []string{"TextModel", "PieceTreeTextBuffer", "PieceTreeBase", "TextBuffer", "ITextModel"},
		TargetType: "symbol",
		Weight:     0.9,
		Source:     "framework",
		Lang:       "typescript",
		},
		{
		Concept:    "VSCODE_DECORATION",
		Phrases:    []string{"decoration", "editor decoration", "inline decoration", "decoration type"},
		Targets:    []string{"Decoration", "DecorationsOverviewRuler", "InlineDecoration", "ModelDecorationOptions", "TrackedRangeStickiness"},
		TargetType: "symbol",
		Weight:     0.9,
		Source:     "framework",
		Lang:       "typescript",
		},
		{
		Concept:    "VSCODE_DIFF",
		Phrases:    []string{"diff algorithm", "diff editor", "line diff", "move detection", "diff computation"},
		Targets:    []string{"DiffAlgorithm", "DiffComputer", "LineSequence", "DetailedLineRangeMapping", "MovedText", "AdvancedLinesDiffComputer"},
		TargetType: "symbol",
		Weight:     0.9,
		Source:     "framework",
		Lang:       "typescript",
		},
		{
		Concept:    "VSCODE_DISPOSABLE",
		Phrases:    []string{"disposable", "dispose", "lifecycle", "cleanup", "leak detection"},
		Targets:    []string{"Disposable", "DisposableStore", "MutableDisposable", "toDisposable", "IDisposable", "dispose"},
		TargetType: "symbol",
		Weight:     0.9,
		Source:     "framework",
		Lang:       "typescript",
		},
		{
		Concept:    "VSCODE_EVENT",
		Phrases:    []string{"event emitter", "event listener", "on event", "fire event"},
		Targets:    []string{"Emitter", "Event", "EventBufferer", "EventMultiplexer", "onDidChange", "onWillChange"},
		TargetType: "symbol",
		Weight:     0.9,
		Source:     "framework",
		Lang:       "typescript",
		},
		{
		Concept:    "VSCODE_COMMAND",
		Phrases:    []string{"command", "keybinding", "keyboard shortcut", "command palette", "action"},
		Targets:    []string{"CommandsRegistry", "registerCommand", "KeybindingResolver", "ResolvedKeybindingItem", "Action"},
		TargetType: "symbol",
		Weight:     0.9,
		Source:     "framework",
		Lang:       "typescript",
		},
	}
}

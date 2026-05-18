package context

import (
	stdctx "context"
	"strings"

	"github.com/blackwell-systems/knowing/internal/types"
)

// graphDerivedAliases generates equivalence classes from the graph structure.
// For each candidate seed node, it looks at callers and callees to extract
// meaningful terms that could serve as aliases for the node.
//
// Example: TransitiveCallers is called by handleBlastRadius. Splitting
// "handleBlastRadius" yields ["handle", "Blast", "Radius"]. Filtering out
// generic words ("handle") leaves ["blast", "radius"]. These become phrases
// that map back to TransitiveCallers.
//
// This is targeted (specific phrases -> specific targets) not untargeted
// (dumping text into BM25), because we only create aliases for nodes that
// are already in the candidate pool and only from their direct neighbors.
func graphDerivedAliases(ctx stdctx.Context, store types.GraphStore, seedHashes []types.Hash) []EquivalenceClass {
	// Generic words that appear in many function names but carry no semantic value.
	generic := map[string]bool{
		"handle": true, "new": true, "get": true, "set": true, "is": true,
		"has": true, "do": true, "run": true, "make": true, "init": true,
		"test": true, "mock": true, "err": true, "ctx": true, "ok": true,
		"the": true, "for": true, "with": true, "from": true, "and": true,
		"server": true, "store": true, "func": true, "type": true,
		"string": true, "int": true, "bool": true, "error": true,
		"require": true, "optional": true, "args": true, "result": true,
	}

	var classes []EquivalenceClass

	for _, seedHash := range seedHashes {
		node, err := store.GetNode(ctx, seedHash)
		if err != nil || node == nil {
			continue
		}

		// Get the target symbol name (last component of qualified name).
		targetName := node.QualifiedName
		if dot := strings.LastIndex(targetName, "."); dot >= 0 {
			targetName = targetName[dot+1:]
		}

		// Collect neighbor symbol names from callers and callees.
		var neighborWords []string

		// Callers (who calls this symbol).
		callerEdges, err := store.EdgesTo(ctx, seedHash, "calls")
		if err == nil {
			for _, e := range callerEdges {
				caller, err := store.GetNode(ctx, e.SourceHash)
				if err != nil || caller == nil {
					continue
				}
				words := extractMeaningfulWords(caller.QualifiedName, generic)
				neighborWords = append(neighborWords, words...)
			}
		}

		// Callees (what does this symbol call) - less useful but adds context.
		calleeEdges, err := store.EdgesFrom(ctx, seedHash, "calls")
		if err == nil {
			for _, e := range calleeEdges {
				callee, err := store.GetNode(ctx, e.TargetHash)
				if err != nil || callee == nil {
					continue
				}
				words := extractMeaningfulWords(callee.QualifiedName, generic)
				neighborWords = append(neighborWords, words...)
			}
		}

		if len(neighborWords) == 0 {
			continue
		}

		// Deduplicate and filter words that are already in the target name.
		targetLower := strings.ToLower(targetName)
		seen := make(map[string]bool)
		var phrases []string
		for _, w := range neighborWords {
			wLower := strings.ToLower(w)
			if seen[wLower] || len(wLower) < 3 || strings.Contains(targetLower, wLower) {
				continue
			}
			seen[wLower] = true
			phrases = append(phrases, wLower)
		}

		// Build multi-word phrases from consecutive pairs (bigrams).
		// "blast" + "radius" from handleBlastRadius -> "blast radius" as a phrase.
		var bigramPhrases []string
		for i := 0; i < len(phrases)-1; i++ {
			bigram := phrases[i] + " " + phrases[i+1]
			bigramPhrases = append(bigramPhrases, bigram)
		}
		phrases = append(phrases, bigramPhrases...)

		if len(phrases) == 0 {
			continue
		}

		classes = append(classes, EquivalenceClass{
			Concept:    "GRAPH_" + targetName,
			Phrases:    phrases,
			Targets:    []string{targetName},
			TargetType: "symbol",
			Weight:     0.7, // lower than seed (1.0) since graph-derived is noisier
			Source:     "graph",
		})
	}

	return classes
}

// extractMeaningfulWords splits a qualified name into its component words,
// filters out generic terms, and returns meaningful words.
func extractMeaningfulWords(qn string, generic map[string]bool) []string {
	// Get the last component (symbol name, possibly Type.Method).
	if dot := strings.LastIndex(qn, "/"); dot >= 0 {
		qn = qn[dot+1:]
	}
	// Remove package prefix (e.g., "mcp.Server.handleBlastRadius" -> "Server.handleBlastRadius").
	if dot := strings.Index(qn, "."); dot >= 0 {
		qn = qn[dot+1:]
	}

	// Split CamelCase into words.
	var words []string
	current := strings.Builder{}
	for i, r := range qn {
		if r == '.' || r == '_' {
			if current.Len() > 0 {
				words = append(words, current.String())
				current.Reset()
			}
			continue
		}
		if i > 0 && r >= 'A' && r <= 'Z' {
			prev := rune(qn[i-1])
			if prev >= 'a' && prev <= 'z' {
				if current.Len() > 0 {
					words = append(words, current.String())
					current.Reset()
				}
			}
		}
		current.WriteRune(r)
	}
	if current.Len() > 0 {
		words = append(words, current.String())
	}

	// Filter generic words.
	var meaningful []string
	for _, w := range words {
		if !generic[strings.ToLower(w)] && len(w) >= 3 {
			meaningful = append(meaningful, w)
		}
	}
	return meaningful
}

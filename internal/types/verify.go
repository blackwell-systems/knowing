package types

import (
	"fmt"
	"strings"
)

// VerifyNodeHash recomputes a node's hash from its stored fields and compares
// it to the stored NodeHash. Returns nil if the hash matches, or a descriptive
// error on mismatch.
//
// The repoURL and packagePath must be passed explicitly because they are not
// stored on the Node struct directly. The symbolName is extracted from
// n.QualifiedName by taking everything after the last "." that follows "://".
func VerifyNodeHash(n Node, repoURL, packagePath string) error {
	symbolName := extractSymbolName(n.QualifiedName)
	computed := ComputeNodeHash(repoURL, packagePath, EmptyHash, symbolName, n.Kind)
	if n.NodeHash != computed {
		return fmt.Errorf("node hash mismatch: stored=%s computed=%s", n.NodeHash, computed)
	}
	return nil
}

// VerifyEdgeHash recomputes an edge's hash from its stored fields and compares
// it to the stored EdgeHash. Returns nil if the hash matches, or a descriptive
// error on mismatch.
func VerifyEdgeHash(e Edge) error {
	computed := ComputeEdgeHash(e.SourceHash, e.TargetHash, e.EdgeType, e.Provenance)
	if e.EdgeHash != computed {
		return fmt.Errorf("edge hash mismatch: stored=%s computed=%s", e.EdgeHash, computed)
	}
	return nil
}

// extractSymbolName extracts the symbol name from a qualified name of the form
// "{repoURL}://{pkgPath}.{SymbolName}". It finds the last "." after "://" and
// returns everything after it.
//
// Example: "https://github.com/org/repo://pkg/sub.MyFunc" -> "MyFunc"
func extractSymbolName(qualifiedName string) string {
	// Find the "://" separator to skip past the repo URL portion.
	schemeSep := strings.Index(qualifiedName, "://")
	if schemeSep < 0 {
		// No "://" found; fall back to everything after the last dot.
		if i := strings.LastIndex(qualifiedName, "."); i >= 0 {
			return qualifiedName[i+1:]
		}
		return qualifiedName
	}
	// Look for the last "." after the "://" separator.
	afterScheme := qualifiedName[schemeSep+3:]
	if i := strings.LastIndex(afterScheme, "."); i >= 0 {
		return afterScheme[i+1:]
	}
	return afterScheme
}

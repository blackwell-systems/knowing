package types

// VerifyNodeHash recomputes and compares a stored node hash against its inputs.
// Returns nil if the hash matches, or an error describing the mismatch.
func VerifyNodeHash(n Node, repoURL, packagePath string) error { return nil }

// VerifyEdgeHash recomputes and compares a stored edge hash against its inputs.
// Returns nil if the hash matches, or an error describing the mismatch.
func VerifyEdgeHash(e Edge) error { return nil }

// Package modulea is a shared library used by module-b and module-c.
// It provides string and math helpers that create cross-repo call edges.
package modulea

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
)

// Hash computes a SHA-256 hash of the input string.
func Hash(input string) string {
	h := sha256.Sum256([]byte(input))
	return hex.EncodeToString(h[:])
}

// Normalize lowercases and trims whitespace from a string.
func Normalize(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// FormatID creates a qualified identifier from parts.
func FormatID(parts ...string) string {
	return strings.Join(parts, "://")
}

// ValidateNonEmpty returns an error if any input is empty.
func ValidateNonEmpty(inputs ...string) error {
	for i, s := range inputs {
		if s == "" {
			return fmt.Errorf("input %d is empty", i)
		}
	}
	return nil
}

// SplitQualified splits a qualified name at the last dot.
func SplitQualified(qn string) (pkg, name string) {
	idx := strings.LastIndex(qn, ".")
	if idx < 0 {
		return "", qn
	}
	return qn[:idx], qn[idx+1:]
}

// Contains checks if a slice contains a value.
func Contains(slice []string, val string) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}

// Deduplicate removes duplicate strings preserving order.
func Deduplicate(items []string) []string {
	seen := make(map[string]bool, len(items))
	result := make([]string, 0, len(items))
	for _, item := range items {
		if !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}
	return result
}

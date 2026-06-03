// Package wire provides graph payload encoding for knowing's MCP server.
//
// Core GCF types and encoding are provided by github.com/blackwell-systems/gcf-go.
// This package re-exports them as type aliases for backward compatibility and adds
// knowing-specific codecs (binary, JSON, TOON) via the pluggable registry.
package wire

import (
	gcf "github.com/blackwell-systems/gcf-go"
)

// Core GCF types, re-exported from github.com/blackwell-systems/gcf-go.
type Symbol = gcf.Symbol
type Components = gcf.Components
type Edge = gcf.Edge
type Payload = gcf.Payload
type Session = gcf.Session
type DeltaPayload = gcf.DeltaPayload

// KindAbbrev and KindExpand are the kind abbreviation maps from gcf-go.
var KindAbbrev = gcf.KindAbbrev
var KindExpand = gcf.KindExpand

// Encode serializes a Payload into GCF text format.
// Delegates to gcf-go.
func Encode(p *Payload) string {
	return gcf.Encode(p)
}

// Decode parses GCF text back into a Payload.
// Delegates to gcf-go.
func Decode(input string) (*Payload, error) {
	return gcf.Decode(input)
}

// NewSession creates a new session tracker for cross-call symbol deduplication.
// Delegates to gcf-go.
func NewSession() *Session {
	return gcf.NewSession()
}

// EncodeWithSession encodes a payload with session deduplication.
// Previously-transmitted symbols are emitted as bare references.
// Delegates to gcf-go.
func EncodeWithSession(p *Payload, sess *Session) string {
	return gcf.EncodeWithSession(p, sess)
}

// EncodeDelta serializes a DeltaPayload into GCF delta format.
// Delegates to gcf-go.
func EncodeDelta(d *DeltaPayload) string {
	return gcf.EncodeDelta(d)
}

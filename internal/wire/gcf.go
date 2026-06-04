// Package wire provides graph payload encoding for knowing's MCP server.
//
// Core GCF types and encoding are provided by github.com/blackwell-systems/gcf-go.
// This package adds knowing-specific codecs (binary, JSON) via the pluggable
// registry and provides FromContextBlock for converting internal types to GCF payloads.
package wire

import (
	gcf "github.com/blackwell-systems/gcf-go"
)

// Re-export core GCF types so existing internal/wire consumers compile without
// changing their imports. New code should import gcf-go directly.
type Symbol = gcf.Symbol
type Components = gcf.Components
type Edge = gcf.Edge
type Payload = gcf.Payload
type Session = gcf.Session
type DeltaPayload = gcf.DeltaPayload

// Re-export kind maps.
var KindAbbrev = gcf.KindAbbrev
var KindExpand = gcf.KindExpand

// Delegating functions for the codec registry adapters and consumers that
// import wire but need GCF encoding. New code should use gcf.Encode directly.

func Encode(p *Payload) string                         { return gcf.Encode(p) }
func Decode(input string) (*Payload, error)            { return gcf.Decode(input) }
func NewSession() *Session                             { return gcf.NewSession() }
func EncodeWithSession(p *Payload, s *Session) string  { return gcf.EncodeWithSession(p, s) }
func EncodeDelta(d *DeltaPayload) string               { return gcf.EncodeDelta(d) }

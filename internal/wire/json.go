package wire

import (
	"encoding/json"
	"fmt"
)

// JSON codec: standard JSON serialization for maximum compatibility.

type jsonPayload struct {
	Tool        string       `json:"tool"`
	TokensUsed  int          `json:"tokens_used"`
	TokenBudget int          `json:"token_budget"`
	PackRoot    string       `json:"pack_root,omitempty"`
	Symbols     []jsonSymbol `json:"symbols"`
	Edges       []jsonEdge   `json:"edges,omitempty"`
}

type jsonSymbol struct {
	QualifiedName string         `json:"qualified_name"`
	Kind          string         `json:"kind"`
	Score         float64        `json:"score"`
	Signature     string         `json:"signature,omitempty"`
	Provenance    string         `json:"provenance"`
	Distance      int            `json:"distance"`
	Components    jsonComponents `json:"components"`
}

type jsonComponents struct {
	BlastRadius float64 `json:"blast_radius"`
	Confidence  float64 `json:"confidence"`
	Recency     float64 `json:"recency"`
	Distance    float64 `json:"distance"`
}

type jsonEdge struct {
	Source   string `json:"source"`
	Target   string `json:"target"`
	EdgeType string `json:"edge_type"`
	Status   string `json:"status,omitempty"`
}

func encodeJSON(p *Payload) (string, error) {
	out := jsonPayload{
		Tool:        p.Tool,
		TokensUsed:  p.TokensUsed,
		TokenBudget: p.TokenBudget,
		PackRoot:    p.PackRoot,
	}

	for _, s := range p.Symbols {
		out.Symbols = append(out.Symbols, jsonSymbol{
			QualifiedName: s.QualifiedName,
			Kind:          s.Kind,
			Score:         s.Score,
			Signature:     s.Signature,
			Provenance:    s.Provenance,
			Distance:      s.Distance,
			Components: jsonComponents{
				BlastRadius: s.Components.BlastRadius,
				Confidence:  s.Components.Confidence,
				Recency:     s.Components.Recency,
				Distance:    s.Components.Distance,
			},
		})
	}

	for _, e := range p.Edges {
		out.Edges = append(out.Edges, jsonEdge{
			Source:   e.Source,
			Target:   e.Target,
			EdgeType: e.EdgeType,
			Status:   e.Status,
		})
	}

	data, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return "", fmt.Errorf("wire/json: marshal: %w", err)
	}
	return string(data), nil
}

func decodeJSON(input string) (*Payload, error) {
	var raw jsonPayload
	if err := json.Unmarshal([]byte(input), &raw); err != nil {
		return nil, fmt.Errorf("wire/json: unmarshal: %w", err)
	}

	p := &Payload{
		Tool:        raw.Tool,
		TokensUsed:  raw.TokensUsed,
		TokenBudget: raw.TokenBudget,
	}

	for _, s := range raw.Symbols {
		p.Symbols = append(p.Symbols, Symbol{
			QualifiedName: s.QualifiedName,
			Kind:          s.Kind,
			Score:         s.Score,
			Signature:     s.Signature,
			Provenance:    s.Provenance,
			Distance:      s.Distance,
			Components: Components{
				BlastRadius: s.Components.BlastRadius,
				Confidence:  s.Components.Confidence,
				Recency:     s.Components.Recency,
				Distance:    s.Components.Distance,
			},
		})
	}

	for _, e := range raw.Edges {
		p.Edges = append(p.Edges, Edge{
			Source:   e.Source,
			Target:   e.Target,
			EdgeType: e.EdgeType,
			Status:   e.Status,
		})
	}

	return p, nil
}

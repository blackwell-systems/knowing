package wire

import (
	"fmt"
	"strconv"
	"strings"
)

// Decode parses GCF text back into a Payload.
func Decode(input string) (*Payload, error) {
	lines := strings.Split(input, "\n")
	if len(lines) == 0 {
		return nil, fmt.Errorf("gcf: empty input")
	}

	p := &Payload{}

	// Parse header.
	header := lines[0]
	if !strings.HasPrefix(header, "GCF ") {
		return nil, fmt.Errorf("gcf: invalid header, expected 'GCF ...' got %q", header)
	}
	if err := parseHeader(header[4:], p); err != nil {
		return nil, err
	}

	// Parse body: symbols and edges.
	var symbols []Symbol
	symByID := make(map[int]*Symbol)
	currentDistance := 0
	inEdges := false

	for _, line := range lines[1:] {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}

		// Group header.
		if strings.HasPrefix(line, "## ") {
			group := line[3:]
			inEdges = group == "edges"
			if !inEdges {
				switch group {
				case "targets":
					currentDistance = 0
				case "related":
					currentDistance = 1
				case "extended":
					currentDistance = 2
				default:
					if strings.HasPrefix(group, "distance_") {
						d, err := strconv.Atoi(group[9:])
						if err == nil {
							currentDistance = d
						}
					}
				}
			}
			continue
		}

		// Comment.
		if strings.HasPrefix(line, "# ") {
			continue
		}

		if inEdges {
			edge, err := parseEdgeLine(line, symByID)
			if err != nil {
				return nil, err
			}
			p.Edges = append(p.Edges, edge)
		} else {
			sym, id, err := parseSymbolLine(line, currentDistance)
			if err != nil {
				return nil, err
			}
			symbols = append(symbols, sym)
			symByID[id] = &symbols[len(symbols)-1]
		}
	}

	p.Symbols = symbols
	return p, nil
}

func parseHeader(fields string, p *Payload) error {
	for _, part := range strings.Fields(fields) {
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		switch kv[0] {
		case "tool":
			p.Tool = kv[1]
		case "budget":
			v, err := strconv.Atoi(kv[1])
			if err != nil {
				return fmt.Errorf("gcf: invalid budget %q: %w", kv[1], err)
			}
			p.TokenBudget = v
		case "tokens":
			v, err := strconv.Atoi(kv[1])
			if err != nil {
				return fmt.Errorf("gcf: invalid tokens %q: %w", kv[1], err)
			}
			p.TokensUsed = v
		case "symbols":
			// informational, reconstructed from parsed symbols
		}
	}
	return nil
}

func parseSymbolLine(line string, distance int) (Symbol, int, error) {
	// Format: @ID kind qname score provenance
	if !strings.HasPrefix(line, "@") {
		return Symbol{}, 0, fmt.Errorf("gcf: expected symbol line starting with @, got %q", line)
	}

	parts := strings.Fields(line)
	if len(parts) < 5 {
		return Symbol{}, 0, fmt.Errorf("gcf: symbol line needs at least 5 fields, got %d in %q", len(parts), line)
	}

	idStr := parts[0][1:] // strip @
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return Symbol{}, 0, fmt.Errorf("gcf: invalid symbol id %q: %w", idStr, err)
	}

	kind := parts[1]
	if expanded, ok := kindExpand[kind]; ok {
		kind = expanded
	}

	qname := parts[2]

	score, err := strconv.ParseFloat(parts[3], 64)
	if err != nil {
		return Symbol{}, 0, fmt.Errorf("gcf: invalid score %q: %w", parts[3], err)
	}

	provenance := parts[4]

	return Symbol{
		QualifiedName: qname,
		Kind:          kind,
		Score:         score,
		Provenance:    provenance,
		Distance:      distance,
	}, id, nil
}

func parseEdgeLine(line string, symByID map[int]*Symbol) (Edge, error) {
	// Format: @target<@source edge_type [status]
	// Example: @0<@4 calls
	// Example: @0<@1 calls added

	parts := strings.Fields(line)
	if len(parts) < 2 {
		return Edge{}, fmt.Errorf("gcf: edge line needs at least 2 fields, got %q", line)
	}

	ref := parts[0]
	// Parse @target<@source
	ltIdx := strings.Index(ref, "<")
	if ltIdx < 0 {
		return Edge{}, fmt.Errorf("gcf: edge line missing '<' separator in %q", ref)
	}

	targetIDStr := ref[1:ltIdx] // strip leading @
	sourceIDStr := ref[ltIdx+2:]  // strip <@

	targetID, err := strconv.Atoi(targetIDStr)
	if err != nil {
		return Edge{}, fmt.Errorf("gcf: invalid target id %q: %w", targetIDStr, err)
	}
	sourceID, err := strconv.Atoi(sourceIDStr)
	if err != nil {
		return Edge{}, fmt.Errorf("gcf: invalid source id %q: %w", sourceIDStr, err)
	}

	targetSym := symByID[targetID]
	sourceSym := symByID[sourceID]
	if targetSym == nil || sourceSym == nil {
		return Edge{}, fmt.Errorf("gcf: edge references unknown symbol id(s): target=%d source=%d", targetID, sourceID)
	}

	edgeType := parts[1]
	status := ""
	if len(parts) >= 3 {
		status = parts[2]
	}

	return Edge{
		Source:   sourceSym.QualifiedName,
		Target:   targetSym.QualifiedName,
		EdgeType: edgeType,
		Status:   status,
	}, nil
}

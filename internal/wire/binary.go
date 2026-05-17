package wire

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math"
)

// Binary codec: compact binary encoding for transport and storage.
// Optimizes for minimal byte size on the wire between services.
// Not intended for direct LLM consumption (use GCF for that).
//
// Wire layout:
//   [magic:4][version:1][header][symbols][edges]
//
// Header:  tool(str) tokens_used(varint) token_budget(varint) num_symbols(varint) num_edges(varint)
// Symbol:  qname(str) kind(uint8) score(float32) provenance(uint8) distance(uint8) signature(str) components(4xfloat32)
// Edge:    source_idx(varint) target_idx(varint) edge_type(uint8) status(uint8)

const (
	binaryMagic   = "GCB1" // Graph Compact Binary v1
	binaryVersion = 1
)

// Kind encoding (1 byte).
var kindToID = map[string]uint8{
	"function":  1,
	"type":      2,
	"method":    3,
	"interface": 4,
	"var":       5,
	"const":     6,
	"resource":  7,
	"table":     8,
	"class":     9,
	"selector":  10,
}

var idToKind = map[uint8]string{
	1: "function", 2: "type", 3: "method", 4: "interface",
	5: "var", 6: "const", 7: "resource", 8: "table",
	9: "class", 10: "selector",
}

// Provenance encoding (1 byte).
var provToID = map[string]uint8{
	"ast_inferred": 1,
	"ast_resolved": 2,
	"lsp_resolved": 3,
	"otel_trace":   4,
}

var idToProv = map[uint8]string{
	1: "ast_inferred", 2: "ast_resolved", 3: "lsp_resolved", 4: "otel_trace",
}

// Edge type encoding (1 byte).
var edgeTypeToID = map[string]uint8{
	"calls":         1,
	"imports":       2,
	"implements":    3,
	"references":    4,
	"handles_route": 5,
	"depends_on":    6,
	"deploys":       7,
	"exposes":       8,
	"configures":    9,
}

var idToEdgeType = map[uint8]string{
	1: "calls", 2: "imports", 3: "implements", 4: "references",
	5: "handles_route", 6: "depends_on", 7: "deploys", 8: "exposes",
	9: "configures",
}

// Edge status encoding (1 byte).
var statusToID = map[string]uint8{
	"":          0,
	"unchanged": 0,
	"added":     1,
	"removed":   2,
}

var idToStatus = map[uint8]string{
	0: "", 1: "added", 2: "removed",
}

func encodeBinary(p *Payload) (string, error) {
	var buf bytes.Buffer

	// Magic + version.
	buf.WriteString(binaryMagic)
	buf.WriteByte(binaryVersion)

	// Header.
	writeStr(&buf, p.Tool)
	writeVarint(&buf, p.TokensUsed)
	writeVarint(&buf, p.TokenBudget)
	writeVarint(&buf, len(p.Symbols))
	writeVarint(&buf, len(p.Edges))

	// Build symbol index for edge encoding.
	symIndex := make(map[string]int, len(p.Symbols))

	// Symbols.
	for i, s := range p.Symbols {
		symIndex[s.QualifiedName] = i
		writeStr(&buf, s.QualifiedName)

		kindID := kindToID[s.Kind]
		buf.WriteByte(kindID)

		writeFloat32(&buf, float32(s.Score))

		provID := provToID[s.Provenance]
		buf.WriteByte(provID)

		buf.WriteByte(uint8(s.Distance))

		writeStr(&buf, s.Signature)

		writeFloat32(&buf, float32(s.Components.BlastRadius))
		writeFloat32(&buf, float32(s.Components.Confidence))
		writeFloat32(&buf, float32(s.Components.Recency))
		writeFloat32(&buf, float32(s.Components.Distance))
	}

	// Edges.
	for _, e := range p.Edges {
		srcIdx := symIndex[e.Source]
		tgtIdx := symIndex[e.Target]
		writeVarint(&buf, srcIdx)
		writeVarint(&buf, tgtIdx)

		etID := edgeTypeToID[e.EdgeType]
		buf.WriteByte(etID)

		stID := statusToID[e.Status]
		buf.WriteByte(stID)
	}

	return buf.String(), nil
}

func decodeBinary(input string) (*Payload, error) {
	r := bytes.NewReader([]byte(input))

	// Magic.
	magic := make([]byte, 4)
	if _, err := io.ReadFull(r, magic); err != nil {
		return nil, fmt.Errorf("wire/binary: read magic: %w", err)
	}
	if string(magic) != binaryMagic {
		return nil, fmt.Errorf("wire/binary: invalid magic %q", magic)
	}

	// Version.
	ver, err := r.ReadByte()
	if err != nil {
		return nil, fmt.Errorf("wire/binary: read version: %w", err)
	}
	if ver != binaryVersion {
		return nil, fmt.Errorf("wire/binary: unsupported version %d", ver)
	}

	p := &Payload{}

	// Header.
	p.Tool, err = readStr(r)
	if err != nil {
		return nil, fmt.Errorf("wire/binary: read tool: %w", err)
	}
	p.TokensUsed, err = readVarint(r)
	if err != nil {
		return nil, fmt.Errorf("wire/binary: read tokens_used: %w", err)
	}
	p.TokenBudget, err = readVarint(r)
	if err != nil {
		return nil, fmt.Errorf("wire/binary: read token_budget: %w", err)
	}
	numSymbols, err := readVarint(r)
	if err != nil {
		return nil, fmt.Errorf("wire/binary: read num_symbols: %w", err)
	}
	numEdges, err := readVarint(r)
	if err != nil {
		return nil, fmt.Errorf("wire/binary: read num_edges: %w", err)
	}

	// Symbols.
	p.Symbols = make([]Symbol, numSymbols)
	for i := range p.Symbols {
		s := &p.Symbols[i]
		s.QualifiedName, err = readStr(r)
		if err != nil {
			return nil, fmt.Errorf("wire/binary: symbol[%d] qname: %w", i, err)
		}

		kindID, err := r.ReadByte()
		if err != nil {
			return nil, fmt.Errorf("wire/binary: symbol[%d] kind: %w", i, err)
		}
		s.Kind = idToKind[kindID]
		if s.Kind == "" {
			s.Kind = fmt.Sprintf("unknown_%d", kindID)
		}

		score, err := readFloat32(r)
		if err != nil {
			return nil, fmt.Errorf("wire/binary: symbol[%d] score: %w", i, err)
		}
		s.Score = float64(score)

		provID, err := r.ReadByte()
		if err != nil {
			return nil, fmt.Errorf("wire/binary: symbol[%d] provenance: %w", i, err)
		}
		s.Provenance = idToProv[provID]
		if s.Provenance == "" {
			s.Provenance = fmt.Sprintf("unknown_%d", provID)
		}

		dist, err := r.ReadByte()
		if err != nil {
			return nil, fmt.Errorf("wire/binary: symbol[%d] distance: %w", i, err)
		}
		s.Distance = int(dist)

		s.Signature, err = readStr(r)
		if err != nil {
			return nil, fmt.Errorf("wire/binary: symbol[%d] signature: %w", i, err)
		}

		br, err := readFloat32(r)
		if err != nil {
			return nil, fmt.Errorf("wire/binary: symbol[%d] blast_radius: %w", i, err)
		}
		conf, err := readFloat32(r)
		if err != nil {
			return nil, fmt.Errorf("wire/binary: symbol[%d] confidence: %w", i, err)
		}
		rec, err := readFloat32(r)
		if err != nil {
			return nil, fmt.Errorf("wire/binary: symbol[%d] recency: %w", i, err)
		}
		distComp, err := readFloat32(r)
		if err != nil {
			return nil, fmt.Errorf("wire/binary: symbol[%d] distance_comp: %w", i, err)
		}
		s.Components = Components{
			BlastRadius: float64(br),
			Confidence:  float64(conf),
			Recency:     float64(rec),
			Distance:    float64(distComp),
		}
	}

	// Edges.
	p.Edges = make([]Edge, numEdges)
	for i := range p.Edges {
		e := &p.Edges[i]

		srcIdx, err := readVarint(r)
		if err != nil {
			return nil, fmt.Errorf("wire/binary: edge[%d] source: %w", i, err)
		}
		tgtIdx, err := readVarint(r)
		if err != nil {
			return nil, fmt.Errorf("wire/binary: edge[%d] target: %w", i, err)
		}

		if srcIdx >= numSymbols || tgtIdx >= numSymbols {
			return nil, fmt.Errorf("wire/binary: edge[%d] index out of range (src=%d tgt=%d, max=%d)", i, srcIdx, tgtIdx, numSymbols-1)
		}

		e.Source = p.Symbols[srcIdx].QualifiedName
		e.Target = p.Symbols[tgtIdx].QualifiedName

		etID, err := r.ReadByte()
		if err != nil {
			return nil, fmt.Errorf("wire/binary: edge[%d] type: %w", i, err)
		}
		e.EdgeType = idToEdgeType[etID]
		if e.EdgeType == "" {
			e.EdgeType = fmt.Sprintf("unknown_%d", etID)
		}

		stID, err := r.ReadByte()
		if err != nil {
			return nil, fmt.Errorf("wire/binary: edge[%d] status: %w", i, err)
		}
		e.Status = idToStatus[stID]
	}

	return p, nil
}

// Encoding helpers.

func writeStr(buf *bytes.Buffer, s string) {
	writeVarint(buf, len(s))
	buf.WriteString(s)
}

func writeVarint(buf *bytes.Buffer, v int) {
	var tmp [binary.MaxVarintLen64]byte
	n := binary.PutUvarint(tmp[:], uint64(v))
	buf.Write(tmp[:n])
}

func writeFloat32(buf *bytes.Buffer, f float32) {
	var tmp [4]byte
	binary.LittleEndian.PutUint32(tmp[:], math.Float32bits(f))
	buf.Write(tmp[:])
}

// Decoding helpers.

func readStr(r *bytes.Reader) (string, error) {
	length, err := readVarint(r)
	if err != nil {
		return "", err
	}
	if length == 0 {
		return "", nil
	}
	buf := make([]byte, length)
	if _, err := io.ReadFull(r, buf); err != nil {
		return "", err
	}
	return string(buf), nil
}

func readVarint(r *bytes.Reader) (int, error) {
	v, err := binary.ReadUvarint(r)
	if err != nil {
		return 0, err
	}
	return int(v), nil
}

func readFloat32(r *bytes.Reader) (float32, error) {
	var tmp [4]byte
	if _, err := io.ReadFull(r, tmp[:]); err != nil {
		return 0, err
	}
	return math.Float32frombits(binary.LittleEndian.Uint32(tmp[:])), nil
}

func init() {
	Register(&Codec{
		Name:        "gcb",
		Description: "Graph Compact Binary: compact transport/storage encoding, 74%+ byte savings",
		Encode:      encodeBinary,
		Decode:      decodeBinary,
	})
}

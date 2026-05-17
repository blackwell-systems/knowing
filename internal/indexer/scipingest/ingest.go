package scipingest

import (
	"context"
	"fmt"
	"os"
	"strings"

	scip "github.com/scip-code/scip/bindings/go/scip"
	"google.golang.org/protobuf/proto"

	"github.com/blackwell-systems/knowing/internal/types"
)

// SCIPIngestOptions configures a SCIP index ingestion run.
type SCIPIngestOptions struct {
	FilePath string // path to the .scip index file on disk
	RepoURL  string // repo URL to associate with the ingested symbols
}

// SCIPIngestResult reports what was imported.
type SCIPIngestResult struct {
	NodesCreated  int
	EdgesCreated  int
	DocsProcessed int
}

// SCIPIngester reads SCIP index files and imports their symbols and
// edges into the knowing knowledge graph.
type SCIPIngester struct {
	store types.GraphStore
}

// NewSCIPIngester creates an ingester backed by the given store.
func NewSCIPIngester(store types.GraphStore) *SCIPIngester {
	return &SCIPIngester{store: store}
}

// IngestFile reads a .scip index file and imports all symbols and
// reference edges into the graph. Returns the number of nodes and
// edges created.
func (s *SCIPIngester) IngestFile(ctx context.Context, opts SCIPIngestOptions) (*SCIPIngestResult, error) {
	data, err := os.ReadFile(opts.FilePath)
	if err != nil {
		return nil, fmt.Errorf("reading SCIP index file: %w", err)
	}

	var index scip.Index
	if err := proto.Unmarshal(data, &index); err != nil {
		return nil, fmt.Errorf("unmarshaling SCIP index: %w", err)
	}

	result := &SCIPIngestResult{}

	for _, doc := range index.Documents {
		if err := s.processDocument(ctx, opts, doc, result); err != nil {
			return nil, fmt.Errorf("processing document %q: %w", doc.RelativePath, err)
		}
		result.DocsProcessed++
	}

	return result, nil
}

// processDocument handles a single SCIP Document, creating nodes for definitions
// and edges for references.
func (s *SCIPIngester) processDocument(ctx context.Context, opts SCIPIngestOptions, doc *scip.Document, result *SCIPIngestResult) error {
	// Compute file hash for this document
	repoHash := types.NewHash([]byte(opts.RepoURL))
	fileHash := computeFileHash(repoHash, doc.RelativePath)

	// Store the file record
	if err := s.store.PutFile(ctx, types.File{
		FileHash: fileHash,
		RepoHash: repoHash,
		Path:     doc.RelativePath,
	}); err != nil {
		return fmt.Errorf("storing file: %w", err)
	}

	// Build a map of symbol string -> node hash for reference resolution
	symbolToHash := make(map[string]types.Hash)

	// Process symbol definitions
	for _, sym := range doc.Symbols {
		if strings.HasPrefix(sym.Symbol, "local ") {
			// Skip local symbols for node creation (they are not useful as graph nodes)
			continue
		}

		repo, pkgPath, symbolName, symbolKind, err := ParseSCIPSymbol(sym.Symbol)
		if err != nil {
			continue // Skip unparseable symbols
		}

		nodeHash := types.ComputeNodeHash(repo, pkgPath, types.EmptyHash, symbolName, symbolKind)
		symbolToHash[sym.Symbol] = nodeHash

		qualifiedName := formatQualifiedName(repo, pkgPath, symbolName)

		var signature string
		if len(sym.Documentation) > 0 {
			signature = sym.Documentation[0]
		}

		node := types.Node{
			NodeHash:      nodeHash,
			FileHash:      fileHash,
			QualifiedName: qualifiedName,
			Kind:          symbolKind,
			Line:          0, // SCIP does not provide definition line in SymbolInformation
			Signature:     signature,
		}

		if err := s.store.PutNode(ctx, node); err != nil {
			return fmt.Errorf("storing node %q: %w", qualifiedName, err)
		}
		result.NodesCreated++
	}

	// Find definition occurrences to build a position-to-symbol map for enclosing scope
	defSymbols := make(map[string]string) // symbol string for definitions in this doc
	for _, occ := range doc.Occurrences {
		if occ.SymbolRoles&int32(scip.SymbolRole_Definition) != 0 {
			defSymbols[occ.Symbol] = occ.Symbol
			// Also ensure this symbol is in our hash map
			if _, ok := symbolToHash[occ.Symbol]; !ok && !strings.HasPrefix(occ.Symbol, "local ") {
				repo, pkgPath, symbolName, symbolKind, err := ParseSCIPSymbol(occ.Symbol)
				if err == nil {
					symbolToHash[occ.Symbol] = types.ComputeNodeHash(repo, pkgPath, types.EmptyHash, symbolName, symbolKind)
				}
			}
		}
	}

	// Process reference occurrences to create edges
	for _, occ := range doc.Occurrences {
		// Skip definitions (we only want references)
		if occ.SymbolRoles&int32(scip.SymbolRole_Definition) != 0 {
			continue
		}
		// Skip local symbols
		if strings.HasPrefix(occ.Symbol, "local ") {
			continue
		}

		// Get or compute the target node hash
		targetHash, ok := symbolToHash[occ.Symbol]
		if !ok {
			// External symbol: parse and create a node for it
			repo, pkgPath, symbolName, symbolKind, err := ParseSCIPSymbol(occ.Symbol)
			if err != nil {
				continue
			}
			targetHash = types.ComputeNodeHash(repo, pkgPath, types.EmptyHash, symbolName, symbolKind)
			symbolToHash[occ.Symbol] = targetHash

			// Create external node if it's from a different repo
			if repo != opts.RepoURL {
				extNode := types.Node{
					NodeHash:      targetHash,
					QualifiedName: formatQualifiedName(repo, pkgPath, symbolName),
					Kind:          symbolKind,
				}
				if err := s.store.PutNode(ctx, extNode); err != nil {
					return fmt.Errorf("storing external node: %w", err)
				}
				result.NodesCreated++
			}
		}

		// Find enclosing definition: use the first defined symbol in this document as source.
		// In practice, a more sophisticated approach would use position-based scoping,
		// but for now we use the first definition symbol we find.
		sourceHash := findEnclosingDefinition(doc, occ, symbolToHash)
		if sourceHash.IsZero() {
			continue // No enclosing definition found, skip this reference
		}

		// Create a "references" edge
		edgeHash := types.ComputeEdgeHash(sourceHash, targetHash, "references", "scip_resolved")
		edge := types.Edge{
			EdgeHash:   edgeHash,
			SourceHash: sourceHash,
			TargetHash: targetHash,
			EdgeType:   "references",
			Confidence: 0.95,
			Provenance: "scip_resolved",
		}

		if err := s.store.PutEdge(ctx, edge); err != nil {
			return fmt.Errorf("storing edge: %w", err)
		}
		result.EdgesCreated++
	}

	return nil
}

// findEnclosingDefinition finds the definition that encloses the given occurrence
// based on position. It returns the node hash of the enclosing symbol, or zero hash
// if none is found.
func findEnclosingDefinition(doc *scip.Document, ref *scip.Occurrence, symbolToHash map[string]types.Hash) types.Hash {
	refLine := int32(0)
	if len(ref.Range) >= 1 {
		refLine = ref.Range[0]
	}

	// Find the closest definition that appears before this reference
	var bestSymbol string
	var bestLine int32 = -1

	for _, occ := range doc.Occurrences {
		if occ.SymbolRoles&int32(scip.SymbolRole_Definition) == 0 {
			continue
		}
		if strings.HasPrefix(occ.Symbol, "local ") {
			continue
		}

		occLine := int32(0)
		if len(occ.Range) >= 1 {
			occLine = occ.Range[0]
		}

		// The enclosing definition should be before or at the reference line
		if occLine <= refLine && occLine > bestLine {
			bestLine = occLine
			bestSymbol = occ.Symbol
		}
	}

	if bestSymbol == "" {
		// Fallback: use the first non-local definition in the document
		for _, sym := range doc.Symbols {
			if !strings.HasPrefix(sym.Symbol, "local ") {
				if h, ok := symbolToHash[sym.Symbol]; ok {
					return h
				}
			}
		}
		return types.EmptyHash
	}

	if h, ok := symbolToHash[bestSymbol]; ok {
		return h
	}
	return types.EmptyHash
}

// formatQualifiedName builds the knowing qualified name format.
func formatQualifiedName(repo, pkgPath, symbolName string) string {
	if pkgPath != "" {
		return fmt.Sprintf("%s://%s.%s", repo, pkgPath, symbolName)
	}
	return fmt.Sprintf("%s://%s", repo, symbolName)
}

// computeFileHash computes a file hash from repo hash and relative path.
// This uses a simplified computation (without content hash) since we don't
// have the file content during SCIP ingestion.
func computeFileHash(repoHash types.Hash, relativePath string) types.Hash {
	data := fmt.Sprintf("%s\x00%s\x00", repoHash.String(), relativePath)
	return types.NewHash([]byte(data))
}

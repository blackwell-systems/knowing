package store

import (
	"context"
	"time"

	"github.com/blackwell-systems/knowing/internal/types"
)

// VocabAssociation represents a learned keyword -> symbol mapping.
type VocabAssociation struct {
	Keyword      string
	SymbolName   string
	SymbolHash   types.Hash
	Count        int
	LastSeen     int64
	SubgraphRoot types.Hash // Merkle root of symbol's package at recording time
}

// RecordVocabAssociation records or reinforces a keyword -> symbol association.
// Each call increments count by 1. Multiple observations of the same association
// increase confidence that the mapping is real (not noise).
// Optional subgraphRoot ties the association to the symbol's package state;
// when the package changes, the association expires via Merkle filtering.
func (s *SQLiteStore) RecordVocabAssociation(ctx context.Context, keyword string, symbolName string, symbolHash types.Hash, subgraphRoot ...types.Hash) error {
	now := time.Now().Unix()
	var rootBytes []byte
	if len(subgraphRoot) > 0 && subgraphRoot[0] != types.EmptyHash {
		rootBytes = subgraphRoot[0][:]
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO vocab_associations (keyword, symbol_name, symbol_hash, count, last_seen, subgraph_root)
		 VALUES (?, ?, ?, 1, ?, ?)
		 ON CONFLICT(keyword, symbol_hash) DO UPDATE SET
		   count = count + 1,
		   last_seen = ?,
		   subgraph_root = COALESCE(?, subgraph_root)`,
		keyword, symbolName, symbolHash[:], now, rootBytes, now, rootBytes,
	)
	return err
}

// LearnedVocabAssociations returns all associations for a set of keywords
// where count >= minCount. Used to generate learned equivalence classes
// for vocabulary expansion.
//
// When subgraphRoots is provided (symbol_hash -> current SubgraphRoot), only
// associations where subgraph_root is NULL or matches the current root are
// returned. This provides Merkle-based expiration: when code changes, old
// associations become invisible.
func (s *SQLiteStore) LearnedVocabAssociations(ctx context.Context, keywords []string, minCount int, subgraphRoots ...map[types.Hash]types.Hash) ([]VocabAssociation, error) {
	if len(keywords) == 0 {
		return nil, nil
	}

	// Build query with placeholders.
	query := `SELECT keyword, symbol_name, symbol_hash, count, last_seen, subgraph_root
		FROM vocab_associations
		WHERE count >= ? AND keyword IN (`
	args := []any{minCount}
	for i, kw := range keywords {
		if i > 0 {
			query += ","
		}
		query += "?"
		args = append(args, kw)
	}
	query += `) ORDER BY count DESC`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Extract subgraph roots map if provided.
	var roots map[types.Hash]types.Hash
	if len(subgraphRoots) > 0 {
		roots = subgraphRoots[0]
	}

	var results []VocabAssociation
	for rows.Next() {
		var va VocabAssociation
		var hashBytes, rootBytes []byte
		if err := rows.Scan(&va.Keyword, &va.SymbolName, &hashBytes, &va.Count, &va.LastSeen, &rootBytes); err != nil {
			return nil, err
		}
		if len(hashBytes) == 32 {
			copy(va.SymbolHash[:], hashBytes)
		}
		if len(rootBytes) == 32 {
			copy(va.SubgraphRoot[:], rootBytes)
		}

		// Merkle expiration: skip associations whose subgraph root doesn't match current.
		if roots != nil && va.SubgraphRoot != types.EmptyHash {
			currentRoot, exists := roots[va.SymbolHash]
			if exists && currentRoot != va.SubgraphRoot {
				continue // package changed, association expired
			}
		}

		results = append(results, va)
	}
	return results, rows.Err()
}

// LearnedVocabDetails returns a map of keyword -> []VocabAssocDetail for all
// associations matching the given keywords with count >= minCount.
// Includes observation counts for confidence-weighted scoring.
func (s *SQLiteStore) LearnedVocabDetails(ctx context.Context, keywords []string, minCount int, subgraphRoots ...map[types.Hash]types.Hash) (map[string][]struct {
	SymbolName string
	Count      int
}, error) {
	assocs, err := s.LearnedVocabAssociations(ctx, keywords, minCount, subgraphRoots...)
	if err != nil {
		return nil, err
	}
	if len(assocs) == 0 {
		return nil, nil
	}
	result := make(map[string][]struct {
		SymbolName string
		Count      int
	})
	for _, a := range assocs {
		result[a.Keyword] = append(result[a.Keyword], struct {
			SymbolName string
			Count      int
		}{a.SymbolName, a.Count})
	}
	return result, nil
}

// LearnedVocabTargets returns a map of keyword -> []symbolName for all
// associations matching the given keywords with count >= minCount.
// This satisfies the context.VocabProvider interface.
func (s *SQLiteStore) LearnedVocabTargets(ctx context.Context, keywords []string, minCount int) (map[string][]string, error) {
	assocs, err := s.LearnedVocabAssociations(ctx, keywords, minCount)
	if err != nil {
		return nil, err
	}
	if len(assocs) == 0 {
		return nil, nil
	}
	result := make(map[string][]string)
	for _, a := range assocs {
		result[a.Keyword] = append(result[a.Keyword], a.SymbolName)
	}
	return result, nil
}

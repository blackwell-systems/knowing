package store

import (
	"context"
	"time"

	"github.com/blackwell-systems/knowing/internal/types"
)

// VocabAssociation represents a learned keyword -> symbol mapping.
type VocabAssociation struct {
	Keyword    string
	SymbolName string
	SymbolHash types.Hash
	Count      int
	LastSeen   int64
}

// RecordVocabAssociation records or reinforces a keyword -> symbol association.
// Each call increments count by 1. Multiple observations of the same association
// increase confidence that the mapping is real (not noise).
func (s *SQLiteStore) RecordVocabAssociation(ctx context.Context, keyword string, symbolName string, symbolHash types.Hash) error {
	now := time.Now().Unix()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO vocab_associations (keyword, symbol_name, symbol_hash, count, last_seen)
		 VALUES (?, ?, ?, 1, ?)
		 ON CONFLICT(keyword, symbol_hash) DO UPDATE SET
		   count = count + 1,
		   last_seen = ?`,
		keyword, symbolName, symbolHash[:], now, now,
	)
	return err
}

// LearnedVocabAssociations returns all associations for a set of keywords
// where count >= minCount. Used to generate learned equivalence classes
// for vocabulary expansion.
func (s *SQLiteStore) LearnedVocabAssociations(ctx context.Context, keywords []string, minCount int) ([]VocabAssociation, error) {
	if len(keywords) == 0 {
		return nil, nil
	}

	// Build query with placeholders.
	query := `SELECT keyword, symbol_name, symbol_hash, count, last_seen
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

	var results []VocabAssociation
	for rows.Next() {
		var va VocabAssociation
		var hashBytes []byte
		if err := rows.Scan(&va.Keyword, &va.SymbolName, &hashBytes, &va.Count, &va.LastSeen); err != nil {
			return nil, err
		}
		if len(hashBytes) == 32 {
			copy(va.SymbolHash[:], hashBytes)
		}
		results = append(results, va)
	}
	return results, rows.Err()
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

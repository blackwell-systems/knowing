-- Add symbol_name column to FTS content table for terminal symbol matching.
-- This stores just the terminal identifier (e.g., "QuerySet.filter" instead of
-- the full "github.com/django/django://django/db/models/query.py.QuerySet.filter").
-- BM25 weighting gives symbol_name the highest score so keyword searches for
-- "before_request" rank symbols by their actual name, not by path token frequency.

ALTER TABLE nodes_fts_content ADD COLUMN symbol_name TEXT NOT NULL DEFAULT '';

-- Recreate the FTS virtual table with the new column.
-- FTS5 doesn't support ALTER, so we drop and recreate.
DROP TABLE IF EXISTS nodes_fts;

CREATE VIRTUAL TABLE nodes_fts USING fts5(
    symbol_name,
    qualified_name,
    signature,
    file_path,
    content='nodes_fts_content',
    content_rowid='rowid',
    tokenize="unicode61 tokenchars '_' remove_diacritics 2"
);

-- Rebuild FTS index from content table.
INSERT INTO nodes_fts(nodes_fts) VALUES('rebuild');

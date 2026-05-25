-- Add doc column to FTS for docstring-based BM25 retrieval.
-- Docstrings are natural-language descriptions of what code does.
-- Task descriptions are also natural language. BM25 on docstrings bridges
-- the vocabulary gap between how developers describe tasks and how symbols
-- are named (e.g., "migration operation" matches a function whose docstring
-- says "Apply each operation in the migration").

ALTER TABLE nodes_fts_content ADD COLUMN doc TEXT NOT NULL DEFAULT '';

-- Recreate FTS virtual table with the new column.
DROP TABLE IF EXISTS nodes_fts;

CREATE VIRTUAL TABLE nodes_fts USING fts5(
    symbol_name,
    concepts,
    qualified_name,
    signature,
    file_path,
    doc,
    content='nodes_fts_content',
    content_rowid='rowid',
    tokenize="unicode61 tokenchars '_' remove_diacritics 2"
);

-- Populate doc column from existing nodes table.
UPDATE nodes_fts_content SET doc = COALESCE(
    (SELECT n.doc FROM nodes n WHERE n.node_hash = nodes_fts_content.node_hash),
    ''
);

-- Rebuild FTS index from content table.
INSERT INTO nodes_fts(nodes_fts) VALUES('rebuild');

-- Add concepts column to FTS for module/file-name derived concept matching.
-- Stores CamelCase-split words from the file name and parent module path.
-- "commandLineParser.ts" -> "command Line Parser" (searchable concepts)
-- This bridges the vocabulary gap: developers say "compiler option" but the
-- implementation lives in "commandLineParser". Searching "parser" now finds it.

ALTER TABLE nodes_fts_content ADD COLUMN concepts TEXT NOT NULL DEFAULT '';

-- Recreate FTS virtual table with the new column.
DROP TABLE IF EXISTS nodes_fts;

CREATE VIRTUAL TABLE nodes_fts USING fts5(
    symbol_name,
    concepts,
    qualified_name,
    signature,
    file_path,
    content='nodes_fts_content',
    content_rowid='rowid',
    tokenize="unicode61 tokenchars '_' remove_diacritics 2"
);

-- Rebuild FTS index from content table.
INSERT INTO nodes_fts(nodes_fts) VALUES('rebuild');

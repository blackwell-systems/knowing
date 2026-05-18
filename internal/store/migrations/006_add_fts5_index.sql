-- BM25 full-text search over node metadata for seed discovery.
-- Uses content-sync with the nodes table via triggers.
CREATE VIRTUAL TABLE IF NOT EXISTS nodes_fts USING fts5(
    qualified_name,
    signature,
    file_path,
    content='nodes_fts_content',
    content_rowid='rowid',
    tokenize='unicode61 remove_diacritics 2'
);

-- Backing content table that maps rowid to node_hash for result lookup.
CREATE TABLE IF NOT EXISTS nodes_fts_content (
    rowid INTEGER PRIMARY KEY AUTOINCREMENT,
    node_hash BLOB NOT NULL,
    qualified_name TEXT NOT NULL,
    signature TEXT NOT NULL DEFAULT '',
    file_path TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_nodes_fts_content_hash ON nodes_fts_content(node_hash);

-- Populate from existing data.
INSERT INTO nodes_fts_content(node_hash, qualified_name, signature, file_path)
SELECT
    n.node_hash,
    n.qualified_name,
    COALESCE(n.signature, ''),
    COALESCE(f.path, '')
FROM nodes n
LEFT JOIN files f ON n.file_hash = f.file_hash;

-- Rebuild FTS index from content table.
INSERT INTO nodes_fts(nodes_fts) VALUES('rebuild');

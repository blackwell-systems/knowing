-- Add subgraph_root column to vocab_associations for Merkle-based expiration.
-- When the SubgraphRoot of a symbol's package changes, associations recorded
-- under the old root become invisible (same pattern as feedback neighborhood_root).
ALTER TABLE vocab_associations ADD COLUMN subgraph_root BLOB;
CREATE INDEX idx_vocab_subgraph ON vocab_associations(subgraph_root);

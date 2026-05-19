-- Add indexed_at epoch timestamp to nodes and edges for GC freshness signal.
-- Recently indexed objects are safe to keep; old objects may be orphaned.
-- Default 0 means "not yet recorded" (pre-migration rows are treated as stale).
ALTER TABLE nodes ADD COLUMN indexed_at INTEGER DEFAULT 0;
ALTER TABLE edges ADD COLUMN indexed_at INTEGER DEFAULT 0;

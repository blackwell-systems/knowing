-- Store full edge data in edge_events so removed-edge diffs work correctly.
-- Previously edge_events only stored edge_hash and joined back to edges,
-- but removed edges are deleted from the edges table, making the JOIN
-- return empty results for "removed" events.
--
-- These columns are NULL for existing events (backward compatible).
-- New events populate all fields. SnapshotDiff uses these columns
-- directly instead of joining to the edges table.
ALTER TABLE edge_events ADD COLUMN source_hash BLOB;
ALTER TABLE edge_events ADD COLUMN target_hash BLOB;
ALTER TABLE edge_events ADD COLUMN edge_type TEXT;
ALTER TABLE edge_events ADD COLUMN confidence REAL;
ALTER TABLE edge_events ADD COLUMN provenance TEXT;

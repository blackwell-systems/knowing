-- General-purpose notes table for metadata that should never affect Merkle
-- computation. Modeled after git notes: attach arbitrary key/value pairs to
-- any content-addressed object (node, edge, snapshot, community, pack root).
--
-- Use cases: community assignments, context pack persistence, quality scores,
-- feedback annotations, agent session state.
--
-- Composite primary key (object_hash, key) means one value per key per object.
-- INSERT OR REPLACE gives upsert semantics matching the rest of the schema.
CREATE TABLE graph_notes (
    object_hash BLOB    NOT NULL,
    key         TEXT    NOT NULL,
    value       TEXT    NOT NULL,
    updated_at  INTEGER NOT NULL,
    PRIMARY KEY (object_hash, key)
);
CREATE INDEX idx_notes_object ON graph_notes(object_hash);
CREATE INDEX idx_notes_key ON graph_notes(key);

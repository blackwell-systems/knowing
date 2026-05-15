CREATE TABLE repos (
    repo_hash   BLOB PRIMARY KEY,
    repo_url    TEXT NOT NULL,
    last_commit TEXT,
    last_indexed INTEGER
);
CREATE TABLE files (
    file_hash    BLOB PRIMARY KEY,
    repo_hash    BLOB NOT NULL REFERENCES repos(repo_hash),
    path         TEXT NOT NULL,
    content_hash BLOB NOT NULL
);
CREATE TABLE nodes (
    node_hash      BLOB PRIMARY KEY,
    file_hash      BLOB NOT NULL REFERENCES files(file_hash),
    qualified_name TEXT NOT NULL,
    kind           TEXT NOT NULL,
    line           INTEGER,
    signature      TEXT
);
CREATE TABLE edges (
    edge_hash    BLOB PRIMARY KEY,
    source_hash  BLOB NOT NULL REFERENCES nodes(node_hash),
    target_hash  BLOB NOT NULL REFERENCES nodes(node_hash),
    edge_type    TEXT NOT NULL,
    confidence   REAL NOT NULL DEFAULT 1.0,
    provenance   TEXT NOT NULL DEFAULT 'ast_resolved'
);
CREATE TABLE edge_events (
    event_id      INTEGER PRIMARY KEY AUTOINCREMENT,
    edge_hash     BLOB NOT NULL,
    event_type    TEXT NOT NULL,
    snapshot_hash BLOB NOT NULL,
    source_commit TEXT NOT NULL,
    indexer_ver   TEXT NOT NULL,
    timestamp     INTEGER NOT NULL
);
CREATE TABLE snapshots (
    snapshot_hash BLOB PRIMARY KEY,
    parent_hash   BLOB REFERENCES snapshots(snapshot_hash),
    repo_hash     BLOB NOT NULL REFERENCES repos(repo_hash),
    commit_hash   TEXT NOT NULL,
    timestamp     INTEGER NOT NULL,
    node_count    INTEGER NOT NULL,
    edge_count    INTEGER NOT NULL
);
CREATE TABLE schema_version (version INTEGER PRIMARY KEY);
CREATE INDEX idx_nodes_qualified ON nodes(qualified_name);
CREATE INDEX idx_nodes_file ON nodes(file_hash);
CREATE INDEX idx_edges_source ON edges(source_hash);
CREATE INDEX idx_edges_target ON edges(target_hash);
CREATE INDEX idx_edges_type ON edges(edge_type);
CREATE INDEX idx_edge_events_snapshot ON edge_events(snapshot_hash);
CREATE INDEX idx_files_repo ON files(repo_hash);

CREATE TABLE IF NOT EXISTS embeddings (
    node_hash  BLOB NOT NULL,
    model      TEXT NOT NULL,
    vector     BLOB NOT NULL,
    PRIMARY KEY (node_hash, model)
);

CREATE TABLE vocab_associations (
    keyword      TEXT NOT NULL,
    symbol_name  TEXT NOT NULL,
    symbol_hash  BLOB NOT NULL,
    count        INTEGER DEFAULT 1,
    last_seen    INTEGER NOT NULL,
    UNIQUE(keyword, symbol_hash)
);
CREATE INDEX idx_vocab_keyword ON vocab_associations(keyword);

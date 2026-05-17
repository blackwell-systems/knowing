CREATE TABLE feedback (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    symbol_hash  BLOB NOT NULL,
    session_id   TEXT NOT NULL,
    useful       INTEGER NOT NULL,
    timestamp    INTEGER NOT NULL
);
CREATE INDEX idx_feedback_symbol ON feedback(symbol_hash);
CREATE INDEX idx_feedback_session ON feedback(session_id);

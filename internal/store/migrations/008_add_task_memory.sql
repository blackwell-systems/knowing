-- Task memory: links task keywords to symbols that were useful.
-- Populated passively from context_for_task results + session tracking.
-- Queried at retrieval time to boost symbols from similar past tasks.
CREATE TABLE IF NOT EXISTS task_memory (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    keywords     TEXT NOT NULL,     -- space-separated normalized keywords from task description
    symbol_hash  BLOB NOT NULL,     -- symbol that was returned and accessed
    score        REAL DEFAULT 1.0,  -- positive = useful, negative = noise
    timestamp    INTEGER NOT NULL   -- unix seconds
);

CREATE INDEX IF NOT EXISTS idx_task_memory_keywords ON task_memory(keywords);
CREATE INDEX IF NOT EXISTS idx_task_memory_symbol ON task_memory(symbol_hash);

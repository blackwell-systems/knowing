-- Add git blame metadata columns to nodes table.
-- Populated by `knowing enrich blame` which runs git blame on each
-- file and stamps last-author + last-commit timestamp on every symbol.
ALTER TABLE nodes ADD COLUMN last_author TEXT DEFAULT '';
ALTER TABLE nodes ADD COLUMN last_commit_at INTEGER DEFAULT 0;

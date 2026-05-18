-- Add test coverage column to nodes table.
-- Populated by `knowing enrich coverage` which parses Go cover profiles
-- (or lcov for other languages) and stamps coverage percentage per symbol.
-- Default -1 means "not measured" (distinguishes from 0% coverage).
ALTER TABLE nodes ADD COLUMN coverage_pct REAL DEFAULT -1;

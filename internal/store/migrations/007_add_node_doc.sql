-- Add doc comment column to nodes table.
-- Stores the first ~200 chars of the documentation comment preceding
-- a symbol declaration. Populated by extractors at index time.
ALTER TABLE nodes ADD COLUMN doc TEXT DEFAULT '';

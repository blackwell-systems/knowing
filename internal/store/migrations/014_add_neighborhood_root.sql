-- Add neighborhood_root column to feedback table for merkleized expiration.
-- When the SubgraphRoot of a symbol's package changes, feedback recorded under
-- the old neighborhood becomes stale and should not influence future rankings.

ALTER TABLE feedback ADD COLUMN neighborhood_root BLOB;
CREATE INDEX idx_feedback_neighborhood ON feedback(neighborhood_root);

ALTER TABLE feedback ADD COLUMN keyword_cluster BLOB;
CREATE INDEX idx_feedback_cluster ON feedback(keyword_cluster);

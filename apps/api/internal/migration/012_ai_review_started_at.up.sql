ALTER TABLE ai_reviews
  ADD COLUMN started_at TIMESTAMP NULL AFTER created_at;

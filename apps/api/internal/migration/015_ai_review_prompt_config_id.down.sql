ALTER TABLE ai_reviews
  DROP FOREIGN KEY fk_ai_reviews_prompt_config,
  DROP INDEX idx_ai_reviews_prompt_config,
  DROP COLUMN prompt_config_id;

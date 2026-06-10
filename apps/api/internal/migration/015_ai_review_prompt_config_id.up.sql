ALTER TABLE ai_reviews
  ADD COLUMN prompt_config_id BIGINT NULL AFTER revision_id,
  ADD INDEX idx_ai_reviews_prompt_config (prompt_config_id),
  ADD CONSTRAINT fk_ai_reviews_prompt_config FOREIGN KEY (prompt_config_id) REFERENCES ai_prompt_configs(id) ON DELETE SET NULL;

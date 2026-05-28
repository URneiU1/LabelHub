ALTER TABLE ai_dry_runs
  ADD COLUMN golden_sample_id BIGINT NULL AFTER ai_prompt_id,
  ADD COLUMN prompt_version INT NOT NULL DEFAULT 1 AFTER golden_sample_id,
  ADD COLUMN payload_snapshot JSON NULL AFTER prompt_version,
  ADD COLUMN expected_answer_snapshot JSON NULL AFTER payload_snapshot,
  ADD COLUMN expected_verdict ENUM('pass','reject','uncertain') NULL AFTER expected_answer_snapshot,
  ADD COLUMN actual_verdict ENUM('pass','reject','uncertain') NULL AFTER expected_verdict,
  ADD COLUMN matched_expected TINYINT(1) NULL AFTER actual_verdict,
  ADD INDEX idx_dry_run_golden_sample (golden_sample_id),
  ADD CONSTRAINT fk_dry_run_golden_sample FOREIGN KEY (golden_sample_id) REFERENCES golden_samples(id) ON DELETE SET NULL;

UPDATE ai_dry_runs AS dry_runs
JOIN ai_prompt_configs AS prompts ON prompts.id = dry_runs.ai_prompt_id
SET dry_runs.prompt_version = prompts.version;

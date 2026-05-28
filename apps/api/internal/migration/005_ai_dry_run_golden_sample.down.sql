ALTER TABLE ai_dry_runs
  DROP FOREIGN KEY fk_dry_run_golden_sample,
  DROP INDEX idx_dry_run_golden_sample,
  DROP COLUMN matched_expected,
  DROP COLUMN actual_verdict,
  DROP COLUMN expected_verdict,
  DROP COLUMN expected_answer_snapshot,
  DROP COLUMN payload_snapshot,
  DROP COLUMN prompt_version,
  DROP COLUMN golden_sample_id;

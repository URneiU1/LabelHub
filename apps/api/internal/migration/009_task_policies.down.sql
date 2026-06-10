ALTER TABLE tasks
  DROP COLUMN daily_submission_limit_per_labeler,
  DROP COLUMN review_sampling_pct,
  DROP COLUMN lease_timeout_minutes,
  DROP COLUMN overlap_coverage_pct,
  DROP COLUMN overlap_count;

-- S8 task policy controls. Defaults preserve the pre-S8 behavior:
-- single annotation, no overlap expansion, 30 minute lease, full review, no daily cap.
ALTER TABLE tasks
  ADD COLUMN overlap_count INT NOT NULL DEFAULT 1 AFTER quota_per_user,
  ADD COLUMN overlap_coverage_pct INT NOT NULL DEFAULT 0 AFTER overlap_count,
  ADD COLUMN lease_timeout_minutes INT NOT NULL DEFAULT 30 AFTER overlap_coverage_pct,
  ADD COLUMN review_sampling_pct INT NOT NULL DEFAULT 100 AFTER lease_timeout_minutes,
  ADD COLUMN daily_submission_limit_per_labeler INT NOT NULL DEFAULT 0 AFTER review_sampling_pct;

-- LabelHub initial schema — 17 tables
-- Sprint 1: minimum viable schema, all core entities

SET NAMES utf8mb4;
SET FOREIGN_KEY_CHECKS = 0;

-- ============================================================
-- 1. users — no single role column, roles via user_roles M2M
-- ============================================================
CREATE TABLE users (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  username VARCHAR(64) NOT NULL,
  password_hash VARCHAR(255) NOT NULL,
  display_name VARCHAR(64) NOT NULL DEFAULT '',
  email VARCHAR(128) NOT NULL DEFAULT '',
  status ENUM('active','disabled') NOT NULL DEFAULT 'active',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY uk_username (username)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ============================================================
-- 2. user_roles — many-to-many, support owner+reviewer overlap
-- ============================================================
CREATE TABLE user_roles (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  user_id BIGINT NOT NULL,
  role ENUM('owner','labeler','reviewer','admin','system') NOT NULL,
  UNIQUE KEY uk_user_role (user_id, role),
  CONSTRAINT fk_user_roles_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ============================================================
-- 3. tasks — owner creates, distributes, publishes
-- ============================================================
CREATE TABLE tasks (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  owner_id BIGINT NOT NULL,
  title VARCHAR(200) NOT NULL,
  description TEXT,
  rich_description JSON COMMENT 'TipTap rich text',
  tags JSON COMMENT '["tag1","tag2"]',
  reward_config JSON COMMENT '奖励规则',
  baseline_description TEXT COMMENT 'AI审核上下文/数据通过基线',
  status ENUM('draft','published','paused','archived') NOT NULL DEFAULT 'draft',
  template_id BIGINT NULL,
  distribution ENUM('first_come','assigned','quota') NOT NULL DEFAULT 'first_come',
  quota_per_user INT NOT NULL DEFAULT 0,
  ai_review_enabled TINYINT(1) NOT NULL DEFAULT 0,
  human_review_enabled TINYINT(1) NOT NULL DEFAULT 1,
  ai_prompt_id BIGINT NULL,
  total_items INT NOT NULL DEFAULT 0,
  finished_items INT NOT NULL DEFAULT 0,
  deadline TIMESTAMP NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  published_at TIMESTAMP NULL,
  INDEX idx_status_owner (status, owner_id),
  INDEX idx_published (status, published_at),
  CONSTRAINT fk_tasks_owner FOREIGN KEY (owner_id) REFERENCES users(id) ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ============================================================
-- 4. task_templates — versioned JSON schema per task
-- ============================================================
CREATE TABLE task_templates (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  task_id BIGINT NOT NULL,
  version INT NOT NULL DEFAULT 1,
  schema_json JSON NOT NULL,
  schema_hash CHAR(64),
  field_map JSON COMMENT '默认导出字段映射',
  created_by BIGINT NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uk_task_version (task_id, version),
  CONSTRAINT fk_templates_task FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE,
  CONSTRAINT fk_templates_user FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- FK: tasks.template_id → task_templates.id (added after both tables exist)
ALTER TABLE tasks ADD CONSTRAINT fk_tasks_template
  FOREIGN KEY (template_id) REFERENCES task_templates(id) ON DELETE SET NULL;

-- ============================================================
-- 5. task_items — raw data items to annotate
-- ============================================================
CREATE TABLE task_items (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  task_id BIGINT NOT NULL,
  external_id VARCHAR(128),
  payload JSON NOT NULL COMMENT '原始数据',
  status ENUM('available','claimed','finished') NOT NULL DEFAULT 'available',
  claimed_by BIGINT NULL,
  claimed_at TIMESTAMP NULL,
  finished_at TIMESTAMP NULL,
  priority INT NOT NULL DEFAULT 0,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uk_task_external (task_id, external_id),
  INDEX idx_task_status (task_id, status),
  INDEX idx_claim (task_id, status, claimed_by),
  CONSTRAINT fk_items_task FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE,
  CONSTRAINT fk_items_claimant FOREIGN KEY (claimed_by) REFERENCES users(id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ============================================================
-- 6. task_assignees — assigned / quota distribution
-- ============================================================
CREATE TABLE task_assignees (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  task_id BIGINT NOT NULL,
  user_id BIGINT NOT NULL,
  item_id BIGINT NULL,
  assigned_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uk_task_user_item (task_id, user_id, item_id),
  CONSTRAINT fk_assign_task FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE,
  CONSTRAINT fk_assign_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ============================================================
-- 7. submissions — 7-state enum, ai_verdict as field not state
-- ============================================================
CREATE TABLE submissions (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  task_id BIGINT NOT NULL,
  item_id BIGINT NOT NULL,
  template_version INT NOT NULL DEFAULT 1,
  labeler_id BIGINT NOT NULL,
  current_revision_id BIGINT NULL,
  status ENUM('draft','submitted','ai_reviewing','human_reviewing','revising','approved','rejected') NOT NULL DEFAULT 'draft',
  ai_verdict ENUM('pass','reject','uncertain') NULL,
  ai_score DECIMAL(5,2) NULL,
  human_verdict ENUM('approve','reject','revise') NULL COMMENT 'latest human verdict, denormalized from human_reviews for queries; source of truth is human_reviews table',
  submitted_at TIMESTAMP NULL,
  approved_at TIMESTAMP NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  UNIQUE KEY uk_item (item_id),
  INDEX idx_task_status (task_id, status),
  INDEX idx_labeler (labeler_id, status),
  CONSTRAINT fk_sub_task FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE RESTRICT,
  CONSTRAINT fk_sub_item FOREIGN KEY (item_id) REFERENCES task_items(id) ON DELETE RESTRICT,
  CONSTRAINT fk_sub_labeler FOREIGN KEY (labeler_id) REFERENCES users(id) ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ============================================================
-- 8. submission_revisions — append-only snapshots
-- ============================================================
CREATE TABLE submission_revisions (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  submission_id BIGINT NOT NULL,
  revision_no INT NOT NULL,
  answer JSON NOT NULL,
  draft TINYINT(1) NOT NULL DEFAULT 0,
  created_by BIGINT NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uk_sub_rev (submission_id, revision_no),
  CONSTRAINT fk_rev_sub FOREIGN KEY (submission_id) REFERENCES submissions(id) ON DELETE CASCADE,
  CONSTRAINT fk_rev_user FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- FK: submissions.current_revision_id → submission_revisions.id
ALTER TABLE submissions ADD CONSTRAINT fk_sub_current_rev
  FOREIGN KEY (current_revision_id) REFERENCES submission_revisions(id) ON DELETE SET NULL;

-- ============================================================
-- 9. ai_reviews — append-only, idempotency key
-- ============================================================
CREATE TABLE ai_reviews (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  submission_id BIGINT NOT NULL,
  revision_id BIGINT NOT NULL,
  idempotency_key CHAR(64) NOT NULL,
  prompt_version INT NOT NULL DEFAULT 1,
  verdict ENUM('pass','reject','uncertain') NULL,
  overall_score DECIMAL(5,2),
  dimensions JSON COMMENT '[{"name":"相关性","score":9,"reason":"..."}]',
  reason TEXT,
  raw_response JSON,
  tokens_input INT DEFAULT 0,
  tokens_output INT DEFAULT 0,
  latency_ms INT DEFAULT 0,
  status ENUM('pending','running','succeeded','failed','dead') NOT NULL DEFAULT 'pending',
  retry_count INT NOT NULL DEFAULT 0,
  error_msg TEXT,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  finished_at TIMESTAMP NULL,
  UNIQUE KEY uk_idempotency (idempotency_key),
  INDEX idx_sub (submission_id),
  INDEX idx_status (status),
  CONSTRAINT fk_ai_rev_sub FOREIGN KEY (submission_id) REFERENCES submissions(id) ON DELETE CASCADE,
  CONSTRAINT fk_ai_rev_rev FOREIGN KEY (revision_id) REFERENCES submission_revisions(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ============================================================
-- 10. human_reviews — append-only
-- ============================================================
CREATE TABLE human_reviews (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  submission_id BIGINT NOT NULL,
  revision_id BIGINT NOT NULL,
  reviewer_id BIGINT NOT NULL,
  stage ENUM('first','second','final') NOT NULL DEFAULT 'first',
  verdict ENUM('approve','reject','revise') NOT NULL,
  reason TEXT,
  patch JSON NULL COMMENT '审核员修订建议',
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  INDEX idx_sub (submission_id),
  CONSTRAINT fk_human_rev_sub FOREIGN KEY (submission_id) REFERENCES submissions(id) ON DELETE CASCADE,
  CONSTRAINT fk_human_rev_rev FOREIGN KEY (revision_id) REFERENCES submission_revisions(id) ON DELETE CASCADE,
  CONSTRAINT fk_human_rev_user FOREIGN KEY (reviewer_id) REFERENCES users(id) ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ============================================================
-- 11. audit_logs — append-only, all state transitions
-- ============================================================
CREATE TABLE audit_logs (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  entity_type ENUM('task','submission','task_item','export') NOT NULL,
  entity_id BIGINT NOT NULL,
  from_state VARCHAR(32),
  to_state VARCHAR(32) NOT NULL,
  actor_type ENUM('user','system','ai_worker') NOT NULL,
  actor_id BIGINT NULL,
  event VARCHAR(64) NOT NULL,
  payload JSON,
  created_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
  INDEX idx_entity (entity_type, entity_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ============================================================
-- 12. uploaded_files — upload metadata and orphan cleanup state
-- ============================================================
CREATE TABLE uploaded_files (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  task_id BIGINT NOT NULL,
  submission_revision_id BIGINT NULL,
  storage_key VARCHAR(255) NOT NULL,
  original_name VARCHAR(255) NOT NULL,
  mime_type VARCHAR(128) NOT NULL,
  size_bytes BIGINT NOT NULL,
  status ENUM('temp','attached','deleted') NOT NULL DEFAULT 'temp',
  created_by BIGINT NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  attached_at TIMESTAMP NULL,
  UNIQUE KEY uk_storage_key (storage_key),
  INDEX idx_status_created (status, created_at),
  INDEX idx_task (task_id),
  CONSTRAINT fk_uploaded_task FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE,
  CONSTRAINT fk_uploaded_revision FOREIGN KEY (submission_revision_id) REFERENCES submission_revisions(id) ON DELETE SET NULL,
  CONSTRAINT fk_uploaded_user FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ============================================================
-- 13. exports — async export jobs
-- ============================================================
CREATE TABLE exports (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  task_id BIGINT NOT NULL,
  created_by BIGINT NOT NULL,
  format ENUM('json','jsonl','csv','xlsx') NOT NULL,
  filter JSON,
  field_map JSON,
  include_reviews TINYINT(1) NOT NULL DEFAULT 0,
  status ENUM('queued','running','succeeded','failed') NOT NULL DEFAULT 'queued',
  file_path VARCHAR(512),
  file_size BIGINT,
  row_count INT,
  error_msg TEXT,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  finished_at TIMESTAMP NULL,
  INDEX idx_task_user (task_id, created_by),
  CONSTRAINT fk_export_task FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE,
  CONSTRAINT fk_export_user FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ============================================================
-- 14. ai_prompt_configs — versioned per task
-- ============================================================
CREATE TABLE ai_prompt_configs (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  task_id BIGINT NOT NULL,
  version INT NOT NULL DEFAULT 1,
  prompt_template TEXT NOT NULL COMMENT '含 {{payload.*}} {{answer.*}} {{task.baseline_description}} 占位',
  dimensions JSON NOT NULL COMMENT '[{"name":"相关性","weight":0.3,"description":"..."}]',
  pass_threshold DECIMAL(5,2) NOT NULL DEFAULT 80.00,
  uncertain_min DECIMAL(5,2) NOT NULL DEFAULT 60.00 COMMENT '60~80分之间走uncertain',
  model VARCHAR(64) NOT NULL DEFAULT 'doubao-seed-2.0-lite',
  created_by BIGINT NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uk_task_version (task_id, version),
  CONSTRAINT fk_prompt_task FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE,
  CONSTRAINT fk_prompt_user FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- FK: tasks.ai_prompt_id → ai_prompt_configs.id
ALTER TABLE tasks ADD CONSTRAINT fk_tasks_ai_prompt
  FOREIGN KEY (ai_prompt_id) REFERENCES ai_prompt_configs(id) ON DELETE SET NULL;

-- ============================================================
-- 15. golden_samples — known-answer samples for AI dry-run
-- ============================================================
CREATE TABLE golden_samples (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  task_id BIGINT NOT NULL,
  ai_prompt_id BIGINT NULL COMMENT 'optional: associated prompt version',
  payload JSON NOT NULL,
  payload_hash CHAR(64) NOT NULL,
  expected_answer JSON NOT NULL,
  expected_verdict ENUM('pass','reject','uncertain') NOT NULL,
  notes TEXT,
  created_by BIGINT NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uk_task_payload_hash (task_id, payload_hash),
  INDEX idx_task (task_id),
  CONSTRAINT fk_golden_task FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE,
  CONSTRAINT fk_golden_prompt FOREIGN KEY (ai_prompt_id) REFERENCES ai_prompt_configs(id) ON DELETE SET NULL,
  CONSTRAINT fk_golden_user FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ============================================================
-- 16. ai_dry_runs — async golden-sample dry-run results
-- ============================================================
CREATE TABLE ai_dry_runs (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  task_id BIGINT NOT NULL,
  ai_prompt_id BIGINT NOT NULL,
  status ENUM('queued','running','succeeded','failed') NOT NULL DEFAULT 'queued',
  result JSON,
  error_msg TEXT,
  created_by BIGINT NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  finished_at TIMESTAMP NULL,
  INDEX idx_task_created (task_id, created_at),
  CONSTRAINT fk_dry_run_task FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE,
  CONSTRAINT fk_dry_run_prompt FOREIGN KEY (ai_prompt_id) REFERENCES ai_prompt_configs(id) ON DELETE CASCADE,
  CONSTRAINT fk_dry_run_user FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE RESTRICT
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ============================================================
-- 17. outbox_events — transactional outbox for async tasks
-- ============================================================
CREATE TABLE outbox_events (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  topic VARCHAR(64) NOT NULL,
  payload JSON NOT NULL,
  status ENUM('pending','published','failed') NOT NULL DEFAULT 'pending',
  retry_count INT NOT NULL DEFAULT 0,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  published_at TIMESTAMP NULL,
  INDEX idx_status (status, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

SET FOREIGN_KEY_CHECKS = 1;

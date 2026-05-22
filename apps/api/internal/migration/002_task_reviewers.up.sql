SET NAMES utf8mb4;

CREATE TABLE IF NOT EXISTS task_reviewers (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  task_id BIGINT NOT NULL,
  user_id BIGINT NOT NULL,
  assigned_by BIGINT NULL,
  assigned_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uk_task_reviewer (task_id, user_id),
  INDEX idx_reviewer_task (user_id, task_id),
  CONSTRAINT fk_task_reviewers_task FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE,
  CONSTRAINT fk_task_reviewers_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
  CONSTRAINT fk_task_reviewers_assigner FOREIGN KEY (assigned_by) REFERENCES users(id) ON DELETE SET NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

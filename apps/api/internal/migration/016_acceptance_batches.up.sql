-- acceptance_batches / acceptance_spot_checks:Owner 数据验收强闭环。
-- 一次验收 = 对某任务当前「已通过(approved)」数据的快照批次;Owner 抽检后做「验收通过/不通过」。
-- 验收不通过会把被标记(flag)的已通过提交打回人工复审(状态机 approved -> human_reviewing)。
-- 验收只是状态标记,不闸导出。每任务至多一个 pending 批次(由服务层保证)。
CREATE TABLE acceptance_batches (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  task_id BIGINT NOT NULL,
  status ENUM('pending', 'accepted', 'rejected') NOT NULL DEFAULT 'pending',
  approved_count INT NOT NULL DEFAULT 0,
  note TEXT NULL DEFAULT NULL,
  decided_by BIGINT NULL DEFAULT NULL,
  decided_at TIMESTAMP NULL DEFAULT NULL,
  created_by BIGINT NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  KEY idx_acceptance_task (task_id, status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE acceptance_spot_checks (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  batch_id BIGINT NOT NULL,
  submission_id BIGINT NOT NULL,
  result ENUM('ok', 'flag') NOT NULL,
  note TEXT NULL DEFAULT NULL,
  checked_by BIGINT NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uk_batch_submission (batch_id, submission_id),
  KEY idx_spotcheck_batch (batch_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

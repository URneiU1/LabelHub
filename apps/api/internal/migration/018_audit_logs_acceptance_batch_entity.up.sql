-- 018: audit_logs 的 ENUM 列补值,支持数据验收(acceptance)写审计。
-- migration 016 引入验收(acceptance_batches),acceptance 服务写 audit 日志时用
-- entity_type='acceptance_batch' 且 actor_type='owner'(见 service/acceptance),但这两个 ENUM 列
-- 当时都未加新值,导致「发起数据验收」写审计触发 MySQL 1265 (Data truncated) → 验收事务回滚
-- → handler 返回 "acceptance operation failed"。这里把两个 ENUM 都补全。
ALTER TABLE audit_logs MODIFY COLUMN entity_type
  ENUM('task', 'submission', 'task_item', 'export', 'acceptance_batch') NOT NULL;
ALTER TABLE audit_logs MODIFY COLUMN actor_type
  ENUM('user', 'system', 'ai_worker', 'owner') NOT NULL;

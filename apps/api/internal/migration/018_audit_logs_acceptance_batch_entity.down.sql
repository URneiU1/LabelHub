-- 回滚 018:先删除验收相关审计行(否则收窄 ENUM 会触发 1265),再还原两个 ENUM。
DELETE FROM audit_logs WHERE entity_type = 'acceptance_batch' OR actor_type = 'owner';
ALTER TABLE audit_logs MODIFY COLUMN entity_type
  ENUM('task', 'submission', 'task_item', 'export') NOT NULL;
ALTER TABLE audit_logs MODIFY COLUMN actor_type
  ENUM('user', 'system', 'ai_worker') NOT NULL;

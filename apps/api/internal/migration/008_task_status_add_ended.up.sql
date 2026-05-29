-- 任务状态机(statemachine/task.go)终态为 'ended',但 001 的 tasks.status ENUM
-- 只到 'archived',导致 Owner「下线 / EndTask」执行 UPDATE status='ended' 时
-- 触发 MySQL Error 1265 (Data truncated for column 'status')——线上「下线」对任意
-- 任务都会失败。扩展 enum 加入 'ended'(保留 'archived' 以向后兼容历史数据)。
ALTER TABLE tasks
  MODIFY status ENUM('draft','published','paused','archived','ended') NOT NULL DEFAULT 'draft';

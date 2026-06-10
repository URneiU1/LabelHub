-- 回滚:先把 'ended' 的任务退回 'paused'(否则收缩 enum 会再次 truncate),
-- 再把 status enum 还原到 008 之前的取值集合。
UPDATE tasks SET status = 'paused' WHERE status = 'ended';
ALTER TABLE tasks
  MODIFY status ENUM('draft','published','paused','archived') NOT NULL DEFAULT 'draft';

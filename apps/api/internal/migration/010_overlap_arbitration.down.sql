-- 回滚 010_overlap_arbitration:把 (item_id, labeler_id) 唯一键收回单列 item_id 唯一,
-- 并去掉 needs_arbitration 状态。
--
-- H-07 前置守卫:up 已允许同一 item 存在多份 submission(overlap),若此时直接恢复
-- uk_item(item_id),会在重复 item_id 上抛 "Duplicate entry" 在迁移中途失败,且此前已经
-- 改过的状态/枚举无法自动还原。这里在做任何变更前先检测:一旦存在重复 item_id,下面的
-- 标量子查询会返回 2 行,触发 "Subquery returns more than 1 row" 报错使整个回滚立即中止
-- (DO 不返回结果集,适配 multiStatements Exec)。操作者应据此先归档重复 submission 或从
-- 备份恢复,再执行回滚 —— 绝不在回滚时静默删除 overlap 提交数据。
DO (
  SELECT guard_pair.x
  FROM (SELECT 1 AS x UNION ALL SELECT 2 AS x) AS guard_pair
  WHERE EXISTS (
    SELECT 1 FROM submissions GROUP BY item_id HAVING COUNT(*) > 1
  )
);

-- 守卫通过(无重复 item_id)后再动数据:先回收 arbitration 态,使后面收窄枚举时不残留非法值。
UPDATE submissions
SET status = 'human_reviewing'
WHERE status = 'needs_arbitration';

UPDATE task_items
SET status = 'claimed'
WHERE status = 'needs_arbitration';

-- 交换唯一键:守卫已保证 item_id 不重复,恢复单列唯一是安全的。
ALTER TABLE submissions
  DROP INDEX uk_item_labeler,
  ADD UNIQUE KEY uk_item (item_id);

-- 收窄枚举,移除 needs_arbitration。
ALTER TABLE submissions
  MODIFY status ENUM('draft', 'submitted', 'ai_reviewing', 'human_reviewing', 'revising', 'approved', 'rejected') NOT NULL DEFAULT 'draft';

ALTER TABLE task_items
  MODIFY status ENUM('available', 'claimed', 'finished') NOT NULL DEFAULT 'available';

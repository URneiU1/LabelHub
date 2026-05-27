-- 回滚 008(随 .up.sql 一起移动到 apps/api/internal/migration/ 后才生效)。
-- ⚠️ MySQL 收缩 ENUM 前必须确保没有行用到将被删除的 'md',否则该列会被截断/报错:
--   DELETE FROM exports WHERE format = 'md';   -- 或先把这些行迁到别的格式
-- 确认无 md 行后再执行下面的 MODIFY,把枚举还原到 001 的定义。

ALTER TABLE exports
  MODIFY format ENUM('json','jsonl','csv','xlsx') NOT NULL;

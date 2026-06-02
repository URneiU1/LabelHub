-- 回滚:收缩 ENUM 前必须确保没有行用到 'md',否则该列会被截断/报错:
--   DELETE FROM exports WHERE format = 'md';   -- 或先把这些行迁到别的格式
ALTER TABLE exports
  MODIFY format ENUM('json','jsonl','csv','xlsx') NOT NULL;

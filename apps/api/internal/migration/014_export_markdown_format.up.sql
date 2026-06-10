-- 把 exports.format 枚举加上 'md',让 pkg/exporter 的 Markdown 编码器能落库一条 md 导出记录。
ALTER TABLE exports
  MODIFY format ENUM('json','jsonl','csv','xlsx','md') NOT NULL;

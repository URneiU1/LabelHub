-- ⚠️ 暂存占位:本文件 **不在** apps/api/internal/migration/ 下,启动时不会被 golang-migrate 自动 apply。
-- 仅当开始做 S4 §Bonus A(Markdown 导出器)时,把本文件与同名 .down.sql 一起
--   `git mv docs/bonus-a-migration/008_export_markdown_format.*.sql apps/api/internal/migration/`
-- 然后 `make api` 会自动 Up。移动前确认 migration/ 下最新版本是 007(本文件是 008);
-- 若届时已有别的 008,改用下一个空号并同步改 .down.sql 文件名。
--
-- 作用:把 exports.format 枚举加上 'md',让 pkg/exporter 的 EncodeMarkdown 能落库一条 md 导出记录。

ALTER TABLE exports
  MODIFY format ENUM('json','jsonl','csv','xlsx','md') NOT NULL;

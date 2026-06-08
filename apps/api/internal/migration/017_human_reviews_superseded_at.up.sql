-- 017: human_reviews.superseded_at —— 数据验收打回的独立重审支持。
-- Owner 验收不通过把已通过提交打回人工复审(approved -> human_reviewing)时,作废该提交当前 revision
-- 的历史人工审核记录(标记 superseded_at)。review 服务的 approve 计数过滤 superseded_at IS NULL,
-- 使打回项必须重新走完整的独立初审 + 终审,而非凭历史 approve 计数被一次复确认即终结。
-- 审计历史仍完整保留在 audit_logs,不删除任何 human_reviews 行。
ALTER TABLE human_reviews ADD COLUMN superseded_at TIMESTAMP NULL DEFAULT NULL;

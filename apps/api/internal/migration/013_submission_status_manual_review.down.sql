-- 回滚:还原到 011 的枚举(去掉 manual_review)。收缩 ENUM 前必须确保无行使用该值,否则会被截断/报错:
--   UPDATE submissions SET status = 'human_reviewing' WHERE status = 'manual_review';
ALTER TABLE submissions
  MODIFY status ENUM('draft', 'submitted', 'ai_reviewing', 'human_reviewing', 'revising', 'approved', 'rejected', 'needs_arbitration', 'consensus_evidence') NOT NULL DEFAULT 'draft';

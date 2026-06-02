-- 把 submissions.status 枚举加上 manual_review(AI 可疑→转人工复核 的专属初审入口态)。
-- 否则 worker 写入该状态在 STRICT_TRANS_TABLES 下直接报错、非严格模式下被静默截断为空串。
ALTER TABLE submissions
  MODIFY status ENUM('draft', 'submitted', 'ai_reviewing', 'human_reviewing', 'manual_review', 'revising', 'approved', 'rejected', 'needs_arbitration', 'consensus_evidence') NOT NULL DEFAULT 'draft';

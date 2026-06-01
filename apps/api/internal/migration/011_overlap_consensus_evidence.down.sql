UPDATE submissions
SET status = 'submitted'
WHERE status = 'consensus_evidence';

ALTER TABLE submissions
  MODIFY status ENUM('draft', 'submitted', 'ai_reviewing', 'human_reviewing', 'revising', 'approved', 'rejected', 'needs_arbitration') NOT NULL DEFAULT 'draft';

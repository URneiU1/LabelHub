ALTER TABLE submissions
  MODIFY status ENUM('draft', 'submitted', 'ai_reviewing', 'human_reviewing', 'revising', 'approved', 'rejected', 'needs_arbitration', 'consensus_evidence') NOT NULL DEFAULT 'draft';

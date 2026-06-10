ALTER TABLE submissions
  DROP INDEX uk_item,
  ADD UNIQUE KEY uk_item_labeler (item_id, labeler_id);

ALTER TABLE task_items
  MODIFY status ENUM('available', 'claimed', 'finished', 'needs_arbitration') NOT NULL DEFAULT 'available';

ALTER TABLE submissions
  MODIFY status ENUM('draft', 'submitted', 'ai_reviewing', 'human_reviewing', 'revising', 'approved', 'rejected', 'needs_arbitration') NOT NULL DEFAULT 'draft';

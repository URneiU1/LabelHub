ALTER TABLE submissions
  ADD UNIQUE KEY uk_item (item_id);

UPDATE submissions
SET status = 'human_reviewing'
WHERE status = 'needs_arbitration';

UPDATE task_items
SET status = 'claimed'
WHERE status = 'needs_arbitration';

ALTER TABLE submissions
  DROP INDEX uk_item_labeler;

ALTER TABLE submissions
  MODIFY status ENUM('draft', 'submitted', 'ai_reviewing', 'human_reviewing', 'revising', 'approved', 'rejected') NOT NULL DEFAULT 'draft';

ALTER TABLE task_items
  MODIFY status ENUM('available', 'claimed', 'finished') NOT NULL DEFAULT 'available';

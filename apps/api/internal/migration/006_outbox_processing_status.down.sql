UPDATE outbox_events
SET status = 'pending', published_at = NULL
WHERE status = 'processing';

ALTER TABLE outbox_events
  MODIFY status ENUM('pending','published','failed') NOT NULL DEFAULT 'pending';

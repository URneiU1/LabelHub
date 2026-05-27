ALTER TABLE outbox_events
  MODIFY status ENUM('pending','processing','published','failed') NOT NULL DEFAULT 'pending';

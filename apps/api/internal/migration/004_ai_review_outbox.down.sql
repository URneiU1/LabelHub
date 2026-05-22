UPDATE outbox_events
SET topic = 'ai.review.requested'
WHERE topic = 'ai:review';

UPDATE ai_reviews
SET verdict = 'uncertain'
WHERE verdict IS NULL;

ALTER TABLE ai_reviews
  MODIFY verdict ENUM('pass','reject','uncertain') NOT NULL;

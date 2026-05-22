ALTER TABLE ai_reviews
  MODIFY verdict ENUM('pass','reject','uncertain') NULL;

UPDATE outbox_events
SET topic = 'ai:review'
WHERE topic = 'ai.review.requested';

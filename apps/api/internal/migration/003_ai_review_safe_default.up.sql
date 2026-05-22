SET NAMES utf8mb4;

ALTER TABLE tasks
  MODIFY ai_review_enabled TINYINT(1) NOT NULL DEFAULT 0;

UPDATE tasks t
LEFT JOIN ai_prompt_configs cfg ON cfg.id = t.ai_prompt_id
SET t.ai_review_enabled = 0
WHERE t.ai_review_enabled = 1
  AND cfg.id IS NULL;

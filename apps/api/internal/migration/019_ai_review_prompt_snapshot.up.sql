ALTER TABLE ai_reviews
  ADD COLUMN prompt_snapshot MEDIUMTEXT NULL COMMENT '渲染后发给 LLM 的完整消息(system+user)JSON,即 AI 实际看到的 prompt' AFTER raw_response,
  ADD COLUMN prompt_hash CHAR(64) NULL COMMENT 'prompt_snapshot 的 sha256 指纹' AFTER prompt_snapshot;

-- refresh_tokens:持久化已签发的 refresh token(按 jti),用于服务端撤销。
-- logout 撤销用户全部活跃 token;refresh 校验 jti 未撤销/未过期并做轮换(撤销旧、签发新)。
CREATE TABLE refresh_tokens (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  jti CHAR(36) NOT NULL,
  user_id BIGINT NOT NULL,
  expires_at TIMESTAMP NOT NULL,
  revoked_at TIMESTAMP NULL DEFAULT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  UNIQUE KEY uk_refresh_jti (jti),
  KEY idx_refresh_user_active (user_id, revoked_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

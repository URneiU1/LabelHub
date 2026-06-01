# HTTPS 与公网演示加固（H-06）

## 问题

当前演示曾以**明文 HTTP**（IP 直连、`CADDY_SITE_ADDRESS=:80`）对外。这条形态下：

- 登录账号/密码、存于浏览器 `localStorage` 的 access token、全部 API 流量，以及 `/asynqmon/`
  的 Basic Auth 凭据都明文传输，处在共享/不可信网络时可被旁路监听与重放。
- 浏览器只在 HTTPS 上才认 HSTS，明文 HTTP 下即使下发 HSTS 头也被忽略。

因此**不要把 `:80` 明文直接暴露公网做正式演示**。`:80` 仅用于本地 `docker compose` 冒烟。

## 启用 HTTPS（有域名时）

基建已就绪，启用只需配置，无需改代码：

1. 准备一个域名（如 `labelhub.example.com`），把 A / AAAA 记录指向演示主机公网 IP。
2. 在 `deploy/.env`（由 `.env.example` 拷贝）里：
   - `CADDY_SITE_ADDRESS=labelhub.example.com`（**真实域名，不是 `:80`**）。
   - `API_CORS_ORIGINS=https://labelhub.example.com`。
3. 确认主机 `HTTP_PORT=80`、`HTTPS_PORT=443` 可入站（Let's Encrypt HTTP-01 校验需要 80）。
4. `docker compose -f deploy/docker-compose.prod.yml up -d`。

配成真实域名后 Caddy 会：

- 自动签发并续期 Let's Encrypt 证书；
- 把 HTTP 自动 301/308 跳转到 HTTPS；
- 下发 `Strict-Transport-Security: max-age=31536000; includeSubDomains`（见 `deploy/Caddyfile`）。
  暂不加 `preload`——那是难以撤销的强承诺，待域名稳定后再单独评估并提交 preload 列表。

## 验证清单

- `curl -sI http://labelhub.example.com/` 返回 301/308 跳转到 `https://`。
- 浏览器全程 TLS 打开站点与 `/asynqmon/`，证书有效、无混合内容告警。
- 响应头含 `Strict-Transport-Security`。
- 登录后在 DevTools Network 里确认请求都走 `https://`。

## 切到 HTTPS 后必须轮换的凭据

凭据曾在明文链路上使用过，切换公网形态时应一并轮换（改 `deploy/.env` 后重启相关容器）：

- [ ] `JWT_SECRET`（轮换即让此前签发的 access/refresh token 全部失效）
- [ ] `MYSQL_ROOT_PASSWORD` / `MYSQL_PASSWORD`
- [ ] `REDIS_PASSWORD`（同步改 redis `--requirepass` 与 api/worker/asynqmon 客户端）
- [ ] `ASYNQMON_USER` / `ASYNQMON_PASSWORD_HASH`
      （`docker run --rm caddy:2-alpine caddy hash-password --plaintext '<新密码>'` 生成 bcrypt）
- [ ] `EXPORT_DOWNLOAD_SECRET`（导出下载签名密钥）
- [ ] 演示账号密码（seed 的 `123456` 仅供本地/受控演示；公网演示应改强密码）

## 现状

本轮按"先记录 + 备好配置"处理：`Caddyfile` 已补 HSTS 头、`.env.example` 已说明域名→TLS 的切换方式，
**未实际上线**（待提供域名）。届时按上面步骤切换并完成凭据轮换即可。

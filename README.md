# LabelHub · 数据标注平台

字节 AI 全栈挑战赛课题 — 覆盖「数据生产 → AI 预审 → 人工审核 → 多格式导出」全生命周期的 Web 数据标注平台。

## 快速启动

```bash
# 1. 起基础设施(MySQL + Redis + Adminer)
make up

# 2. 安装依赖
make install

# 3. 初始化官方 qa_quality seed
make seed

# 4. 跑 API(终端 A,复用或新开)
make api                 # http://localhost:8080/health

# 5. 跑前端(终端 B,新开)
make web                 # http://localhost:5173

# MySQL(host):        localhost:13306  (user=labelhub, pass=labelhub_dev)
# Adminer(DB GUI):    http://localhost:18080  (server=mysql, user=labelhub, pass=labelhub_dev)
```

如果本机 8080 被占用,可用 `API_PORT=18080 make api` 启 API,再用 `VITE_API_PROXY_TARGET=http://localhost:18080 make web` 启前端。

## 架构

```
apps/web      — React 18 + TypeScript + Semi Design (单一 SPA,角色路由)
apps/api      — Go + Gin REST API (port 8080)
apps/ai-worker — Go Asynq AI 预审 Worker
```

详见 `docs/PLAN.md`。

## 开发记录

每次完成代码改动后,需要同步更新本节,并按实际情况维护 `CLAUDE.md` / `CODEX.md`:

- 本次完成: 写清楚做了哪些功能、修复或重构。
- 仍需提升: 写清楚当前已知风险、体验问题、技术债或 warning。
- 下一步: 写清楚下一轮最应该推进的任务。
- 验证记录: 写清楚已跑过的测试、lint、build 或手动验证。

### 最近完成

- 2026-05-23 `s3-ai-same-task-guard`: 修复 Owner Dashboard 同一 task 重复点击会让 in-flight action guard 失效的问题:当前已选 task 再点击直接 no-op,只在实际切换不同 task 时递增 action generation;补 deferred dry-run 回归,确认同 task 晚到 response 仍正常应用且 loading 复位。
- 2026-05-23 `s3-ai-review-followups`: 完成 S3 AI review P2 cleanup:Owner Dashboard 的 save prompt / AI review settings / dry-run 都加 task/action guard,切换 task 后晚到的 action response/error 不再污染当前任务 UI;OpenAI-compatible provider HTTP 400/401 error body 不进入 error message;LLM schema/threshold validation error 标记为 non-retryable,worker 遇到后直接 failover 到人工审核避免重复扣费;补 submit invalid active prompt 和 disallowed active model 的 422 HTTP 回归。
- 2026-05-23 `s3-ai-p1-hardening`: 修复 S3 AI review P1 阻塞项:worker 校验 payload 与 idempotency key 一致,所有 claim/finalize/failover 更新绑定 submission/revision/prompt version,非最终失败回写 failed 以便 Asynq 重试;submission submit 事务内锁定并重载 task 后再决定 AI plan,enabled 但 active prompt 缺失/跨 task/模型不允许时明确拒绝提交;LLM verdict 与 score thresholds 不一致会被拒绝;Owner Dashboard 忽略过期 prompt 响应,加载中/失败时禁用保存、启停和 dry-run,失败 dry-run 清空旧结果;provider HTTP error body 不再写入 dry-run/worker error 持久化路径。
- 2026-05-23 `s3-ai-prompt-form-reset`: 修复 Owner Dashboard 跨任务 AI Prompt 表单残留:切换到无 prompt 的任务或 prompt 加载失败时,表单会重置为默认 prompt_template/dimensions/thresholds/空 model,避免把上一任务 prompt 误保存到当前任务。
- 2026-05-23 `s3-ai-review-controls`: 完成任务级 AI review 启用/关闭控制:新增 owner/admin `POST /tasks/:taskId/ai-review-settings`,复用 `loadOwnedTask` 权限边界,启用前校验 active `ai_prompt_id` 且 prompt 属于当前 task,关闭时只更新 `tasks.ai_review_enabled=false` 并保留 prompt history;Owner Dashboard 在 AI Prompt 区块展示当前状态,无 active prompt 时禁用启用动作,有 prompt 时可启用或关闭并本地更新状态;submission flow 回归确认 `ai_review_enabled=false` 即使有 prompt 也跳过 AI。
- 2026-05-23 `s3-ai-review-fixes`: 修复 S3 AI Prompt review 发现的问题:Owner 新 prompt 默认 model 不再硬编码 `mock-model`,由后端按 `LLM_MODEL`/fallback 决定并受 `LLM_ALLOWED_MODELS` 校验;Owner UI 改用 dimensions JSON textarea,重保存保留 `description`/`weight`;AI worker 成功重试写 `succeeded` 时清空旧 `error_msg`。
- 2026-05-23 `s3-ai-product-layer-part1`: 完成 S3 AI 产品层第一段:新增共享 `llmreview` OpenAI-compatible provider,支持 `LLM_PROVIDER/LLM_BASE_URL/LLM_API_KEY/LLM_MODEL/LLM_ALLOWED_MODELS/LLM_TIMEOUT_MS`;worker 从 prompt version 读取 payload/answer/baseline,通过 Function Calling strict schema 取得结构化 verdict/score/dimensions/reason,Go 端严格校验并记录 tokens/latency/raw_response,未配置时保留 deterministic/mock fallback;Owner API 增加 `/tasks/:taskId/ai-prompts` GET/POST 和 `/tasks/:taskId/ai-prompts/:promptId/dry-run`,按 owner/admin + task ownership 校验,POST append-only 创建 version 并同事务更新 `tasks.ai_prompt_id`;Owner 页面增加 AI Prompt 编辑和 dry-run 结构化结果展示。
- 2026-05-23 `p1-ai-edge-hardening`: 修复 P1/P2 边界复查问题:FileUpload 支持打回修改后复用同 submission 历史 revision 已 attached 的文件;AI worker duplicate replay 在 `markRunning` 抢不到 pending/failed 行时 no-op,且 failover 不再污染 succeeded 记录;API 侧新增 AI review sweeper,对超时停留在 `ai_reviewing` 的 pending/running 预审强制转 `human_reviewing` 并写 `ai_fail_max` audit。
- 2026-05-23 `p1-p2-closeout`: 一次性补齐 P1/P2 审查项:FileUpload 在 submit 事务内把答案引用的 `storageKey` 校验并绑定到 `submission_revision_id`,非法 owner/cross-task/重复 key 有测试;前后端 schema 校验对齐 trim 后 name、严格 options、FileUpload `maxFiles`、LLMTrigger `target_field`;Labeler/Reviewer 增加页面级 schema runtime 回归;AI submit 事务创建 pending `ai_reviews` + `ai:review` outbox,API 后台 publisher 投递 Asynq,worker 幂等消费并在成功/失败后流转到 `human_reviewing`。
- 2026-05-22 `p1-safety-defaults`: 补 P1 安全口径:Save/Submit 在 `submission.Save` 事务内用 `FOR UPDATE` 锁住 `task_items` 并二次校验 `claimed_by/status`;并将 `ai_review_enabled` 的初始 schema、GORM 默认值和 003 迁移统一改为安全默认关闭,旧库中未配置 AI prompt 的任务会被迁移为关闭 AI。
- 2026-05-22 `phase2-schema-runtime`: 完成阶段二动态模板运行时闭环:Labeler Plaza 和 Reviewer Queue 改为读取 `bundle.template.schemaJson` 并通过 `SchemaRenderer` 渲染;Reviewer 详情改走 `/reviewer/submissions/:submissionId`;补齐 RichText/JSONEditor/FileUpload/LLMTrigger 真实可交互 widget;前后端 parser/canonical 保存保留 `export_fields` 与 `x-*` 扩展;Vite 对 React/Semi/lottie 做分包并将 lottie 指到 light player,build 不再出现 eval/chunk warning。
- 2026-05-22 `fix(api): harden review and claim transactions`: 完成阶段一 P0 后端并发与越权重构:新增 `task_reviewers` 显式审核授权表;Reviewer Queue/Detail/Review 统一按 owner/admin/task_reviewers 做资源隔离;`review.Apply` 在事务内锁 task/submission 并校验 `human_reviewing`;Claim 在事务内锁 task、抢 item、立即创建 draft submission 并冻结 `template_version`;Save/Submit 在 submission 行锁保护下生成 revision_no。
- 2026-05-22 `0b871d5`: 修复安全和边界问题,包括 ShowItem URL 白名单、reviewer/owner 审核资源边界、upload task 权限与 MIME/content 校验、template/answer body 限制、CORS 显式 origin、submission template_version snapshot。
- 2026-05-22 `92f08ac`: 完成 S2 Day2 SchemaRenderer runtime,支持官方 schema 解析、核心 widget 渲染、答案校验和 renderer 单测。

### 仍需提升

- AI 预审 P1 安全/状态/前端竞态问题已收敛,并补了 P2 action stale guard/provider error/non-retryable validation cleanup,但 golden sample 管理、Reviewer AI verdict/score 展示还未做。
- FileUpload 已完成 temp→attached 绑定和打回复用,但下载/预览授权接口与 temp orphan cleanup 定时清理还未做。
- `task_reviewers` 目前通过 seed 赋予官方任务的 `reviewer1` 权限,Owner 后台的审核员分配 UI/API 还未实现。

### 下一步

- 推进 S2 后续:进入 Designer 的 append/delete/简易属性编辑能力。
- 推进 S3 下一段前先补 P2 golden sample persistence shape,再做 golden sample 管理 UI 和 Reviewer AI verdict/score 展示。
- 补 FileUpload 下载/预览授权与 orphan cleanup。

### 验证记录

- 2026-05-23 S3 AI same-task guard: `pnpm -F web test` 通过;`pnpm -F web lint` 通过;`pnpm -F web build` 通过。
- 2026-05-23 S3 AI review followups: `cd apps/api && go test -count=1 ./...` 通过;`cd apps/ai-worker && go test -count=1 ./...` 通过;`pnpm -F web test` 通过;`pnpm -F web lint` 通过;`pnpm -F web build` 通过。
- 2026-05-23 S3 AI P1 hardening: `cd apps/api && go test -count=1 ./...` 通过;`cd apps/ai-worker && go test -count=1 ./...` 通过;`pnpm -F web test` 通过;`pnpm -F web lint` 通过;`pnpm -F web build` 通过。
- 2026-05-23 S3 AI prompt form reset: `pnpm -F web test` 通过;`pnpm -F web lint` 通过;`pnpm -F web build` 通过。
- 2026-05-23 S3 AI review controls: `cd apps/api && go test -count=1 ./...` 通过;`cd apps/ai-worker && go test -count=1 ./...` 通过;`pnpm -F web test` 通过;`pnpm -F web lint` 通过;`pnpm -F web build` 通过。
- 2026-05-23 S3 AI review fixes: `cd apps/api && go test -count=1 ./...` 通过;`cd apps/ai-worker && go test -count=1 ./...` 通过;`pnpm -F web test` 通过;`pnpm -F web lint` 通过;`pnpm -F web build` 通过。
- 2026-05-23 S3 AI product layer part1: `cd apps/api && go test -count=1 ./...` 通过;`cd apps/ai-worker && go test -count=1 ./...` 通过;`pnpm -F web test` 通过;`pnpm -F web lint` 通过;`pnpm -F web build` 通过。
- 2026-05-23 P1/P2 edge hardening: `cd apps/api && go test -count=1 ./...` 通过;`cd apps/ai-worker && go test -count=1 ./...` 通过;`pnpm -F web test` 通过;`pnpm -F web lint` 通过;`pnpm -F web build` 通过。
- 2026-05-23 P1/P2 closeout: `cd apps/api && go test -count=1 ./...` 通过;`cd apps/ai-worker && go test -count=1 ./...` 通过;`pnpm -F web test` 通过;`pnpm -F web lint` 通过;`pnpm -F web build` 通过。
- 2026-05-22 P1: `cd apps/api && go test ./...` 通过;`make seed` 通过;本地 MySQL 确认 `tasks.ai_review_enabled` 默认值为 `0`,官方 seed 任务 AI 为关闭。
- 2026-05-22 阶段二: `cd apps/api && go test ./...` 通过;`pnpm -F web test` 通过;`pnpm -F web lint` 通过;`pnpm -F web build` 通过且无 lottie eval/chunk size warning。
- 2026-05-22 阶段一: `go test ./...` 通过;`make seed` 通过并确认本地 MySQL `task_reviewers` 中官方任务已分配 `reviewer1`。

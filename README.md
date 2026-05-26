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

- 2026-05-25 `s2-designer-drag-ordering`: Template Designer 增加字段拖拽排序体验:latest 模板的 canvas field 提供 Drag handle,拖拽只按 `_draftId` 重排现有 `fields` 数组,保留选中字段和 Copy/Up/Down 键盘 fallback;历史只读、task mismatch、schema error 等不可编辑状态会禁用拖拽,保存 payload 的 `fields/export_fields` 顺序跟随画布且继续剥离 `_draftId`。
- 2026-05-25 `s2-designer-real-item-preview`: Template Designer 接入真实 task item payload 预览:新增 owner/admin `GET /tasks/:taskId/item-preview` 窄接口,只返回当前 task 第一条 available item 的 `id/externalId/payload` JSON value,不暴露 claimed/status/submission/revision;Designer 自动加载该 payload 并传给 widget preview,ShowItem 默认 path 修正为 `$payload`,切换 task 后晚到 preview response 不污染当前画布。
- 2026-05-25 `s2-designer-field-controls`: Template Designer 增加字段级 Copy/Up/Down 控制和逐字段 validation 提示:复制字段会生成唯一 name 并保持 `_draftId` 不入保存 payload;上/下移调整 `fields` 与 `export_fields` 顺序;重复 name、空 options、长度范围、LLM target 等错误会显示在对应 canvas field 和属性栏,Save 在校验错误时继续禁用。
- 2026-05-25 `s2-designer-route-race-hardening`: 加固 Template Designer route 异步一致性:快速切换 `/owner/tasks/:taskId/templates/:templateId` 时旧模板加载响应不再覆盖当前页面;invalid template id、404/network load failure、task/template mismatch 都会 fail closed 清空旧草稿并禁用编辑;save/fork 请求 resolve 后若 route 已变化,不会再 Toast 或 navigate 回旧模板。
- 2026-05-25 `s2-designer-task-mismatch-hardening`: 加固 Template Designer route/template 归属一致性:加载 `/templates/:templateId` 后校验返回的 `template.taskId` 必须等于 URL 中的 `taskId`;不一致时显示错误、清空草稿并禁止 save/fork,避免手改 URL 或未来跳转 bug 把模板保存到错误 task。
- 2026-05-25 `s3-owner-history-refresh-fix`: 修复 Owner 单条 golden sample dry-run 失败后 history/trend 不刷新的 P2:单样本失败路径现在与成功路径一样重新加载当前 task/filter 的 dry-run history,让失败记录和趋势摘要及时反映后端已持久化的 failed dry-run。
- 2026-05-25 `s2-designer-editing-v1`: 新增 Owner 模板版本入口和 Template Designer 最小编辑闭环:`/owner/tasks/:taskId/templates` 列出版本,`/owner/tasks/:taskId/templates/:templateId` 加载模板;latest 版本支持点击添加 9 个核心 widget、画布选中/删除、编辑 name/label/required 以及 options/length/path/mode/maxFiles/LLM target/prompt 等基础属性,保存时通过既有 `POST /tasks/:taskId/templates` 创建新版本并剥离 `_draftId`;历史版本只读并可 Fork 为新版本。
- 2026-05-25 `s3-provider-retry-backoff`: OpenAI-compatible provider 新增短重试/backoff 配置:支持 `LLM_RETRY_MAX_ATTEMPTS`、`LLM_RETRY_BACKOFF_MS`、`LLM_RETRY_MAX_BACKOFF_MS`,默认最多 2 次且上限 2s;仅网络错误、HTTP 429 和 5xx 会重试,并尊重/clamp `Retry-After`;HTTP 400/401、schema/threshold validation 仍 fail fast,provider error body 继续不进入持久化错误信息。
- 2026-05-25 `s3-reviewer-ai-verdict-display`: Reviewer Queue/Detail 增加 AI 预审结果展示,复用 submission 上已有 `aiVerdict/aiScore`,左侧队列显示 `AI verdict · score`,详情页在人工审核前显示 AI verdict/score 或空状态;未改后端 API。
- 2026-05-25 `s3-owner-dry-run-history-ui`: Owner Dashboard 的 Golden Samples 区域新增 dry-run history/trend 最小可用视图,复用 `GET /tasks/:taskId/ai-dry-runs?golden_sample_id=&limit=` 自动加载最近记录,支持按 sample filter/History 按钮查看单样本历史,用 compact summary 展示 total/matched/mismatch/failed,table 展示 expected/actual verdict、matched、status/error、prompt version 和 finished time,并沿用 task switch stale guard 防止晚到 history 响应污染当前 task。
- 2026-05-25 `s3-golden-sample-batch-dry-run`: 新增 owner/admin batch golden sample dry-run endpoint:`POST /tasks/:taskId/golden-samples/dry-runs`,body 为 `{sample_ids:[...], ai_prompt_id?: number}`;服务端限制单批最多 20 个样本并串行执行,避免 provider 并发突刺;请求中的 sample 必须全部属于当前 task,否则整体 404;单样本 provider/evaluator/config 失败写入对应 failed dry-run 或返回 per-sample failed result,不阻断后续样本;响应返回 `summary` 与每个 sample 的 status/dryRunId/result/matchedExpected/error。
- 2026-05-25 `s3-owner-batch-run-wiring`: Owner Dashboard 的 Run all visible samples 改接后端 batch dry-run endpoint,一次请求返回 partial results 并映射到 result table;保留单样本 Run endpoint;后端 batch 循环新增 `LLM_BATCH_DRY_RUN_DELAY_MS` 可选样本间隔,默认 0,用于按 provider 限制做基础节流。
- 2026-05-25 `s3-ai-dry-run-history-api`: 新增 owner/admin task-scoped dry-run history list API:`GET /tasks/:taskId/ai-dry-runs?golden_sample_id=&limit=`,复用 task ownership 边界,支持按 golden sample 过滤和 limit 上限,返回 dry-run 记录的 prompt version、payload/expected answer snapshot、expected/actual verdict、matched flag、status/result/error 和完成时间,其中 JSON snapshot/result 以 JSON value 返回而不是 escaped string。
- 2026-05-25 `s3-owner-dry-run-raw-json`: 修复 Owner Dashboard ad-hoc AI prompt dry-run 端到端 raw JSON 精度:前端不再 `JSON.parse` 后走 `apiPost`,改为校验后用 `apiPostRawJSON` 发送 textarea 原始 JSON body,配合后端 RawMessage/provider UseNumber 避免大整数在前端、handler 或 provider message 路径被 float64 精度坍塌;同时补 golden sample prompt choice reset 断言和 ad-hoc dry-run missing/null/invalid JSON 回归。
- 2026-05-25 `s3-golden-sample-draft-reset`: 修复 Owner Dashboard golden sample create draft 跨 task 泄漏:切换到不同 task 时重置 payload/expected_answer/expected_verdict/notes/prompt choice,同 task 重复点击仍 no-op;同时将 ad-hoc AI prompt dry-run 的 payload/answer 改为 raw JSON 传递,避免大整数在 handler/provider message 路径被 float64 精度坍塌。
- 2026-05-25 `s3-golden-sample-owner-ui`: 修复 golden sample JSON contract:创建接口用 raw JSON/canonical hash 保留大整数精度,GET/POST 响应用 JSON value 而不是 escaped string;Owner Dashboard 新增 Golden Samples 管理区,支持加载/创建/删除样本、按 active/指定 prompt 绑定、单样本 dry-run、Run all visible samples 和 result table,并沿用 task/action stale guard 防止跨 task 晚到响应污染 UI。
- 2026-05-23 `s3-golden-sample-dry-run-linking`: 新增 golden sample 单样本 dry-run linking: additive migration 扩展 `ai_dry_runs` 记录 `golden_sample_id`、prompt version、输入快照、expected/actual verdict、matched flag 和完成时间,并从 `ai_prompt_configs` 回填历史 prompt version;新增 `POST /tasks/:taskId/golden-samples/:sampleId/dry-run`,复用 owner/admin ownership、sample pinned prompt/task active prompt、prompt allowlist、mock/OpenAI-compatible provider 和 threshold consistency,成功/失败均写 dry-run 记录且 provider failure 只返回/持久化安全错误信息。批量 dry-run 暂缓,避免在结果表 UI 前先固化 partial failure 和路由语义。
- 2026-05-23 `s3-golden-sample-api`: 新增 owner/admin golden sample persistence API:`GET/POST/DELETE /tasks/:taskId/golden-samples`,复用 task ownership 边界;创建时校验 payload/expected_answer/expected_verdict,可选 `ai_prompt_id` 必须属于当前 task,对 canonical payload JSON 计算 sha256 并用 `uk_task_payload_hash` 做同 task 去重,重复 payload 返回 409。
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

- S2 Designer 已有模板版本列表、latest 编辑、历史只读/Fork、append/delete/copy/Up-Down/drag order/simple property editing、逐字段 validation、真实 item payload 预览和 Save as new version,但还没有 Tabs/Group 加分物料。
- AI 预审 P1 安全/状态/前端竞态问题已收敛,并补了 P2 action stale guard/provider error/non-retryable validation cleanup;golden sample 后端 persistence API、Owner 管理 UI、batch result table、task-scoped dry-run history API、Owner history/trend 最小视图、Reviewer AI verdict/score 展示、同步串行 batch dry-run endpoint、基础 batch delay 配置和 provider 429/backoff 短重试已具备,但 server-side quota/circuit breaker 与更丰富的 trend/history 分析还未做。
- FileUpload 已完成 temp→attached 绑定和打回复用,但下载/预览授权接口与 temp orphan cleanup 定时清理还未做。
- `task_reviewers` 目前通过 seed 赋予官方任务的 `reviewer1` 权限,Owner 后台的审核员分配 UI/API 还未实现。

### 下一步

- 推进 S2 后续:补 Designer Tabs/Group 加分物料或更完整的布局编辑。
- 推进 S3 下一段:补 server-side dry-run quota/circuit breaker 或更丰富的 Owner dry-run trend/history 分析。
- 补 FileUpload 下载/预览授权与 orphan cleanup。

### 验证记录

- 2026-05-25 S2 Designer drag ordering: targeted `pnpm -F web test -- Designer.integration.test.tsx` 通过;full `pnpm -F web test` 通过;`pnpm -F web lint` 通过;`pnpm -F web build` 通过。未改后端。
- 2026-05-25 S2 Designer task mismatch hardening: targeted `pnpm -F web test -- Designer` 通过;`pnpm -F web build` 通过;`pnpm -F web lint` 通过。
- 2026-05-25 S3 Owner history refresh fix: targeted `pnpm -F web test -- Dashboard` 通过;`pnpm -F web lint` 通过;`pnpm -F web build` 通过。
- 2026-05-25 S2 Designer editing v1: targeted `pnpm -F web test -- src/modules/template/Designer.integration.test.tsx` 通过;full `pnpm -F web test` 通过;`pnpm -F web lint` 通过;`pnpm -F web build` 通过。未改后端。
- 2026-05-25 S3 provider retry/backoff: targeted `cd apps/api && go test -count=1 ./internal/service/aiprompt` 通过;full `cd apps/api && go test -count=1 ./...` 通过;extra `cd apps/ai-worker && go test -count=1 ./...` 通过。
- 2026-05-25 S3 Reviewer AI verdict display: targeted `pnpm -F web test -- Queue` 通过;`pnpm -F web lint` 通过;`pnpm -F web build` 通过。未改后端。
- 2026-05-25 S3 Owner dry-run history UI: targeted `pnpm -F web test -- Dashboard` 通过;`pnpm -F web lint` 通过;`pnpm -F web build` 通过。未改后端。
- 2026-05-25 S3 Owner batch run wiring: targeted `cd apps/api && go test -count=1 ./internal/handler -run 'GoldenSampleBatchDryRun'` 通过;targeted `pnpm -F web test -- Dashboard` 通过;full `cd apps/api && go test -count=1 ./...` 通过;extra `cd apps/ai-worker && go test -count=1 ./...` 通过;`pnpm -F web test` 通过;`pnpm -F web lint` 通过;`pnpm -F web build` 通过。
- 2026-05-25 S3 golden sample batch dry-run: targeted `cd apps/api && go test -count=1 ./internal/handler -run 'GoldenSampleBatchDryRun|GoldenSampleDryRunRecordsMatchedExpected'` 通过;full `cd apps/api && go test -count=1 ./...` 通过;extra `cd apps/ai-worker && go test -count=1 ./...` 通过。未改前端,未跑 web test/lint/build。
- 2026-05-25 S3 AI dry-run history API: targeted `cd apps/api && go test -count=1 ./internal/handler -run 'AIDryRuns'` 通过;full `cd apps/api && go test -count=1 ./...` 通过;extra `cd apps/ai-worker && go test -count=1 ./...` 通过。未改前端,未跑 web test/lint/build。
- 2026-05-25 S3 Owner dry-run raw JSON: targeted `cd apps/api && go test -count=1 ./internal/handler -run 'AIPromptDryRun'` 通过;targeted `pnpm -F web test -- Dashboard` 通过;full `cd apps/api && go test -count=1 ./...` 通过;`cd apps/ai-worker && go test -count=1 ./...` 通过;`pnpm -F web test` 通过;`pnpm -F web lint` 通过;`pnpm -F web build` 通过。
- 2026-05-25 S3 golden sample draft reset: targeted `cd apps/api && go test -count=1 ./internal/handler -run 'AIPromptDryRun|GoldenSample'` 通过;targeted `cd pkg/llmreview && go test -count=1 ./...` 通过;targeted `pnpm -F web test -- Dashboard` 通过;full `cd apps/api && go test -count=1 ./...` 通过;extra `cd apps/ai-worker && go test -count=1 ./...` 通过;`pnpm -F web test` 通过;`pnpm -F web lint` 通过;`pnpm -F web build` 通过。
- 2026-05-25 S3 golden sample owner UI: `cd apps/api && go test -count=1 ./...` 通过;`pnpm -F web test` 通过;`pnpm -F web lint` 通过;`pnpm -F web build` 通过。
- 2026-05-23 S3 golden sample dry-run linking: `cd apps/api && go test -count=1 ./...` 通过;`pnpm -F web test` 通过;`pnpm -F web lint` 通过;`pnpm -F web build` 通过。
- 2026-05-23 S3 golden sample persistence API: `cd apps/api && go test -count=1 ./...` 通过;`pnpm -F web test` 通过;`pnpm -F web lint` 通过;`pnpm -F web build` 通过。
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

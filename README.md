# LabelHub · 数据标注平台

字节 AI 全栈挑战赛课题 — 覆盖「数据生产 → AI 预审 → 人工审核 → 多格式导出」全生命周期的 Web 数据标注平台。

## 快速启动

```bash
# 最短路径:起基础设施、安装依赖、seed,然后按提示开长跑进程
make dev

# 1. 起基础设施(MySQL + Redis + Adminer)
make up

# 2. 安装依赖
make install

# 3. 初始化官方 qa_quality seed
make seed

# 4. 跑 API(终端 A,复用或新开)
make api                 # http://localhost:8080/health

# 5. 跑 Worker(终端 B,新开)
make worker

# 6. 跑前端(终端 C,新开)
make web                 # http://localhost:5173

# MySQL(host):        localhost:13306  (user=labelhub, pass=labelhub_dev)
# Adminer(DB GUI):    http://localhost:18080  (server=mysql, user=labelhub, pass=labelhub_dev)
# Ops profile:        docker compose --profile ops -f deploy/docker-compose.yml up -d
# asynqmon(nginx):    http://localhost:18081  (dev basic auth labelhub / labelhub_dev)
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

- 2026-05-28 `s6-runtime-tabs`: S6 Day3 第一段完成: `SchemaRenderer` 的 Tabs 从“所有 tab panel 堆叠显示”改成真实 `tablist`/`tabpanel` 交互,默认显示第一 tab,点击切换后只渲染 active tab 字段,同时保持 flat answer object 中已填写答案不丢;补 `SchemaRenderer` 测试验证 tab 切换、隐藏字段、切回后 radio 选择保持;Designer round-trip 测试跟随新 Tabs 行为先切到 Tab 2 再断言子字段。
- 2026-05-28 `s6-dense-status-surfaces`: S6 Day2 第一段完成:Owner Dashboard、ExportPanel、Labeler Plaza、Reviewer Queue 开始复用共享 `StatusBadge`/`EmptyState`/`LoadingBlock`,导出历史、任务列表、AI review 开关、golden sample、dry-run history、标注空态和审核空态不再直接裸显示 raw status/loading 文本;相关测试断言更新为新中文状态/可访问 loading 状态。
- 2026-05-28 `s6-state-primitives`: S6 Day1 底座完成: `tokens.css` 从 Schematic/Inter/default-blue 收敛为 Editorial console tokens,移除 Google font 依赖,加入稳定 7 态 status 色、Semi 常用控件 CSS override、全局 focus-visible、skeleton/top-progress keyframes;新增共享 `StatusBadge`/`statusLabel`、`EmptyState`、`LoadingBlock`、`TopProgress` 和 `StatePrimitives` 测试,为后续 Owner/Labeler/Reviewer 页面统一状态、空态、加载态做准备。
- 2026-05-28 `s6-plan`: 新增 `docs/PLAN-S6-IMPL.md`,把 Sprint 6 拆成 Editorial token/shared state components、Owner/Labeler/Reviewer 高密度界面、Designer 响应式布局、Tabs/Group 真实交互、友好错误/A11y、Browser smoke 和 `docs/S6_ACCEPTANCE.md` 验收文档。计划明确将 dark mode、Framer Motion、完整移动端、TanStack Query/Formily/dnd-kit 迁移列为可砍项。
- 2026-05-28 `s5-deploy-docs`: S5 Day5 收尾完成:新增生产部署模板 `deploy/docker-compose.prod.yml`,覆盖 api/worker/web/mysql/redis/asynqmon/caddy;新增 api/worker 共用多阶段 Dockerfile、web 静态 Caddy 镜像和 SPA fallback;生产 compose 通过 env 显式注入密钥/LLM/JWT/导出配置,api/worker 共享绝对 `EXPORT_DIR` volume,api 上传目录单独持久化;新增 `deploy/Caddyfile`、`deploy/.env.example`、`.dockerignore`;`make dev` 现在可一键起基础设施、安装依赖、seed 并提示分别启动 api/worker/web;新增 `docs/ARCHITECTURE.md`、`docs/DEPLOY.md`、`docs/S5_ACCEPTANCE.md`。
- 2026-05-28 `s5-openapi-error-boundary`: S5 Day4/Day5 第一段完成:按计划 fallback 手写主流程 `docs/openapi.yaml`(auth/tasks/templates/labeler/reviewer/exports/stats),新增 `pnpm -F web gen:api` 用 `openapi-typescript` 生成 `apps/web/src/shared/api/schema.d.ts`;新增 `docs/LabelHub.postman_collection.json` 覆盖登录、领题、提交、审核、导出、stats 主流程;前端新增全站 `ErrorBoundary`,在 `main.tsx` 包住 `<App/>`,并补抛错 fallback/reset 测试。
- 2026-05-28 `s5-frontend-strict-roundtrip`: S5 Day3 前端质量补强完成:开启 `apps/web/tsconfig.app.json` 的 `strict:true` 且 `pnpm -F web build` 直接通过;Renderer 新增 ShowItem text/video/json 模式测试与 LLMTrigger 写回 `target_field` 测试;Designer 集成测试补保存 payload → `parseTemplateSchema` → `SchemaRenderer` 的 round-trip 断言,覆盖 Group/Tabs 子字段、导出字段和 Renderer 实际渲染一致性。
- 2026-05-28 `s5-integration-main-flow`: S5 Day2 新增 build-tag 隔离的 testcontainers 集成测试 `apps/api/internal/integration/main_flow_integration_test.go`:真 MySQL 8.0.36 + Redis 7 容器,真实迁移 SQL,seed owner/labeler/reviewer/task/prompt/item,覆盖 submit→ai_review outbox→Redis publish→AI 结果入库→review revise→labeler resubmit→AI pass→review approve→JSONL export 落盘的主链路。CI 新增独立 `integration` job 跑 `go test -tags=integration ./apps/api/internal/integration -count=1`,普通单测仍不跑 Docker。
- 2026-05-28 `s5-quality-day1`: 按 `docs/PLAN-S5-IMPL.md` 开始 S5 工程质量 sprint:新增并纳入 S5 执行计划;CI Go workspace 补跑 `./pkg/exporter`;状态机测试改为覆盖 `Transitions()` 暴露的全部合法边与每个状态的非法事件,`internal/statemachine` 覆盖率到 100%;`review.Apply` 补 approve/reject/revise/非法状态/并发写 RowsAffected=0 的事务测试,覆盖率从 13.0% 提升到 70.1%;outbox publisher 补 stale processing reset、claim lost update、enqueue 失败重试/死信、TaskID 冲突幂等成功测试,覆盖率从 29.1% 提升到 45.5%;AI worker 补 `handleAIReview` verdict→状态 table-driven 覆盖(pass 自动通过/pass 转人工/reject 转人工/uncertain 转人工)和熔断 open 直接 failover 到人工路径;修复 S4 review 遗留两处质量问题:导出行加载 DB 错误不再被写成 terminal failed,保持 asynq 可重试;StatsBoard 维度均分只统计 `ai_reviews.revision_id = submissions.current_revision_id`,避免旧 revision AI 结果污染看板。前端 StatsBoard retry 改 request sequence guard,切任务后晚到 retry 响应不再覆盖当前任务。
- 2026-05-28 `s4-review-fixes`: 修复 S4 code review(go/typescript/security 三审 + 人工核验)发现的 5 个 HIGH 与 2 个 MEDIUM:① api/worker 启动强制 `EXPORT_DIR` 绝对路径(相对路径在两进程不同 CWD 下会让签名下载静默 403),`.env.example` 改绝对示例;② `pkg/exporter` 新增 `ErrTerminal` 区分永久失败(编码/落盘,已写 `failed`)与瞬时失败(DB),worker 据此 `asynq.SkipRetry` 或重试,并对不可解析 payload 直接 SkipRetry;③ CSV 导出对 `= + - @ \t \r` 开头单元格加前导单引号,防 Excel/LibreOffice 公式注入;④ ExportPanel 历史加载 `catch` 补 stale guard,切任务后晚到失败不再误弹 Toast;⑤ StatsBoard 增加错误态 + 重试按钮,fetch 失败不再永久转圈;⑥ VChart 改 `React.lazy`,并在 `vite.config` 让 `@visactor` 脱离 eager vendor,首屏 eager vendor 从 2.2MB(gzip 616KB)降到 412KB(gzip 125KB),VChart 进按需加载的 StatsBoard chunk。顺带把 AI 饼图负值用 `Math.max(0, …)` 兜底。
- 2026-05-27 `s3-hardening-cleanup`: 收尾 S3 hardening 未提交修复:状态机补 `ai_auto_approved` transition 并纳入全表覆盖;`GET /uploads/:id` 复用 task 上传权限策略下载非 deleted 文件;outbox publisher 在事务内只取 pending event、事务外 enqueue,并用 deterministic Asynq `TaskID(outbox:<id>)` 防重复投递;AI worker 5xx 熔断默认调整为连续 20 次/5 分钟;API 主进程新增 hourly orphan temp upload cleaner,24h 以上未 attached 的 temp 记录会先条件标 deleted 再删除磁盘文件。
- 2026-05-27 `s3-review-finding-fixes`: 修复提交 diff review 的三处回归/边界问题:所有已有 submission 的答题 bundle 都固定按 `submission.template_version` 读取历史模板,避免待审/详情状态在 owner 改模板后误用当前 schema;Reviewer 规则面板改回只读查看 + 跳 Owner 编辑,移除 reviewer 侧直接启用 AI prompt 的 mutating route;ai-worker 的 golden dry-run 在 provider circuit open 时会先 claim run 再写 failed,避免 owner 轮询长期停在 queued。
- 2026-05-27 `s3-durable-dry-run-rule-ops`: 收尾 S3 demo 稳定性缺口:单条 golden sample dry-run 从 API 进程内 goroutine 升级为 durable outbox/asynq 链路,API 同事务写 `ai_dry_runs` + `outbox_events(ai:dry-run)`,ai-worker 消费后读取 prompt/sample snapshot、调用 provider、写 succeeded/failed 结果;Reviewer 规则配置可查看历史/当前 prompt 并跳转 Owner 编辑;Owner dry-run history API 返回 guard snapshot,Dashboard 展示 quota/circuit 状态、匹配率和均分。
- 2026-05-27 `s0-s3-plan-gap-polish`: 补齐第一批 S0-S3 计划缺口:新增 GitHub Actions CI,覆盖 Go workspace、web test/lint/build;deploy compose 增加 `ops` profile 下的 asynqmon + nginx basic auth;`/style-guide` 补齐 Button/Card/Form/Table/Modal/Tag 六类核心组件展示;Labeler Plaza 增加 3s debounce 自动保存草稿并带 request identity guard;schema parser/validator 和后端模板校验支持 `regex` 与 `requiredWhen`;AI worker 增加连续 provider HTTP 5xx 熔断,成功响应自动重置。
- 2026-05-26 `s3-demo-productization-closure`: 按获奖 demo 可见度补齐 S3 产品闭环:Owner Dashboard 可直接编辑并保存 `baseline_description`;AI Prompt dimensions 从 JSON 主路径升级为可增删改表单,threshold 增加 slider+number 控制;单条 golden sample dry-run 改为创建 queued run 后后台执行并由前端轮询 pending/running/succeeded/failed;AI worker 支持 `pass + human_review_enabled=false` 自动 approved 并写 `ai_auto_approved` audit;Labeler Plaza 增加“待修改”入口和上一轮打回意见;Reviewer Queue 增加多选批量 approve/revise 与上一轮意见展示,后端新增 `/reviews/batch`。
- 2026-05-26 `s3-reviewer-rule-config-entry`: Reviewer 的“规则配置”从静态提示升级为真实任务规则入口:新增 reviewer/owner/admin `GET /reviewer/tasks/:taskId/ai-prompts`,按 `task_reviewers`/owner/admin 边界列出该 task 的 AI Prompt versions、active prompt 和 AI review 开关;Reviewer Queue 右侧可打开规则面板、选择历史/当前规则版本查看 prompt/dimensions/threshold/model,并通过 `/owner?taskId=...&aiPromptId=...#ai-prompts` 跳转 Owner 编辑对应规则版本;Owner Dashboard 支持该深链初始选中 task 和 prompt version;前端补规则加载 stale guard,避免切换提交后晚到规则响应污染当前面板。
- 2026-05-26 `s3-reviewer-ai-detail-wiring`: Reviewer Detail 接入真实 AI 预审详情:后端 `/reviewer/submissions/:submissionId` 返回当前 submission/revision 的 latest `ai_reviews`、对应 prompt version 元数据和 submission audit logs;新增 `POST /reviewer/submissions/:submissionId/ai-review/retry`,仅允许 failed/dead AI review 重置为 pending 并重新写入 outbox;前端 Reviewer Queue 优先渲染真实 AI verdict/score/dimensions/reason/tokens/latency、Prompt 模板和处理日志,并把失败重跑按钮接到真实 API。
- 2026-05-26 `s2-designer-nested-visual-editing`: S2 Designer 把 Group/Tabs 子字段属性栏从 JSON textarea 升级为直接编辑控件:可改子字段 name/label/widget/required/options,可添加/删除/上下移动子字段,可增删 Tabs tab 并改 tab label;保存仍走原有递归 schema/export_fields 路径。修复嵌套编辑行用正在编辑的 child name 做 React key 导致输入中断、重复 name 校验误触发的问题。
- 2026-05-25 `s2-s3-structure-and-dryrun-guard`: S2 Designer/Renderer 补齐 Tabs/Group 加分物料最小闭环: schema parser 支持递归 `Group.fields` 与 `Tabs.tabs[].fields`,全局校验字段 name/LLM target;Renderer 递归渲染容器且答案继续保持 flat answer model;Designer 可添加 Group/Tabs,属性栏用 JSON 编辑子结构,保存时递归剥离 `_draftId` 并把 `export_fields` 展开为叶子字段顺序。S3 dry-run 增加 env-gated task-scoped quota/circuit breaker: `LLM_DRY_RUN_QUOTA_MAX_RUNS`、`LLM_DRY_RUN_CIRCUIT_MAX_FAILURES`、`LLM_DRY_RUN_GUARD_WINDOW_MINUTES`,对 ad-hoc prompt dry-run、单样本 golden dry-run、batch golden dry-run 超限返回 429 且不调用 provider。
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

- S5 工程质量 sprint 已完成主目标;生产 compose 目前完成 `config` 级校验,真实 `up --build` 留给有目标域名/端口和部署机资源的发布环境执行。
- S2 Designer 已有模板版本列表、latest 编辑、历史只读/Fork、append/delete/copy/Up-Down/drag order/simple property editing、逐字段 validation、`regex`/`requiredWhen` runtime 校验、真实 item payload 预览、Tabs/Group 最小结构物料、子字段属性栏可视化编辑和 Save as new version;但 Tabs/Group 子字段还不是画布内嵌套拖拽,layout 编辑也仍较基础。
- AI 预审 P1 安全/状态/前端竞态问题已收敛,并补了 P2 action stale guard/provider error/non-retryable validation cleanup;golden sample 后端 persistence API、Owner 管理 UI、batch result table、task-scoped dry-run history API、Owner history/trend/guard 视图、Reviewer AI verdict/score/detail/audit 展示、Reviewer failed/dead AI retry、Reviewer 规则查看/Owner 编辑跳转、单样本 durable queued dry-run + polling、同步串行 batch dry-run endpoint、基础 batch delay 配置、provider 429/backoff 短重试、server-side dry-run quota/circuit breaker、AI worker 5xx 熔断、AI 自动 approved、Labeler 3s 自动保存/修订入口和 Reviewer 批量操作已具备。若后续要继续产品化,主要是把 batch dry-run 也异步化、补更完整 trend 图表和真实运营告警。
- FileUpload 已完成 temp→attached 绑定、打回复用、下载授权接口和 temp orphan cleanup 定时清理;但前端文件预览/下载入口仍较基础,还需要 seeded browser smoke 覆盖。
- `task_reviewers` 目前通过 seed 赋予官方任务的 `reviewer1` 权限,Owner 后台的审核员分配 UI/API 还未实现。

### 下一步

- 继续 `docs/PLAN-S6-IMPL.md` Day3:做 Designer 响应式布局和 Group/Tabs 嵌套画布体验补强。
- 推进 S2 后续:补 Tabs/Group 子字段画布内嵌套拖拽和更完整 layout 编辑。
- 推进 S3 验收:做一次本地 seeded browser smoke,覆盖 Owner 配规则/跑 golden dry-run、Labeler 提交/修订、AI worker、Reviewer 批量审核和规则查看。
- 补 FileUpload 前端预览/下载入口的 seeded browser smoke。
- 若必须严格贴 PLAN.md 技术路线,再单独评估 Formily、TipTap、`packages/schema-spec` 和 `@dnd-kit/core` 迁移;当前实现优先保持既有自写 renderer/designer 稳定。

### 验证记录

- 2026-05-28 S6 runtime tabs: `pnpm -F web test -- SchemaRenderer Designer.integration` 通过(13 files,108 tests,仍有 jsdom canvas warning);`pnpm -F web lint` 通过;`pnpm -F web build` 通过(仍有既存 StatsBoard chunk >500KB warning);`git diff --check` 通过。
- 2026-05-28 S6 dense/status surfaces: `pnpm -F web test -- ExportPanel Plaza Queue Dashboard StatePrimitives` 通过;full `pnpm -F web test` 通过(13 files,108 tests,仍有 jsdom canvas warning);`pnpm -F web lint` 通过;`pnpm -F web build` 通过(仍有既存 StatsBoard chunk >500KB warning);`git diff --check` 通过。
- 2026-05-28 S6 state primitives: `pnpm -F web test -- StatePrimitives` 通过(13 files,108 tests,仍有 jsdom canvas warning);`pnpm -F web lint` 通过;`pnpm -F web build` 通过(仍有既存 StatsBoard chunk >500KB warning);`git diff --check` 通过。
- 2026-05-28 S6 plan: 文档-only 变更,未跑测试;前一提交的 S5 全量 Go/web/config 门禁已通过。
- 2026-05-28 S5 deploy/docs: `docker compose --env-file deploy/.env.example -f deploy/docker-compose.prod.yml config` 通过;`make -n dev` 通过;`go test ./apps/api/... ./apps/ai-worker/... ./pkg/exporter ./pkg/llmreview -count=1` 通过;`pnpm -F web test` 通过(12 files,103 tests,仍有 jsdom canvas warning);`pnpm -F web lint` 通过;`pnpm -F web build` 通过(仍有既存 StatsBoard chunk >500KB warning);`pnpm -F web gen:api` 通过;`jq empty docs/LabelHub.postman_collection.json` 通过;`git diff --check` 通过。生产 compose 未在本机执行 `up --build`,避免占用 80/443 和拉取/构建全部镜像。
- 2026-05-28 S5 openapi/error-boundary: `pnpm -F web gen:api` 通过并生成 `schema.d.ts`;`jq empty docs/LabelHub.postman_collection.json` 通过;`pnpm -F web test -- ErrorBoundary SchemaRenderer Designer.integration` 通过(12 files,103 tests);`pnpm -F web lint` 通过;`pnpm -F web build` 通过(仍有既存 StatsBoard chunk >500KB warning)。
- 2026-05-28 S5 frontend strict/roundtrip: `pnpm -F web build` 通过(`strict:true`,仍有既存 StatsBoard chunk >500KB warning);`pnpm -F web test -- SchemaRenderer Designer.integration` 通过(11 files,102 tests;含新增 Renderer/Designer round-trip);`pnpm -F web lint` 通过。
- 2026-05-28 S5 integration main-flow:普通 `go test ./apps/api/... ./apps/ai-worker/... ./pkg/exporter ./pkg/llmreview -count=1` 通过;本机 Colima 环境用 `DOCKER_HOST=unix:///Users/dadadineiyou/.colima/default/docker.sock TESTCONTAINERS_RYUK_DISABLED=true go test -tags=integration ./apps/api/internal/integration -count=1 -v` 通过(真 MySQL+Redis 容器,7.8s);`git diff --check` 通过。
- 2026-05-28 S5 quality day1 partial: targeted `go test -cover ./apps/api/internal/statemachine ./apps/api/internal/service/review ./apps/api/internal/service/outbox ./apps/api/internal/handler ./pkg/exporter` 通过,覆盖率分别为 statemachine 100.0%、review 70.1%、outbox 45.5%、handler 61.4%、exporter 87.8%;targeted `go test -cover ./apps/ai-worker/cmd/worker` 通过,worker 覆盖率 60.6%(含新增 verdict mapping + circuit open failover);targeted `pnpm -F web test -- StatsBoard` 通过(100 tests,含新增 stale retry guard)。
- 2026-05-28 S4 review fixes: `go test -race ./apps/api/... ./apps/ai-worker/... ./pkg/exporter ./pkg/llmreview` 全绿;`go vet` 通过;`pnpm -F web test` 99 通过(新增 stale-error / 轮询会停 / StatsBoard 错误态+重试 / 空维度 / CSV 注入回归);`pnpm -F web lint` 通过;`pnpm -F web build` 通过(VChart 进异步 StatsBoard chunk,eager vendor 降至 412KB);`git diff --check` 干净。
- 2026-05-27 S3 hardening cleanup: targeted `cd apps/api && go test -count=1 ./internal/handler -run 'Upload|DownloadUpload|RespondItem|ReviewerAIPromptActivateRouteIsNotRegistered'` 通过;targeted `cd apps/api && go test -count=1 ./internal/statemachine ./internal/service/submission ./internal/service/aireview ./internal/handler` 通过;targeted `cd apps/ai-worker && go test -count=1 ./cmd/worker -run 'AIWorkerCircuit|HandleAIDryRunCircuitOpenMarksRunFailed'` 通过;full `cd apps/api && go test -count=1 ./...` 通过;full `cd apps/ai-worker && go test -count=1 ./...` 通过;`git diff --check` 通过。
- 2026-05-27 S3 review finding fixes: targeted `cd apps/api && go test -count=1 ./internal/handler -run 'TestRespondItem_ExistingSubmissionUsesTemplateVersion|TestReviewerAIPromptsScopedToAssignedReviewer|TestReviewerAIPromptsRejectsUnassignedReviewer|TestReviewerAIPromptActivateRouteIsNotRegistered|TestReviewerDetailIncludesAIReviewAndAuditLogs'` 通过;targeted `cd apps/ai-worker && go test -count=1 ./cmd/worker -run 'TestHandleAIDryRun|TestAIWorkerCircuit'` 通过;`pnpm -F web test -- Queue` 通过;full `cd apps/api && go test -count=1 ./...` 通过;full `cd apps/ai-worker && go test -count=1 ./...` 通过;`pnpm -F web lint` 通过。
- 2026-05-27 S3 durable dry-run/rule ops: targeted `go test ./apps/api/internal/handler ./apps/ai-worker/cmd/worker` 通过;targeted `pnpm -F web test -- Queue Dashboard` 通过;full `go test ./apps/api/... ./apps/ai-worker/... ./pkg/llmreview` 通过;full `pnpm -F web test` 通过;`pnpm -F web lint` 通过;`pnpm -F web build` 通过(仍有既有 >500k chunk warning);`git diff --check` 通过。
- 2026-05-27 S0-S3 plan gap polish: `docker compose --profile ops -f deploy/docker-compose.yml config` 通过;targeted `go test ./apps/api/internal/handler -run 'TemplateSchema|Template'` 通过;targeted `go test ./apps/ai-worker/cmd/worker ./pkg/llmreview` 通过;CI 等价 Go command `go test ./apps/api/... ./apps/ai-worker/... ./pkg/llmreview` 通过;targeted `pnpm -F web test -- validator parser Plaza` 通过;full `pnpm -F web test` 通过;`pnpm -F web lint` 通过;`pnpm -F web build` 通过。
- 2026-05-26 S3 demo productization closure: targeted `cd apps/api && go test -count=1 ./internal/handler -run 'UpdateTaskBaseline|GoldenSampleDryRunQueuesAsyncRun|BatchReviewAppliesApprove|ReviewerDetailIncludesAIReviewAndAuditLogs'` 通过;targeted `cd apps/ai-worker && go test -count=1 ./cmd/worker -run 'CompleteAutoApprovesPassWhenHumanReviewDisabled|CompleteMovesSubmissionToHumanReviewWithAIVerdict|HandleAIReviewUsesProviderResultAndRecordsUsage|CompleteClearsPreviousErrorMessageOnRetrySuccess'` 通过;targeted `pnpm -F web test -- Dashboard Plaza Queue` 通过;full `cd apps/api && go test -count=1 ./...` 通过;full `cd apps/ai-worker && go test -count=1 ./...` 通过;full `pnpm -F web test` 通过;`pnpm -F web lint` 通过;`pnpm -F web build` 通过;`git diff --check` 通过。
- 2026-05-26 S3 Reviewer rule config entry: targeted `cd apps/api && go test -count=1 ./internal/handler -run 'ReviewerAIPrompts|ReviewerDetailIncludesAIReviewAndAuditLogs'` 通过;targeted `pnpm -F web test -- Queue Dashboard` 通过;full `cd apps/api && go test -count=1 ./...` 通过;full `pnpm -F web test` 通过;`pnpm -F web lint` 通过;`pnpm -F web build` 通过;`git diff --check` 通过。
- 2026-05-26 S3 Reviewer AI detail wiring/retry: targeted `cd apps/api && go test -count=1 ./internal/handler -run 'RetryAIReview|ReviewerDetailIncludesAIReviewAndAuditLogs'` 通过;targeted `pnpm -F web test -- Queue` 通过;full `cd apps/api && go test -count=1 ./...` 通过;`pnpm -F web lint` 通过;`pnpm -F web build` 通过。
- 2026-05-26 S2 Designer nested visual editing: targeted `pnpm -F web test -- Designer.integration.test.tsx SchemaRenderer` 通过;full `pnpm -F web test` 通过;`pnpm -F web lint` 通过;`pnpm -F web build` 通过。
- 2026-05-25 S2 Tabs/Group + S3 dry-run guard: targeted `pnpm -F web test -- SchemaRenderer Designer.integration.test.tsx` 通过;targeted `cd apps/api && go test -count=1 ./internal/handler -run 'AIPromptDryRun|GoldenSample.*DryRun|DryRunGuard'` 通过;full `cd apps/api && go test -count=1 ./...` 通过;`pnpm -F web lint` 通过;`pnpm -F web build` 通过。
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

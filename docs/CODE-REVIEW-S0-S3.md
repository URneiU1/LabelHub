# LabelHub S0–S3 Code Review

**Reviewed**: 2026-05-27
**Scope**: 完整 S0–S3（Go 后端 ~12.5k LOC / 68 文件 + ai-worker，前端 ~11.8k LOC / 47 文件，6 个 migration）
**Method**: 4 个并行领域 reviewer（workflow & state / API & AI pipeline / frontend / 跨切面安全），结论经本地运行验证
**Decision**: **REQUEST CHANGES** —— 1 个失败测试（CI 红） + 多个 HIGH 级正确性/安全问题需在 demo/提交前处理
**状态(2026-05-27 已修复, commit `6ead3c1`)**: C-1 outbox 测试、autosave 竞态、protocol-relative URL、登录限流、JWT 服务端吊销+轮换、BatchDryRun 异步化、DB 密码兜底/multiStatements/安全头/gin release、actor_type、死代码、gitignore 均已落地并验证(后端 -race 全绿 / 前端 89/89 / migration live / auth e2e)。**review.Apply 锁顺序经核实为误报**(Save 与 review 都 task→submission 先锁,本就无死锁;reviewer 建议的"先锁 submission"反会制造死锁),故未改。

---

## 验证结果（实跑）

| 检查 | 结果 |
|---|---|
| 前端 Vitest | ✅ 89/89 通过（autosave 假定时器泄漏已修） |
| 后端 `go vet`（分模块） | ✅ 干净 |
| 后端 outbox 单测 | ❌ `TestPublishEventUsesDeterministicTaskIDAndProcessingStatus` 失败 |
| `.env` 是否入仓 | ✅ 未入仓，已 gitignore |
| 嵌套目录 `apps/api/apps/web` | 未被 git 跟踪（杂物） |
| 5 种导出格式 | JSONL/CSV/XLSX 按代码注释明确属 **Sprint 4**，本轮不算缺口 |

---

## CRITICAL — 必须先修

### C-1. outbox publisher 单测失败，且暴露真实 bug（已实跑确认）
`apps/api/internal/service/outbox/publisher_test.go` + `publisher.go:133-138`

实跑报错 `remaining expectation which was not matched` —— 标记事件 `published` 的 `UPDATE` **根本没执行**（与你上轮 session 笔记 obs 4293 "UPDATE Never Executed" 一致）。根因不是测试写法，而是 `publishEvent` 的成功路径里那次 `Updates()` 出错后被 `p.warn()` 静默吞掉（logger 为 nil 时 warn 是 no-op）。

**影响**：① 评委直接看到红色 CI；② 生产中若该 DB 写失败，事件永远停在 `processing`，靠 `resetStaleProcessing` 5 分钟后才补救，且完全无日志。
**修**：在 `NewPublisher` 强制非 nil logger（默认 `zap.NewNop()`），让 `warn` 不再静默；同时定位 `Updates()` 失败的真实原因（GORM 隐式事务 vs 测试 mock 未配 `ExpectBegin/Commit`）并让测试反映真实代码路径。

---

## HIGH — demo 前应修

### 后端 · 工作流（评分轴：长链路状态机）
- **H-2 `StateSubmitted` 从不落库** — `submission.go:138-162`。审计日志写了 `draft→submitted→ai_reviewing`，但 `submissions.status` 一步写到终态；schema ENUM 含 `'submitted'`，任何按 `status='submitted'` 统计"待派发"的查询恒为 0。要么注释声明它是审计专用瞬时态，要么从 ENUM 移除。
- **H-1 `review.Apply` 锁顺序反转** — `review.go:75-93`。先无锁读 submission 拿 TaskID，再锁 task→submission；与 `Save` 的锁顺序相反，并发下死锁概率，MySQL 回滚后返回 500 而非 409。改成从已加锁的 submission 行取 TaskID，消除二次读并对齐锁序。

### 后端 · AI 流水线（评分轴：AI Agent 工程化）
- **H(api)-1 `BatchDryRun` 在 HTTP goroutine 同步阻塞最长 ~600s** — `handler/golden_sample.go:444-514`。20 样本 × 30s 超时全在请求线程，且未设 `WriteTimeout`，稍有并发就打满 Gin worker 池。单样本 `DryRun` 已走 outbox/Asynq，`BatchDryRun` 应同样改为逐样本入队即返回 `{runId, queued}`。
- **H(worker)-3 熔断器检查在 `markRunning` 之后** — `ai-worker/cmd/worker/ai_review.go:24-43`。熔断开路时仍先占任务、`retry_count++`、多一次 DB 往返才被拒。把 `circuit.check()` 提到 `markRunning` 之前。
- **M(worker)-2 `sql.ErrNoRows` 当成可重试错误** — `ai_review.go:47-49`。revision 被删等永久性失败会被 Asynq 重试到 maxRetry 才罢休。`errors.Is(err, sql.ErrNoRows)` 时应直接 `failover()` 走人工审核。

### 前端（评分轴：动态表单 Designer/Renderer/Schema）
- **H1 autosave 序列号竞态仍有漏洞** — `modules/labeler/Plaza.tsx:143`。定时器体内无条件回写 `autoSaveSeq.current = requestSeq`，当手动保存把 seq 推到同值时，stale-guard（:147）会误判通过，旧 answer 闭包覆盖 `lastSavedDraftKey`。修：定时器内不要改 `autoSaveSeq.current`，仅用 `requestSeq` 比较，自增放在 API 调用前。
- **H2 `isSafeURL` 协议相对 URL 绕过** — `shared/security/url.ts:10-13`。`//attacker.com/x.png` 解析后 protocol 是 `http:` → 通过校验，payload 里的外链图片/视频会被渲染，泄漏 Referer。修：解析后校验 `hostname === window.location.hostname`，或在 `new URL` 前先挡 `startsWith('//')`。

### 安全 · 认证
- **H 无登录限流** — `cmd/server/main.go`。`/auth/login` 无任何 rate limit，可暴力枚举。加 `golang.org/x/time/rate` 中间件（如 10 req/min/IP）。
- **H JWT 无服务端吊销** — `handler/auth.go:102-114`，logout 是客户端 no-op，已知属 S5。失窃 token 到自然过期前一直有效。

---

## 安全基建（demo 语境下"真问题但非生产事故"）

本项目无生产 VPS、全是 demo 凭据，下列项**对自托管演示机可接受**，但都是一两行的廉价加固，且会出现在评委可见的代码/配置里，建议顺手处理：

- **DB 密码硬编码兜底** `db.go:21` / `ai-worker/main.go:87`：`getEnv("DB_PASSWORD","labelhub_dev")` —— 去掉兜底串，缺失即 `log.Fatal`（与 `JWT_SECRET` 一致）。
- **`multiStatements=true`** `db.go:24` / worker DSN：应用层 DSN 去掉，放大未来注入面，迁移用单独连接。
- **`asynqmon.htpasswd.example` 被当真文件挂载** `deploy/docker-compose.yml:63`：重命名挂载目标为非 `.example`，把真 htpasswd 加进 `.gitignore`，改 bcrypt。
- **compose 明文 root/app 密码** `docker-compose.yml:6-9`：迁到 gitignore 的 `deploy/.env` + `${VAR}` 替换。
- **Adminer `0.0.0.0:18080` 无认证** / **Redis `0.0.0.0:6379` 无密码**：绑 `127.0.0.1`，或加 ops profile / requirepass。
- **缺安全响应头** `cmd/server/main.go`：加 `SecurityHeaders()`（nosniff / X-Frame-Options: DENY / Referrer-Policy）。
- **`gin.Default()` 默认 debug**：加 `gin.SetMode(gin.ReleaseMode)` 或 `.env.example` 设 `GIN_MODE=release`。

> 提示：`JWT_SECRET` 校验逻辑本身正确（强制 ≥32 字符、空则 fatal、算法校验、bcrypt），GORM 查询全参数化、无 Raw 拼接 —— 核心认证与查询层是干净的。

---

## MEDIUM / LOW（择机清理）

**后端**
- 包级可变全局 `NowUTC`（helpers.go:17 / review.go:49）与 `db.DB`（db/db.go:16）—— 并行测试 `-race` 隐患，建议注入 clock / 用返回值替代全局。
- 派发事件审计 `actor_type` 写成 `"user"` 应为 `"system"`（submission.go:150-162）。
- sweeper 用 `EventAIFailMax` 表示超时转移，与"重试耗尽"混淆，建议加 `EventAISweepTimeout`（aireview/sweeper.go:74）。
- 缺并发 claim 测试（claim.go）—— 状态机是评分轴，最关键的串行点却无并发测试。
- `pkg/llmreview/review.go:771` `truncateForError` 死代码；`zap.NewDevelopment()` 用于 server+worker 生产入口，应 `NewProduction()`。
- `BatchReview/ReviewSubmission` 用无限 `ShouldBindJSON`，应套 `http.MaxBytesReader`/`bindLimitedJSON`（reviewer.go:211,253）。

**前端**
- `Queue.tsx`(1604 行) / `Designer.tsx`(1602 行) 均超 800 行守则一倍，拆子组件。
- `renderMarkdown` 在 `renderer/widgets/ShowItem.tsx` 与 `shared/components/ShowItem.tsx` 重复实现，抽公共工具。
- tsconfig 缺 `"strict": true`（当前靠外部 `--strict` 跑过，新人易引入 null 不安全代码）。
- `widgetRegistry` 的 Group/Tabs 是静默 stub（不渲染子组件），非 SchemaRenderer 路径调用会静默丢子节点 —— 应 `throw` 或正确渲染。
- `RichText` 目前是 `<textarea>`，无 live XSS；若 S? 升级真富文本编辑器，输出须过 DOMPurify（加注释提醒）。

**杂物**
- 删除未跟踪的嵌套目录 `apps/api/apps/web/`，并把 `apps/api/apps/` 加进 `.gitignore`。

---

## 总评

S0–S3 的骨架质量高于一般 hackathon 项目：状态机定义正确、claim 用 `FOR UPDATE SKIP LOCKED` 正确串行、事务性 outbox 模式实现正确、前端 Designer→Schema→Renderer 用 `ParseResult<T>` 判别联合强制处理错误路径、`parseTemplateSchema` 是严谨的 fail-fast 递归校验器。三大评分轴都立住了。

对评委而言最该先动的：① 修掉失败的 outbox 测试（红 CI 最伤）；② `BatchDryRun` 改异步（否则"durable job 架构"这个亮点被同步阻塞自我否定）；③ 前端两个真 bug（autosave 竞态、协议相对 URL）；④ 顺手做廉价安全加固（去硬编码兜底 + 绑 localhost + gitignore htpasswd）。其余 MEDIUM/LOW 可在 S4 一并清理。

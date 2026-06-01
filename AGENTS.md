# AGENTS.md

## Project Workflow Notes

After every code change, update project documentation before the final handoff:

- Append a dated entry to **`docs/CHANGELOG.md`** under `## 最近完成` covering: what was completed, what still needs improvement (update `## 仍需提升`), next steps (update `## 下一步`), and verification (update `## 验证记录`). The `README.md` is now the judge-facing front page and should NOT be touched for dev-log updates.
- Update this file when Codex-specific workflow, handoff, or review notes change.
- Update `submission/` when delivery artifacts change (DEMO_SCRIPT, assets manifest, judge README).
- Keep entries factual and tied to commits or completed work.

## Current Handoff

- Current branch: **`s5-engineering-quality`**. S5 engineering-quality sprint is complete from `docs/PLAN-S5-IMPL.md`: CI now includes `./pkg/exporter` and a separate `-tags=integration` job; state machine transition coverage is 100%; `review.Apply` and outbox publisher tests were expanded; AI worker verdict mapping and circuit-open failover are covered; testcontainers integration now covers submit→outbox/Redis→AI result→revise→resubmit→approve→export against real MySQL+Redis; web `strict:true` is enabled and Renderer/Designer round-trip tests cover ShowItem modes, LLMTrigger, and Designer payload→Renderer parsing; OpenAPI fallback lives in `docs/openapi.yaml`, `pnpm -F web gen:api` generates `schema.d.ts`, Postman main-flow collection exists, and `ErrorBoundary` wraps the app root; exporter row-load DB errors are retryable instead of terminal failed; stats dimension averages only count the current submission revision; StatsBoard retry uses request sequence guarding; production compose/Caddy/Dockerfile templates plus `make dev`, ARCHITECTURE, DEPLOY, and S5_ACCEPTANCE docs are in place. Targeted coverage gates passed for statemachine/review/outbox/handler/exporter/worker, integration passed locally with Colima env, deploy compose config passed with `deploy/.env.example`, `pnpm -F web build/lint/gen:api` passed, and targeted web tests passed.
- Latest completed work: **S4 (async multi-format export + Owner stats board) is done** on branch `s4-async-export`. New `pkg/exporter` module (shared by api+worker) encodes JSON/JSONL/CSV/XLSX; `POST /tasks/:id/exports` writes exports(queued)+outbox+audit in one tx, the export worker handler runs `exporter.Run` (queued→running→succeeded, atomic temp→rename), and HMAC-signed `/exports/download` streams the file (expired 410 / tampered 401 / path-traversal 403). Owner dashboard gains `ExportPanel` (format/field-map/history-poll/download) and `StatsBoard` (VChart: progress/pass-rate/status/AI-vs-human/dimension averages via `GET /tasks/:id/stats`). `first_come` has an explicit concurrency acceptance test. All 4 formats validated end-to-end against live MySQL+Redis. See `docs/S4_ACCEPTANCE.md`.
- Also closed earlier this session: S0–S3 review findings (commit `6ead3c1` — failing outbox test, autosave race, JWT server-side revocation+rotation, login rate-limit, BatchDryRun async, security hardening).
- S5 was code-reviewed (Go / TypeScript / security) this session: production logic confirmed correct, one Critical deploy-template hole fixed (asynqmon admin UI was reverse-proxied unauthenticated in prod Caddy → now Caddy `basic_auth` with `${ASYNQMON_*:?}` enforcement) plus the Postman owner password de-hardcoded. Remaining review findings are test-confidence gaps and a minor React inline-style warning, tracked under Known Follow-Ups — not blockers.
- Current focus: **S6 is complete** (Day1–5 done; `docs/S6_ACCEPTANCE.md` finalized). Day5 browser smoke ran against a local stack (API on :8090 because :8080 is taken by an unrelated FastAPI; web proxied to it) via chrome-devtools — Designer responsive confirmed (1920 three-column / 1280 property panel wrapped below), StatusBadge/Editorial consistent, no console errors beyond React Router v7 future-flag warnings; the dev servers were killed afterward (no lingering processes). The smoke found and fixed a stale `/style-guide` (was Schematic/Inter/#0f62fe → now Editorial, live CSS-var palette + real StatusBadge). Note for future browser smokes: this user authorized backgrounding dev servers for the smoke and they were cleaned up; default is still NOT to spawn background dev servers without that authorization. Day4 delivered: Task 4.1 friendly error copy (`client.ts` throws a backwards-compatible `ApiError` with `code`/`requestId` and Chinese action copy for `INVALID_STATE`/`LLM_PROVIDER_ERROR`); Task 4.2 a11y — verified Designer nested controls already carry `aria-label` and the global focus-visible ring shipped Day1, plus a Labeler `Ctrl/Cmd+Enter` submit shortcut. Reviewer single-key A/R/B shortcuts were deliberately skipped (accidental-keypress risk on irreversible review actions). Day5 browser smoke needs the user to run `make web` locally — do NOT spawn a background dev server. Day1 primitives, Day2 status/empty/loading replacement, Runtime Tabs interaction, and now the Designer responsive layout + nested Group/Tabs canvas preview have all landed. `SchemaRenderer` Tabs render a real tablist/tabpanel and preserve flat answers across switches; Designer uses `Designer.css` responsive grid (1920 three-column / ≤1599 palette+canvas with property panel wrapped full-width below / ≤768 single column) and `NestedCanvasPreview` shows Group/Tabs child fields as mini rows. Note: a fix this session created the missing `Designer.css` that Codex had imported but never committed (build was broken until then); `make worker` needs LLM config to boot (`LLM_PROVIDER=mock` for export-only smoke); Makefile go-run targets fixed to package paths.

## Known Follow-Ups

- AI queue/failover/provider, prompt enable controls, Owner baseline/prompt product controls, Owner golden sample management, task-scoped dry-run history listing, durable queued single dry-run polling, Owner history/trend/guard table, Owner batch run wiring, backend batch semantics, basic batch pacing, AI auto-approve, Labeler autosave/revision UX, Reviewer batch operations, Reviewer AI verdict/detail/audit/retry display, Reviewer rule selection/activation plus Owner edit deep-link, provider retry/backoff controls, env-gated dry-run quota/circuit breaker, and AI worker 5xx circuit breaker are closed for the first S3 slices.
- Remaining S3 product follow-ups are optional polish rather than blockers for the current demo: async batch dry-run, richer charted trends/alerts, and broader seeded browser smoke coverage.
- S2 Designer v1 still lacks canvas-level nested dragging inside Tabs/Group and fuller layout editing.
- Formily, TipTap, `packages/schema-spec`, and `@dnd-kit/core` are still migration-level choices; do not mix them into small hardening commits without an explicit plan.
- Add FileUpload download/preview authorization and orphan temp-file cleanup.
- Add Owner API/UI for assigning `task_reviewers`; current demo assignment is seeded for `reviewer1`.
- S5 review test-confidence gaps (not bugs, tests pass but could give false confidence): worker `TestHandleAIReviewCircuitOpenFailsOverImmediately` passes via missing asynq retry-context rather than proving the circuit-open path (would still pass if `circuit.check()` were deleted); `review_test.go` concurrent-update and `publisher_test.go` TaskID-conflict tests use loose `AnyArg`/regex that wouldn't catch a dropped optimistic-lock WHERE or a wrong post-conflict status; integration test stubs the AI result via direct SQL instead of running the worker. Tighten these when touched.
- Minor: `SchemaRenderer.tsx` tab buttons mix `border` shorthand + `borderColor` longhand in `tabButtonActiveStyle`, triggering a React inline-style console warning on tab switch; fold the color into the full `border` shorthand when next editing that file.
- Deploy hardening (lower priority, single-host demo acceptable): pin base images by digest, set Redis `--requirepass`, and require `LLM_API_KEY` when `LLM_PROVIDER != mock`.


<claude-mem-context>
# Memory Context

# [LabelHub] recent context, 2026-06-01 3:58am GMT+8

Legend: 🎯session 🔴bugfix 🟣feature 🔄refactor ✅change 🔵discovery ⚖️decision 🚨security_alert 🔐security_note
Format: ID TIME TYPE TITLE
Fetch details: get_observations([IDs]) | Search: mem-search skill

Stats: 50 obs (7,977t read) | 469,885t work | 98% savings

### May 31, 2026
S769 LabelHub owner 审核结果质检反馈闭环功能实现 — 用户要求"做你推荐的"，即将 Owner「审核结果」节从薄回显升级为逐条 AI vs 人工判定对比面板。 (May 31 at 9:33 PM)
S770 Redesign LabelHub UI to match PetaV2 style; work executed comprehensive navigation architecture refactoring as prerequisite (URL-routed sections, in-page sub-tabs, CSS-based filtering) (May 31 at 10:00 PM)
S771 Redesign LabelHub Owner navigation from single-page internal section switching to URL-based independent page routing, matching PetaV2 style conventions; verify all existing tests remain passing (May 31 at 11:15 PM)
S772 LabelHub Owner 后台「数据导出」分节拆分为「导出配置/导出历史」子页，并泛化子页过滤机制 (May 31 at 11:19 PM)
### Jun 1, 2026
S773 用户询问数据导出页面是否已拆分为标签页 (Jun 1 at 12:18 AM)
S774 LabelHub Owner 后台「数据导出」分节去掉"全部"标签，只保留两个子页标签 (Jun 1 at 12:56 AM)
S775 LabelHub「数据导出」分节去掉"全部"标签，只保留「导出配置」和「导出历史」两个子页标签 (Jun 1 at 2:37 AM)
5885 2:37a ✅ 生产端 /owner/export 重定向到 /owner/export/config 验证成功
5886 " ✅ 生产端 export 分节 UI 全链路验证通过
5887 2:38a ✅ LabelHub Sprint 进度更新：Sprint 6 进行中，1-5 已完成
5888 " ✅ CHANGELOG.md 追加 owner-export-no-overview 条目
5889 " ✅ CHANGELOG commit 72a5266 提交，近三条 commit 序列确认
S776 LabelHub 数据看板丰富方案调研与规划 (Jun 1 at 2:38 AM)
5890 2:57a 🔵 LabelHub StatsBoard 现有实现结构
5891 2:58a 🔵 API stats 端点实现文件位置确认
5892 " 🔵 LabelHub stats.go 完整实现细节
5893 " 🔵 Submission 和 AIReview 模型可用字段确认
5894 " 🔵 LabelHub 全量数据模型结构速览
5895 2:59a 🔵 HumanReview 有 Stage 字段，支持多阶段审核
S777 全仓代码审查 LabelHub 项目 (Jun 1 at 2:59 AM)
5896 3:07a 🔵 LabelHub stats_test.go 测试覆盖结构
5897 3:08a 🟣 LabelHub stats.go 新增混淆矩阵、分桶、趋势三个响应字段
5898 " 🟣 stats.go 接入 buildConfusion 填充混淆矩阵响应字段
5899 3:09a 🟣 TaskStats 新增 AI 评分分桶与每日完成趋势两条 SQL 查询
5900 " 🟣 实现 buildConfusion 和 bucketScores 两个纯函数
5901 " 🟣 stats_test.go 扩展集成测试覆盖混淆矩阵、分桶、趋势三个新字段
5902 " 🟣 新增 TestBuildConfusion 和 TestBucketScores 纯函数单元测试
5903 " 🔵 stats 测试失败：ai_score 查询被 sqlmock 拒绝返回 500
5904 3:10a 🔴 修复 sqlmock ai_score 正则不匹配 GORM 反引号问题
5905 " 🔴 stats handler 全部 4 个测试修复后通过
5906 3:11a 🔵 StatsBoard.tsx 当前仅渲染 5 个卡片，三个新字段尚未接入前端
5907 3:12a 🟣 StatsBoard.tsx 扩展 TaskStats 类型接入三个新字段
5908 " 🟣 StatsBoard.tsx 重构图表 spec 并准备混淆矩阵渲染数据
5909 " 🟣 StatsBoard.tsx 全面重构看板布局，新增 KPI 行、混淆矩阵热力表和三个新图表卡
5910 3:13a 🟣 StatsBoard.tsx 补全新布局所需 CSS 常量，移除废弃进度条样式
5911 " 🟣 LabelHub stats 看板扩展全链路验证通过
5912 3:14a 🔵 前端 vitest 运行失败，退出码 1
5913 " 🔵 StatsBoard 测试失败：mock 数据缺少三个新字段导致 .map() 报错
5914 3:15a 🔵 StatsBoard.test.tsx 与新组件有四处不兼容，需同步更新
5915 " 🔴 StatsBoard.tsx 对三个新字段和 dimensionAverages 加防御性空值默认
5916 3:16a 🔴 StatsBoard.test.tsx mock 补全三个新字段，组件 confMap/confMax 改用防御变量
5917 " 🔴 StatsBoard.test.tsx 第一个测试用例更新 aria-label 断言和通过率格式
5918 " 🟣 LabelHub stats 看板全栈扩展完整通过所有验证
5919 3:17a 🟣 stats 看板扩展 commit 并触发生产部署
5920 3:18a 🔵 LabelHub Template Designer 结构概览
5921 3:19a 🟣 stats 看板扩展成功部署到生产环境
5922 3:20a 🔵 生产 stats API 实测：新三字段正常返回，发现两处细节问题
5923 " 🔵 生产看板页面浏览器验证全部通过
5924 " 🔵 生产 /owner/stats 页面截图验证看板完整渲染
5925 3:21a 🔵 生产混淆矩阵真实数据验证：pass/approve=7，pass/reject=2
5926 3:25a 🔵 LabelHub 项目完成开发，进入代码审查阶段
5927 3:26a ⚖️ LabelHub 标注质量机制字段选型决策
5928 3:27a 🔵 LabelHub 项目未初始化 CodeGraph 索引
5929 3:29a ⚖️ LabelHub S8 质量控制功能规划 — Handoff 文档
5930 " 🔵 LabelHub Template Designer 现有版本控制与只读机制
5931 3:30a 🔵 LabelHub API 架构盘点：状态机、分发逻辑、迁移文件位置
5932 3:31a ✅ LabelHub S8 Handoff 文档创建完成
5933 3:32a ✅ CLAUDE.md 更新：S8 交接入口指向 HANDOFF-S8.md
5934 " ✅ S8 Handoff 文档提交入库
S778 为 LabelHub S8 sprint 写 handoff 文档，说明已完成工作并为下一个开发者指定两项待办（Designer 重做 + S8 任务条例） (Jun 1 at 3:32 AM)
**Investigated**: - 阅读了 Designer.tsx 渲染部分（第362-511行），确认现有三栏布局、LATEST/EDITABLE vs READONLY 双态、Fork 版本机制、右侧属性面板已有「基础/校验/联动」三标签
    - 用 git log + grep 盘点了后端结构：状态机模块（statemachine/machine.go + task.go）、分发逻辑（service/submission/claim.go）、迁移文件（最新 008_task_status_add_ended）
    - 确认 Task 模型已有 Deadline/Tags/HumanReviewEnabled/Distribution/QuotaPerUser/PublishedAt 等字段，S8 新字段只需从 009 起追加迁移
    - 读取了 CLAUDE.md 现有 Current Handoff 区块，了解历史交接脉络（S4→S5→S6→S7）

**Learned**: - LabelHub 前端版本控制模式已完备：isLatest 决定 EDITABLE/READONLY，READONLY 强制 Fork 新版本，与 S8「发布后冻结」设计方向一致，后端补状态机拒绝逻辑即可
    - 分发/领题逻辑集中在 claim.go，overlap/lease/sampling 均应在此挂钩
    - 导航 URL↔store 同步边界在 AppLayout.tsx，Dashboard.tsx 保持 router-free，改 Designer 时不得破坏这个边界
    - 子页过滤用 CSS data-sub 属性，默认 all 既支持概览又保证 jsdom 测试通过

**Completed**: - 创建 docs/HANDOFF-S8.md：§0 本会话已完成工作（6 个 commit 表 + 关键设计边界）；§1 Designer 待办（现状 vs 设计稿差距，含画布 Tab 栏/顶部栏/物料分组/字段卡精修）；§2 S8 任务条例（第一批必做 5 项 + 第二批 3 项 + 任务创建页三区块 + 代码落点）；§3 工程约定（测试门槛/sqlmock 反引号坑/迁移规范）
    - 更新 CLAUDE.md Current Handoff 顶部，插入 2026-06-01 指针条目指向 HANDOFF-S8.md
    - commit 629780f：docs(handoff): write S8 handoff (Designer reskin + task policies)，2 文件 108 行新增，已提交到 feat/ui-reskin-s7-and-followups 分支

**Next Steps**: 本会话已明确结束于 handoff 文档交付，两项待办（Designer 重做 + S8 任务条例实现）留给下一个开发者，代码未动。用户可选择在此会话继续开始其中一项，或留给新会话处理。


Access 470k tokens of past work via get_observations([IDs]) or mem-search skill.
</claude-mem-context>
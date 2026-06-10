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

# [LabelHub] recent context, 2026-06-09 1:19am GMT+8

Legend: 🎯session 🔴bugfix 🟣feature 🔄refactor ✅change 🔵discovery ⚖️decision
Format: ID TIME TYPE TITLE
Fetch details: get_observations([IDs]) | Search: mem-search skill

Stats: 50 obs (9,706t read) | 492,268t work | 98% savings

### Jun 2, 2026
6274 5:24a 🔵 Designer 布局 1280×800 实测：overflow=0 但不能确认三栏
6275 5:25a 🔵 Designer.tsx 已有 1 行本地修改，其他 6 文件为队友变更
6276 " 🔴 Designer 属性面板内容溢出修复（commit 3dfc17b）
S800 Designer 三栏布局修复：属性栏常驻右侧第三栏，专注模式隐藏全局侧边栏 (Jun 2 at 5:26 AM)
6278 1:03p 🔵 LabelHub 模板搭建界面空间问题 — 调研 AppLayout 侧边栏隐藏方案
6280 " 🔵 AppLayout.tsx 结构：侧边栏固定渲染，无路由感知隐藏逻辑
6281 1:04p 🔵 LabelHub 路由结构：TemplateDesigner 嵌套在 AppLayout 内，路径为 /owner/:taskId/templates/:templateId
6292 1:10p ✅ Designer 三栏布局断点修复交接文档已提交
6299 1:12p 🔵 Designer.tsx 布局结构：网格由 CSS 类 + 内联 style 双轨驱动
6304 1:13p 🔵 Designer.tsx 三栏 DOM 结构精确行号确认
6306 1:14p 🟣 AppLayout 新增 isDesigner 标志用于 Designer 专注模式
6307 " 🟣 AppLayout 实现 Designer 专注模式：侧边栏在 Designer 页自动隐藏
6309 1:15p 🔴 Designer.css 三栏列宽更新：property 栏拓宽至 340px，注释同步反映专注模式
6310 " 🔴 Designer.css 媒体查询断点从 1199px 降至 1024px，新增专注模式 sticky 面板规则
S801 Designer 三栏布局修复 + 专注模式（隐藏侧边栏）实现，tsc 已通过，等待 eslint 完成后跑测试和部署 (Jun 2 at 1:17 PM)
6323 1:20p 🔵 LabelHub web tsc -b 会挂起，需手动 pkill 终止
6324 1:22p 🔵 tsc -b 无报错通过，耗时约 84 秒
6326 1:25p 🔵 本地开发机内存压力导致 eslint/tsc 工具链严重变慢
S802 Designer 三栏布局修复 + 专注模式实现，代码已 rsync 至 prod，Docker 重建 web 容器中 (Jun 2 at 1:26 PM)
6333 1:27p 🔵 vitest --pool=threads 对 AppLayout + Designer 报 2 errors，0 测试运行
6335 1:28p 🔵 vitest 4.1.7 不支持 --poolOptions CLI 参数，该配置只能写 vitest.config
6341 1:29p 🔵 vitest --pool=threads 固定 60 秒超时崩溃，原因是 transform 0ms 说明 worker 未能初始化
6342 1:31p 🔵 AppLayout.test.tsx 3 个测试全部通过，setup 耗时 51.59s（jsdom 环境初始化极慢）
6343 1:32p 🔵 Designer 模块 17 个测试全部通过，无回归
6344 " ✅ Designer 专注模式代码已 rsync 至 prod 服务器 43.155.210.70
6345 " 🔵 LabelHub prod 部署方式：docker compose --build web 重建前端容器
S803 Designer 三栏布局 + 专注模式修复，sticky 面板失效根因已定位并修复，第二轮 rsync+rebuild 部署中 (Jun 2 at 1:32 PM)
6348 1:34p 🔵 prod 验证：nav 隐藏和三栏布局正确，但 palette/property sticky 未生效（panels 随 document 滚走）
6349 1:35p 🔵 lh-topnav 是 position:sticky（非 fixed），与 palette/property sticky 共享同一 scroll container
6351 " 🔴 Designer.css 修复 sticky 面板失效：解除 lh-shell__main overflow，top 值补入 topnav 高度
S804 Designer 三栏布局专注模式完整实现并部署：隐藏全局导航、放宽列宽、属性/物料面板 sticky 吸顶自身滚动 (Jun 2 at 1:36 PM)
6352 1:37p 🔵 第二轮 prod 验证：overflow 修复生效，palette pinned ✅，property 因内容超过视口高度仍会滚出
6353 1:38p 🔵 property 面板 offsetH=1000px（几乎满视口），scrollH=4921px；sticky 失效原因与 CSS Grid 行高关联
6354 1:40p 🔵 Designer 专注模式 1920px 视口截图：三栏并排、无侧边栏、property 内容超高导致无法完全 sticky
6355 " 🔵 prod Designer 页面唯一控制台错误：CSP 阻止 Google Fonts 加载（style-src 未含外部域）
6356 1:41p 🔵 1280x800 视口验收全部通过：三栏正常，palette/property 均 sticky 锁定在 y=72
6357 " ✅ Designer 专注模式 1280x800 视口截图验收通过：三栏并排，sticky 面板锁定正常
6358 1:42p ✅ CHANGELOG.md 新增 designer-focus-mode 条目，完整记录专注模式实现细节和验证结果
6359 " ✅ Designer 专注模式改动已提交至 feat/ui 分支，commit 4bcae06
S805 Designer 专注模式完整交付（commit 4bcae06），任务已完成，等待用户决定是否进一步改为全程固定三栏 app-shell (Jun 2 at 1:42 PM)
S806 LabelHub Designer 画布预览纵向过长的原因分析 + 是否修改的推荐 (Jun 2 at 1:43 PM)
6368 3:52p 🔵 LabelHub Designer 画布预览纵向过长的代码根因调查
S807 LabelHub Designer 画布预览过长问题——根因分析 + 紧凑化改造实施 (Jun 2 at 3:52 PM)
6374 3:55p 🔵 Designer.integration.test.tsx 覆盖范围确认
6376 3:56p 🔵 Designer 集成测试包含 Schema round-trip 验证和嵌套预览 aria-label 断言
6379 3:57p ✅ Designer 画布 minHeight 从 800 降至 360
6380 " 🟣 CanvasField 新增 per-card `showPreview` 状态，实现按需展开 Widget 预览
6381 3:58p 🟣 Designer 画布紧凑化改造完成：Widget 预览改为每卡按需展开
S808 LabelHub Designer 画布紧凑化改造——根因分析 + 实施 + 测试 + 部署 (Jun 2 at 4:01 PM)
6382 4:03p 🔵 Designer 集成测试 2/17 失败——per-card 预览 toggle 导致 Widget DOM 不再默认渲染
6383 4:04p 🔵 2 个失败测试精确定位：ShowItem 预览内容断言因 toggle 默认关闭失效
6388 " 🔴 ShowItem 默认展开修复：`useState(field.widget === 'ShowItem')` 解决测试失败
6389 4:05p 🟣 Designer 画布紧凑化改造全部测试通过：17/17 绿灯
6390 4:07p 🔵 生产环境实测：compact canvas 画布高度 2670px，12 个 toggle 按钮，ShowItem 默认展开，toggle 功能正常
6391 " 🔵 生产实测：单个 Radio 字段卡片高度 161px，toggle 按钮 35px，无 Widget 渲染区域
6392 4:09p ✅ 删除 CanvasField header 中冗余的 `canvas-field__meta` span
6393 4:10p ✅ meta span 删除后 tsc + vitest 全绿：17/17 通过，TypeScript 无错误
S809 LabelHub Designer 画布紧凑化改造——5 步改动全部完成，第二轮 rsync 推生产中 (Jun 2 at 4:10 PM)
6394 4:11p 🔵 生产最终实测：画布从 2670px 降至 2328px（-13%），Radio 卡片从 161px 降至 132px（-18%）
6395 4:12p 🟣 LabelHub Designer 画布紧凑化改造完成并截图验证：终态 scrollHeight 2328px，卡片 132px

Access 492k tokens of past work via get_observations([IDs]) or mem-search skill.
</claude-mem-context>
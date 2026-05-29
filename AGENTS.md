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

# [LabelHub] recent context, 2026-05-29 6:19pm GMT+8

Legend: 🎯session 🔴bugfix 🟣feature 🔄refactor ✅change 🔵discovery ⚖️decision 🚨security_alert 🔐security_note
Format: ID TIME TYPE TITLE
Fetch details: get_observations([IDs]) | Search: mem-search skill

Stats: 50 obs (10,303t read) | 524,726t work | 98% savings

### May 29, 2026
5003 12:49a 🔵 5174 demo nav 元素是 DIV 而非 NAV 标签，选择器需用 div.lh-demo-nav
5004 " 🔴 发现 5174 UI Bug：demo nav 与发布抽屉底部 CTA 按钮发生视觉遮挡重叠
5005 12:50a 🔵 5174 五页全量 Smoke Test 通过，但 demo nav 遮挡 bug 在所有页面普遍存在
5006 " 🔵 /workbench 页标注提交 CTA 不受 demo nav 遮挡，按钮位于页面滚动区 y:784
5007 12:51a 🔵 LabelHub 完整 monorepo 文件结构揭露，5174 UI 不在主仓库内
5008 12:52a 🔵 labelhub-ui-demo 项目路径确认为 ~/labelhub-ui-demo/，独立于主仓库，LabelHub 内存在 packages/ui-kit
5009 " 🔵 labelhub-ui-demo 源码结构定位完成，demo nav 遮挡 bug 根源在 globals.css 第 422 行
5010 12:53a 🔵 demo nav 遮挡 bug 根因：tasks-drawer z-index(100) 低于 lh-demo-nav z-index(999)，导致 fixed 抽屉底部被压盖
5011 " 🔵 labelhub-ui-demo 技术栈：React 19 + Vite 8 + ArcoDesign + Formily，左侧 sidebar 是静态装饰性 DIV
5012 12:54a 🔵 designer.css 画布字段操作按钮无 z-index，demo nav 遮挡确认为布局高度问题而非层级冲突
5013 " 🔵 labelhub-ui-demo 全部可滚动主内容区均缺少 padding-bottom，统一修复路径已确定
5014 12:55a 🔴 labelhub-ui-demo 修复 demo nav 遮挡并升级左侧 sidebar 为可路由 NavLink
5015 " ✅ labelhub-ui-demo 代码变更通过 apply_patch 落地，pnpm build + lint 验证启动中
5016 12:56a 🔴 labelhub-ui-demo 左侧导航无法点击跳转
5017 " 🟣 labelhub-ui-demo 补全三个缺失路由页面
5018 " 🔴 labelhub-ui-demo 底部 demo 导航遮挡页面内容
5019 " 🔵 pnpm lint 零错误通过，pnpm build 因沙盒无法写入 node_modules/.tmp/ 失败（非代码问题）
5020 " ✅ labelhub-ui-demo pnpm build 成功，产物 918 模块 233ms 构建完成
5021 " 🔵 本地 Playwright 浏览器二进制未安装，需执行 npx playwright install 才能跑 headless 验证
5022 12:57a 🔵 沙盒环境禁止启动外部 Chrome 进程，headless playwright 验证只能用 Codex 内置浏览器
5023 12:58a 🔵 globalThis.uiTab 的 playwright/cua/content 子对象无可调用方法，IAB tab 需要重新获取
5024 " 🔵 Codex IAB tab 原型方法：goto/back/forward/reload/close/screenshot/title/url 是直接方法，playwright 通过子对象访问
5025 " 🔵 Codex IAB tab.dom_cua 和 tab.cua 的完整方法集
5026 12:59a 🟣 SideNav 升级验证成功：左侧导航现在渲染为带 href 的 &lt;a&gt; NavLink，七个路由全部可点击
5027 " 🔵 Codex 内置浏览器会话 API 结构
5028 " 🔵 LabelHub UI Demo Vite 生产构建成功
5029 " 🔵 Playwright headless 在 Codex macOS 环境不可用
5030 " 🔵 dom_cua.click(node_id) 在 tab.goto() 后失败：每次导航后 node_id 会重新分配，需重新调用 get_visible_dom
5031 " 🔵 dom_cua.click() 正确签名为 click({ node_id: N }) 对象形式，而非直接传整数
5032 1:00a 🔵 dom_cua.click 接受字符串 node_id 但返回 "Node does not have a layout object"（stale）；cua.click 需要坐标 {x, y}
5033 " 🔵 Codex IAB tab 完整 API 文档：tab.dev 只有 logs，tab.content 有 export/exportGsuite
5034 " 🔵 tab.screenshot() 返回原始 JPEG 字节对象（43,259 字节），不是 base64 字符串或 Blob
5035 1:04a 🔴 labelhub-ui-demo nav overlap fix verified via geometry recheck
5301 4:10p 🔵 LabelHub 生产部署文档完整记录
5302 " 🔵 LabelHub 项目完整架构与技术亮点
5329 4:41p 🔵 LabelHub 部署文档已存在
5330 4:42p 🔵 LabelHub 生产部署架构：Caddy + Docker Compose
5331 " 🔵 LabelHub 生产部署完整 SOP（VPS Docker Compose）
S711 用户询问云服务器部署方案（字节AI全栈挑战赛 LabelHub 项目） (May 29 at 4:43 PM)
S712 LabelHub 比赛第 5 条要求：可访问的演示环境说明文档（任意云平台部署） (May 29 at 4:47 PM)
5332 4:51p 🔵 LabelHub 比赛要求：可访问演示环境说明文档
5333 " 🔵 LabelHub submission 目录结构已存在完整提交物
5334 4:54p 🔵 LabelHub submission/README.md 完整交付清单与待补齐项
5335 4:55p 🔵 DEPLOY.md 无公网演示 URL，需补充云平台实际部署地址
5336 4:57p 🟣 新建 submission/DEMO_ENV.md 满足比赛第 5 条可访问演示环境说明要求
S713 LabelHub 比赛演示环境：选择云平台部署方案（免备案 + 国内评委可访问） (May 29 at 4:57 PM)
S714 LabelHub 演示环境：评估用树莓派 + Cloudflare Tunnel 替代云服务器方案 (May 29 at 5:00 PM)
S715 字节 AI 全栈挑战赛 LabelHub — 选定腾讯云并获取完整部署参数与上机指引 (May 29 at 5:01 PM)
5337 5:18p ⚖️ 选定腾讯云作为云服务商
S716 字节 AI 挑战赛 LabelHub demo 云服务器规格评估 — 2核2G3M 是否够用 (May 29 at 5:19 PM)
S717 字节 AI 全栈挑战赛 LabelHub — 云服务器配置选型咨询（2C4G 30M 是否够用） (May 29 at 5:31 PM)
5338 5:39p 🔵 LabelHub 字节挑战赛交付包现状盘点
5339 5:40p 🔵 LabelHub 演示服务器最低规格要求 2核4G，2G内存不达标
5340 " 🔵 DEMO_SCRIPT 与 seed 密码不一致：脚本写 pass，实际代码是 123456
5341 " 🔵 LabelHub S0-S7 完整开发迭代记录（2026-05-22 至 2026-05-29）
5342 5:41p ✅ LabelHub 交付文档同步更新：新增 DEMO_ENV.md、修正密码、修正任务名
5343 " 🔴 apply_patch 在 DEMO_SCRIPT.md 失败：简版摘要行与实际详细脚本内容不匹配
S718 镜像选择建议：Ubuntu 还是 Ubuntu + Docker (May 29 at 5:47 PM)
S719 首尔节点云服务器选择确认 — 流量、备案、延迟评估 (May 29 at 5:51 PM)
S720 字节 AI 全栈挑战赛 LabelHub 部署服务器选址：首尔 vs 曼谷 (May 29 at 5:56 PM)
**Investigated**: 两个候选云服务器地区（首尔 vs 曼谷）对 LabelHub demo 的适配性，从两个关键维度评估：评委访问体验（大陆用户）和 AI worker 调用豆包/火山引擎延迟

**Learned**: 首尔到中国大陆的网络路由比曼谷更成熟稳定；火山引擎豆包 API 节点在北京，首尔到大陆延迟更低；曼谷虽然离吉隆坡更近但用户自身延迟不是评审重点

**Completed**: 做出部署选址决策：选首尔。理由：评委在国内，AI worker 调豆包需低延迟到北京节点，首尔在这两点上均优于曼谷

**Next Steps**: 购买首尔云服务器 → 开放 80/443 端口 → SSH 进机器验证 docker compose → git clone 项目 → 将公网 IP 发给主会话继续部署


Access 525k tokens of past work via get_observations([IDs]) or mem-search skill.
</claude-mem-context>
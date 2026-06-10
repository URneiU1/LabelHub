# LabelHub 交付前 Code Review 汇总

**初次 Review**：2026-05-28（4 路并行 — go-reviewer / typescript-reviewer / security-reviewer / 交付物 readiness）
**第二轮 Review + 最终核验**：2026-06-10
**仓库 / 分支**：`~/Desktop/LabelHub` · `feat/ui-reskin-s7-and-followups`
**比赛 deadline**：2026-07-08 ~ 2026-07-15
**审查范围**：~31k LOC（15.5k Go + 14k TS/TSX）+ 部署配置 + 交付物

---

## 最终结论（2026-06-10 核验）

**可交付。** 初次 review 标记的 **8 个 Critical 与 10 个 High 已全部修复并部署**到 prod（`http://43.155.210.70`，本地 HEAD == 线上）；`submission/` 交付包已组装完整（源码 / 评委 README / 架构 / 11 张截图 / OpenAPI + Postman / AI-Coding PDF / 部署 SOP）；在此基础上第二轮 review 又发现并修复 **6 处问题（含 1 处 IDOR Critical）**。

**测试状态**：Go 全绿（handler 209 / submission 70 / worker 34 / exporter 41 / llmreview 24 / middleware 3，`go build` / `go vet` 干净）；Web 此前跑不起来的套件在 vitest pool 修复后通过（AcceptancePanel / Plaza / ItemNav 29/29）；类型检查以服务端 `tsc -b && vite build` 为准，每次部署均通过。

**唯一硬缺口**：演示视频（脚本见 [`DEMO_SCRIPT.md`](DEMO_SCRIPT.md)，需实录）。

剩余 Medium / Low 中有若干**有意推迟**项（三大文件行数拆分、handler `ctx` 透传、`review.Apply` 的 probe SELECT、`docs/` 归档），均为**非阻塞工程债**，在下文逐条标注了状态与原因——本汇总不把未修项伪装成已修。

> 本文档保留 2026-05-28 原始 review 的全部条目作为过程留痕；每条的 `[x]/[ ]` 与状态标注均经 2026-06-10 对当前代码 grep 核验后更新（Critical / High 附 `file:line` 证据）。

---

## 第二轮 Review（2026-06-10）

初次 review 落地后，对 S7/S8 新增代码（labeler 大任务领取、AI 审核队列视图、acceptance 复核）又做了一轮针对性 review，发现并修复 6 处：

- **`7e25f8b` · Critical（IDOR）** — `GET /reviewer/ai-reviews` 无任务范围限制，任一 reviewer 可枚举所有任务的 AI review + prompt 配置。改为 join `submissions` 并套 `applyReviewQueueScope`（`task_reviewers`）；handler 测试断言 scope join 存在以防回归。
- **`57f7a32`** — `approveCountsByRevision` 漏 `superseded_at IS NULL`（打回后队列计数虚高）；`ReviewerResults` 在 `(updated_at, id)` 排序下用 id-only 游标会跳/重 → 改复合游标。
- **`ba2f40d`** — worker 遇不可解析 payload 返 `nil`（静默成功）→ 改 `SkipRetry`（死队列可见）；`markExportFailed` 吞掉自己的 UPDATE 错 → 返回可重试错，导出不会卡死在 `running`。
- **`dfbbf8a`** — `openTask` 吞掉所有 claim 错 → 只吞 `CONFLICT`；`stepItem` 可能 `claim(0)` / nav-load 竞态重领；`AIVerdictPanel` 硬编码阈值 80 → 改读 `prompt.passThreshold`；`JSONEditor` 非法 JSON 增加内联警告。
- **`f60682e`** — 补 threshold 边界 + acceptance 写操作覆盖测试。
- **`ed322ed`** — Plaza client mock 补 `claimTask`。

**3 个 agent 报告的 "Critical" 经核验为误报，未改**（教训：改前先验证）：

- sweeper "partial commit" —— 实由 `FOR UPDATE SKIP LOCKED` 保护，无半提交。
- SchemaRenderer "infinite loop" —— `pruneHiddenAnswerValues` 在值未变时返回同一引用，不触发重渲染。
- TabsField "jumps every keystroke" —— effect 依赖是原始值，按值比较，不会每次击键跳变。

### 交付前硬化（同期，非 bug 修复）

- **`a2c46da`** — AI 审计以 seed 的 `system_ai` 账号身份可追溯（`actor_type='ai_worker'` + `actor_id`）。
- **`777eac8`** — 题目导航定高虚拟滚动（`visibleRange` 纯函数 + 定窗渲染），解 5000 题压测暴露的全量渲染瓶颈。
- **`78ed3ff`** — vitest `pool: vmForks → threads`，根治本机"测试卡死"（node 24 上 vmForks 的 vm 隔离编译病态慢，同文件 929s → 6.4s）。
- **5000 题线上压测** — API 无瓶颈（import 5.4s / publish 0.24s / `GET labeler/items` 全 5000 行 ≈468KB 0.7s / claim-open 0.2–0.4s）；压测数据测后已清。

---

## 🟥 Critical — 必须修，否则交不了 / 评委直接打回

### 交付物层（4 条）

- [x] **C1. `submission/` 目录是空的** → ✅ 已组装完整（11 项交付物：源码 + 评委 README + DEMO_SCRIPT + ARCHITECTURE + 11 张截图 + OpenAPI + Postman + AI-Coding PDF + DEPLOY + CODE-REVIEW + BEYOND-REQUIREMENTS + LICENSE）。

- [x] **C2. 演示视频 + AI Coding 8-15 页 PDF 完全没起** → ✅ AI-Coding-Process PDF 已生成（`assets/AI-Coding-Process.pdf`，含本轮硬化章节）；⏳ 演示视频脚本就绪（`DEMO_SCRIPT.md`），待实录上传。

- [x] **C3. `.env.example:32` 占位 `/absolute/path/to/...`，API 启动 fatal** → ✅ `Makefile:5` `EXPORT_DIR := $(CURDIR)/data/exports` 自动注入，`make api` / `make worker` 均 export 该值。

- [x] **C4. 根 README 前 187 行是 sprint 流水账 + "仍需提升" 红字** → ✅ 根 README 改为干净评委门面（CI/License/Go/React/TS badge + "评委友好链接" + 5 步快速启动），旧流水内容挪 `docs/CHANGELOG.md`。

### 代码层（4 条）

- [x] **C5. `reviewer.go` `retryAIReview` 漏 `RowsAffected` 检查** → ✅ `reviewer.go:441/530/644-645` 加 `ErrConcurrentWrite` 返回 + `RowsAffected != 1` 守卫。

- [x] **C6. `ReviewerQueue` 没 `.Limit()`，大任务 OOM** → ✅ `reviewer.go:188` `Order(...).Limit(200).Find(...)`。

- [x] **C7. `Queue.tsx` 决策按钮没 disable 在 `loading`，双提交** → ✅ `Queue.tsx:811/815/819` 三按钮 `disabled={loading || (!showingDemo && !schema.ok)}`。

- [x] **C8. `Dashboard.tsx` `pollGoldenSampleRun` 递归 `setTimeout` 无 cleanup** → ✅ 改用 `goldenSampleRunSeq` generation guard + `isCurrentTaskAction` + `clearTimeout`（切走页面即失效）。

---

## 🟧 High — 评委读代码会扣工程质量分

### 后端 / 安全

- [x] **H1. 4 处 handler 把 GORM 报错原文吐给 owner** → ✅ `ai_dry_run.go:77` / `ai_prompt.go:134` 改静态 `"internal error"`，`golden_sample.go` 用静态 `"failed to ..."`；残留 `err.Error()` 仅在 `VALIDATION_ERROR`(422) / `RATE_LIMITED`(429) 等安全可暴露的消息上。

- [x] **H2. `zap.NewDevelopment()` 跑生产容器** → ✅ `apps/api/cmd/server/main.go:113-116` 与 `apps/ai-worker/cmd/worker/main.go:60-63`：`GIN_MODE==debug` 才 `NewDevelopment`，否则 `NewProduction`。

- [x] **H3. `upload.go` 在 10MB 检查前已 buffer 整个 body** → ✅ `upload.go:71` handler 开头 `c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, uploadMaxBytes+4096)`。

- [x] **H4. `.env` `EXPORT_DOWNLOAD_SECRET` 是真值 64 hex** → ✅ `.env.example:49` 改为 `change-me-to-a-random-32+-char-string`（`.env` 本身不入仓）。

### 前端

- [x] **H5. `Dashboard.tsx` `window.confirm` 阻塞 + kiosk 静默删除** → ✅ `Dashboard.tsx:580` 改 Semi `Modal.confirm({...})`。

- [x] **H6. `Designer.tsx` 英文按钮文本（中文比赛穿帮）** → ✅ `"Delete tab"/"Add tab"/"Up"/"Down"` 等英文标签已全部中文化（grep 无残留）。

- [x] **H7. `Designer.tsx` 嵌套字段 key 用 `${name}-${index}`，拖拽焦点丢失** → ✅ 嵌套字段统一分配 `_draftId`（`Designer.tsx:53/206/281/345/394/400-402`），rename 不再重建。

- [x] **H8. `Queue.tsx` 三个 filter tab 没 onClick 没 handler** → ✅ 真实队列分区是 `role="tablist"` + `onClick={() => setQueueFilter(...)}` + 真计数（`submissions.length` / `humanReviewingCount` / `manualReviewCount`）；`128/47/9` 硬编码数仅在 `showingDemo` 分支（刻意的 demo 占位）。

### 交付物

- [x] **H9. Worker quickstart 漏 `cp .env.example .env`** → ✅ `submission/README.md:49-50` quickstart Step 1 已含 `cp .env.example .env`（默认 `LLM_PROVIDER=mock`）。

- [x] **H10. 未提交的 S7 `preference_compare` seed** → ✅ `tools/seed/templates/preference_compare_review.json` 已入库，`apps/api/cmd/seed/main.go:59/133-134` 引用；demo 可展示文本质检 + A/B 偏好两类任务。

---

## 🟨 Important / Medium — 影响读感，不阻塞

### 后端

- [ ] **M1. `upload.go` 文件落盘先于 DB row，崩了留孤儿文件** → ⏸ **推迟**：仍是先 `SaveUploadedFile` 再建 row，但 row 用 `temp` status + 计划 cron 清理未 attach 的孤儿（`upload.go:55-56` 注释）兜底；非阻塞。

- [ ] **M2. 所有 handler 不传 `ctx`（client 断连不取消 goroutine）** → ⏸ **推迟**：目前仅 `llm.go` 透传；其余 handler 未透传，请求级超时由上游兜。非阻塞，列为已知工程债。

- [ ] **M3. `review.Apply` 多一次没用的 probe SELECT** → ⏸ **推迟**：`review.go:127-128` probe SELECT 仍在，功能正确，仅一次多余查询，Medium 性能 nit。

### 前端

- [ ] **M4. 三大文件 >800 LOC（仓库规则上限）** → ⏸ **推迟**：`Dashboard.tsx 1912` / `Designer.tsx 2434` / `Queue.tsx 1971`（较 review 时还涨了）。拆分牵动大量竞态/状态逻辑，deadline 下回归风险高，**接受为已知工程债**。

- [x] **M5. `Dashboard.tsx` 硬编码 "官方 qa_quality 主线任务"** → ✅ `Dashboard.tsx:880` 改 `{selected.description ?? '配置标注模板与 AI 预审参数。'}`。

- [ ] **M6. 多处硬编码 hex 颜色绕 token** → ⏸ **部分**：`Designer.tsx` 的 `#e8f5e9/#fff3e0` 已清；`Queue.tsx` ScoreBars 的 `#f97316` 是 **demo 假数据数组**里的字段值（非生产渲染路径），保留。

### 安全

- [x] **M7. 无 CSP header** → ✅ `deploy/Caddyfile:12` 配置完整 CSP（`default-src 'self'` + script/style/img/font/connect/frame-ancestors/base-uri）。

- [x] **M8. Redis 没 `--requirepass`** → ✅ `docker-compose.prod.yml:24` `redis-server --requirepass`，`REDIS_PASSWORD` 注入 api/worker/asynqmon，`.env.example:22` 占位。

- [x] **M9. `LLM_API_KEY` 软默认，doubao 时启动不报错** → ✅ `pkg/llmreview/review.go:138-139`：非 mock provider 校验 `BaseURL/APIKey/Model` 缺失即返回错误（worker 启动构建 provider 时失败），line 143 未知 provider 也报错。

### 交付物

- [x] **M10. `openapi.yaml` 只覆盖主流程** → ✅ `submission/api/openapi.yaml` `info.description` 标注 "Main-flow API contract for LabelHub S8 acceptance"（扩展接口见 Postman + handler 源码）。

- [ ] **M11. Postman 单 token 不分角色** → ⏸ **部分**：collection 已按 owner / labeler / reviewer 业务流命名分组（claim / submit / queue / approve / export 全覆盖），但仍是单 owner login，未拆 3 个独立 token folder。改善但未做满，非阻塞。

- [ ] **M12. `docs/` 一堆 interim 文件** → ⏸ **推迟**：`docs/` 仍 36 个 md 未归档 `archive/`；评委已由根 README "评委友好链接" 引导只读 `submission/` + 少数 `docs/` 入口，非阻塞。

- [x] **M13. `apps/web/README.md` 还是 Vite 模板原文** → ✅ **本次修复**：改为一行指回根 README。

- [x] **M14. README 缺 CI badge + LICENSE + demo 凭据表** → ✅ 根 README 5 个 badge（CI/License/Go/React/TS）；`submission/README.md` 含三角色凭据表（owner1/labeler1/reviewer1，密码 123456）+ LICENSE 引用。

---

## 🟦 Minor — 评委大概率注意不到

- [ ] **L1. `golden_sample.go` / `ai_prompt.go` `strings.Contains(err, "duplicate")` 脆弱** → ⏸ **改善**：加了 `uk_task_payload_hash` / `uk_task_version` 约束名校验（`golden_sample.go:654` / `ai_prompt.go:430`），仍是 string-based，Low。

- [ ] **L2. `review.go:398/552` `raw, _ := json.Marshal(...)` 吞错** → ⏸ **推迟**：marshaling 已知类型 `map[string]any` 不会失败，空字符串安全，Low。

- [ ] **L3. `reviewer.go` `httpx.PageOK` 用空 `Page{}`，total 总 0** → ⏸ **部分**：`ReviewerResults` 已用复合游标真 `page`（`reviewer.go:284-313`）；`ReviewerQueue` 仍空 `Page{}`（定长 200 列表，envelope total 留空可接受）。

- [x] **L4. `main.go` `gin.Default()` 注册 Logger 中间件，prod 噪音** → ✅ `apps/api/cmd/server/main.go:59` 改 `gin.New()` 显式加中间件。

- [x] **L5. `SchemaRenderer.tsx` tab border shorthand** → ✅ 已修。

- [ ] **L6. `url.ts` `isSafeURL` SSR path 允许 `file:`** → ⏸ **接受**：运行时不可达，保留 hardening note，Low。

- [x] **L7. `upload.go:276` `Content-Disposition` 反射 `OriginalName` 含控制字符** → ✅/NA：下载头移到 `export.go:170`，`filename` 为服务端生成（非用户 `OriginalName`），原注入面消失。

- [x] **L8. `Caddyfile` 无 HSTS** → ✅ `Caddyfile:16` `Strict-Transport-Security "max-age=31536000; includeSubDomains"`。

---

## 🟩 整体亮点（保住，不要破坏）

### 后端

- Outbox publisher 用 `SELECT FOR UPDATE SKIP LOCKED` + `RowsAffected` 守 claim — 正确的并发轮询模式（`publisher.go`）。
- AI worker `complete()`/`failover()` 双锁 + `RowsAffected != 1` 守每个状态跃迁，多处一致（`ai_review.go`）。
- `pkg/llmreview/review.go` 强制 function calling（tool `submit_ai_review`、`tool_choice` 锁定、`strict:true`、维度 enum）+ 显式防注入指令 + 严格 schema/threshold 校验。
- `export.go:safeExportPath` + `mustAbsExportDir` 组合防 path traversal。

### 前端

- 0 个 `any` / 0 个 `!` non-null / 0 个 `as unknown as` cast — TS strict mode 真守。
- 序列号 + generation counter 防竞态（`loadSeq` / `taskActionGeneration` / `goldenSampleRunSeq` / `autoSaveSeq`）一致正确。
- `Plaza.tsx` 自动保存（`answerDraftKey` + `autoSaveSeq` + 防抖）textbook 实现。
- SchemaRenderer Tabs 完整 ARIA `tablist/tab/tabpanel` + auto-jump first error tab — 竞赛项目里少见的 a11y 投入。
- 题目导航定高虚拟滚动，5000 题大任务不掉帧（本轮新增）。

### 安全

- 无任何 hardcoded secret 进仓（`git ls-files` 验证）。
- 所有敏感值 `${VAR:?}` 强制（JWT_SECRET / MYSQL_PASSWORD / REDIS_PASSWORD / ASYNQMON_*）。
- JWT 算法守卫防 algorithm confusion + refresh token 服务端可撤销 + JTI 轮转。
- Asynqmon 生产路径仅 Docker 内网 + basic_auth（无 ports 映射）。
- 所有 SQL 走 `?` 参数化；零 shell exec。
- Upload magic byte 校验 + MIME 白名单排除 HTML/SVG/script，存储 path 用 random hex。
- IDOR 守卫一致（reviewer 通过 `task_reviewers`，`canReviewTask` 全路径覆盖；本轮补齐 `GET /reviewer/ai-reviews` 的 scope）。
- Markdown 走 JSX 不 `dangerouslySetInnerHTML`，URL 过 `isSafeURL`。
- Login 端点 IP token bucket 防暴力 + 防用户名时序枚举。

### 交付物

- `ARCHITECTURE.md`（3 Mermaid + 决策表）submission-grade。
- `DEPLOY.md` 完整（compose 路径、env 表、asynqmon auth、backup、verify curls）。
- conventional commit 按 S0→S8 自然分章成稿。
- CI workflow 在跑（`.github/workflows/ci.yml`）。

---

## 执行结果（替代原 Day1–4 行动清单）

初次 review 的 Day1–4 行动清单已全部执行完毕：

- **Critical（8/8）** 全修并部署。
- **High（10/10）** 全修并部署。
- **Medium（14）**：已修 M5/M7/M8/M9/M10/M13/M14；部分 M6/M11；有意推迟 M1/M2/M3/M4/M12（非阻塞工程债，原因见上）。
- **Low（8）**：已修 L4/L5/L7/L8；改善 L1/L3；接受 L2/L6。
- **第二轮 review** 额外修复 6 处（含 IDOR Critical），3 个误报经验证未改。

剩余唯一硬缺口：演示视频实录。

---

## Review 元信息

| Reviewer | 范围 | LOC | 主要发现 |
|---|---|---|---|
| go-reviewer | apps/api + worker + pkg | 15.5k | 2 Critical / 3 Important / 4 Minor |
| typescript-reviewer | apps/web | 14k | 2 Critical / 5 Important / 4 Minor |
| security-reviewer | 跨栈 auth/secrets/injection | 全栈 | 0 Critical / 4 High / 3 Medium / 4 Low |
| 交付物 readiness | submission/docs/deploy/judge UX | — | 5 Critical / 7 Important / 6 Polish |
| 第二轮 review（2026-06-10） | S7/S8 新增（labeler 大任务 / AI 队列 / acceptance） | — | 1 Critical(IDOR) + 5 修复，3 误报排除 |

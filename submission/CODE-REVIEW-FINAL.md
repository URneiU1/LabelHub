# LabelHub 交付前 Code Review 汇总

**Review 时间**：2026-05-28
**仓库**：`~/Desktop/LabelHub`
**分支**：`main`（5 个未提交 S7 改动）
**比赛 deadline**：2026-07-08 ~ 2026-07-15
**审查范围**：~31k LOC（15.5k Go + 14k TS/TSX）+ 部署配置 + 交付物
**审查方式**：4 路并行 — go-reviewer / typescript-reviewer / security-reviewer / 交付物 readiness

## 结论

**代码可发，交付包不存在**。代码层无 CRITICAL 安全漏洞，状态机/outbox/AI worker 设计扎实，TypeScript strict mode 真守，0 个 `any`。但 `submission/` 是空的，6 项官方交付物（PLAN.md line 27）一项没在，演示视频和 AI Coding PDF 也没起。

**预估 2-4 天**（基本是组装 + 录视频 + 写 PDF + 修 8 个 Critical bug，新代码极少）即可投。

---

## 🟥 Critical — 必须修，否则交不了 / 评委直接打回

### 交付物层（4 条）— 最大风险

- [ ] **C1. `submission/` 目录是空的** — PLAN.md line 27 强制要求 6 项交付物全部放这里：
  1. 源码 Monorepo
  2. README（架构 / 模块划分 / 本地启动 / 关键取舍）
  3. 演示视频 5-10 分钟（覆盖三角色完整链路）
  4. 相关文档（架构图 + 关键技术点 + Demo 截图 + AI Coding 过程记录 + 基础技术文档）
  5. 可访问演示环境说明文档（用户自行 VPS）
  6. API 文档（Postman / 飞书 / Markdown 任选）

- [ ] **C2. 演示视频 + AI Coding 8-15 页 PDF 完全没起** — PDF 原料 50 条 conventional commits 按 sprint 天然分章，需要 1-2 天写

- [ ] **C3. `.env.example:32` 占位 `/absolute/path/to/LabelHub/data/exports`** — API 启动 `mustAbsExportDir()` 会直接 fatal。评委 `cp .env.example .env` 后跑不起来
  - 修：Makefile 用 `$(PWD)/data/exports` 自动注入

- [ ] **C4. 根 README 前 187 行是 sprint 流水账 + "仍需提升" 红字** — 评委第一屏看到"AI 预审 P1 安全/状态/前端竞态问题已收敛"是公开内部 tech debt
  - 修：替换为干净的评委门面；老内容挪 `docs/CHANGELOG.md`

### 代码层（4 条）— demo 日炸车

- [ ] **C5. `apps/api/internal/handler/reviewer.go:428-435`** — `retryAIReview` 改 submission 状态时漏 `RowsAffected` 检查，并发写会静默腐败
  - 修：照 `service/review/review.go:128` 加 `ErrConcurrentWrite` 返回

- [ ] **C6. `apps/api/internal/handler/reviewer.go:157`** — `ReviewerQueue` 没 `.Limit()`，大任务会一次性 SELECT 全部 `human_reviewing`，demo OOM + 超时
  - 修：一行 `.Limit(200)`

- [ ] **C7. `apps/web/src/modules/reviewer/Queue.tsx:391-402`** — 三个决策按钮（通过/打回/修订）只 disable 在 `!schema.ok`，没 disable 在 `loading`。评委快速点击会双提交不可逆 verdict
  - 修：`disabled={loading || (!showingDemo && !schema.ok)}`

- [ ] **C8. `apps/web/src/modules/owner/Dashboard.tsx:732`** — `pollGoldenSampleRun` 递归 `setTimeout` 无 cleanup。评委切走页面后还在跑 20+ 请求
  - 修：用 `useEffect` 包装 + 记录 timer ID + return 清理

---

## 🟧 High — 评委读代码会扣工程质量分

### 后端 / 安全

- [ ] **H1. `apps/api/internal/handler/{ai_dry_run.go:77, ai_prompt.go:134, golden_sample.go:394, golden_sample.go:459}`** — `httpx.Error(c, 500, "INTERNAL_ERROR", err.Error())` 把 GORM 报错（含表名/列名/MySQL 错误码）直接吐给 owner
  - 修：返 `"internal error"` 静态字符串，服务端用 zap 记真错

- [ ] **H2. `apps/api/cmd/server/main.go:30` + `apps/ai-worker/cmd/worker/main.go:20`** — `zap.NewDevelopment()` 跑在生产容器里，stdout 全是带颜色 + 栈跟踪（含 file:line）
  - 修：`GIN_MODE=release` 时用 `zap.NewProduction()`

- [ ] **H3. `apps/api/internal/handler/upload.go:69-123`** — Gin `ParseMultipartForm(32MB)` 在 10 MB 检查前已经 buffer 完整个 body。labeler 可以并发刷 32 MB body 打 RAM
  - 修：handler 开头 `c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, uploadMaxBytes+4096)`

- [ ] **H4. `.env:45`** — `EXPORT_DOWNLOAD_SECRET=105b26a681184fc4...`（真值 64 hex），不是 `change-me-...` 风格。评委 `cat .env` 会以为是生产密钥
  - 修：换成 `change-me-64-hex-chars`，加注释 `# openssl rand -hex 32`

### 前端

- [ ] **H5. `apps/web/src/modules/owner/Dashboard.tsx:518`** — `window.confirm('删除 golden sample #N?')` 阻塞主线程 + 原生系统对话框 + kiosk Chrome 直接返 false 静默失败删除
  - 修：换 Semi `Modal.confirm()`

- [ ] **H6. `apps/web/src/modules/template/Designer.tsx:817,820,920-928`** — `"Delete tab"`, `"Add tab"`, `"Up"`, `"Down"`, `"Delete"`, `` `Add ${widget}` `` 是英文按钮文本（其他地方是 `↑↓ 复制 删除`）— 中文比赛穿帮

- [ ] **H7. `apps/web/src/modules/template/Designer.tsx:553,560,563,862`** — 嵌套字段 key 用 `${name}-${index}` 复合，rename 同时改 index 会 React 销毁重建，拖拽时焦点丢失
  - 修：嵌套字段也分配 `_draftId`

- [ ] **H8. `apps/web/src/modules/reviewer/Queue.tsx:336-338`** — 三个 filter tab 按钮（128/47/9）没 onClick 没 handler，硬编码 count。demo 模式 ok，真数据穿帮

### 交付物

- [ ] **H9. Worker 没说要 `cp .env.example .env`** — `make worker` 不复制就 fatal。`.env.example` 默认 `LLM_PROVIDER=mock`（评委不需要豆包 key），但 README quickstart 漏了这步
  - 修：README quickstart 加 Step 0 `cp .env.example .env`

- [ ] **H10. 未提交的 S7 `preference_compare` seed** — `apps/api/cmd/seed/main.go` + `tools/seed/templates/preference_compare_review.json`
  - 提交后 demo 能展示两种任务类型（文本质检 + A/B 偏好），强化"覆盖多场景"叙事

---

## 🟨 Important / Medium — 影响读感，不阻塞

### 后端

- [ ] **M1. `apps/api/internal/handler/upload.go:131-148`** — 文件落盘先于 DB row。崩了就是孤儿文件（cleaner 按 status 查找不到）
  - 修：先 INSERT temp row 再写文件

- [ ] **M2. 所有 handler 不传 `ctx`**（56 处）— `h.db.WithContext(c.Request.Context())` 没传，client 断连后 goroutine 不取消

- [ ] **M3. `apps/api/internal/service/review/review.go:76-93`** — `review.Apply` 多了一次没用的 `probe SELECT`
  - 修：去掉 probe，从 locked submission 拿 taskID

### 前端

- [ ] **M4. 三大文件 >800 LOC（仓库规则上限）** — 拆点：
  - `Dashboard.tsx` 1663 → 拆 `AIPromptPanel` ~900 行
  - `Designer.tsx` 1673 → 拆 `PropertyPanel`（含 GroupControls/TabsControls/NestedFieldsEditor/LLMTriggerControls）~600 行
  - `Queue.tsx` 1604 → 拆 `ReviewDetail`（DemoReviewDetail + RealReviewDetail + RealAIDiagnostics）~350 行

- [ ] **M5. `Dashboard.tsx:797-800`** — 硬编码 "官方 qa_quality 主线任务"，换 `selected.description`

- [ ] **M6. 多处硬编码 hex 颜色绕 token** — `Designer.tsx:336` (`#e8f5e9`, `#fff3e0`)；`Queue.tsx:585-591` ScoreBars 默认 `#f97316`；Dashboard 也有

### 安全

- [ ] **M7. 无 CSP header**（API + Caddy 都没设）— markdown 渲染走 JSX 不 `dangerouslySetInnerHTML`，风险低但补一个是 1 行 Caddy 改动
  ```
  header Content-Security-Policy "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' https: data:; media-src 'self' https:; frame-ancestors 'none'"
  ```

- [ ] **M8. `deploy/docker-compose.prod.yml:19-29`** — Redis 没 `--requirepass`。Docker bridge 同网段容器可读写 Asynq job/outbox
  - 修：加 `REDIS_PASSWORD` 到 `.env.example` + compose `command` + API/worker env

- [ ] **M9. `deploy/docker-compose.prod.yml:57,101`** — `LLM_API_KEY: ${LLM_API_KEY:-}` 软默认，doubao provider 时启动不报错只在第一次调用炸
  - 修：worker `main.go` 加 `LLM_PROVIDER != mock && LLM_API_KEY == ""` 启动校验 `log.Fatal`

### 交付物

- [ ] **M10. `docs/openapi.yaml`** 只覆盖主流程 14 个 path，AI Prompt / Golden Sample / Upload 缺
  - 修：顶部加一行 "本文档覆盖主流程，扩展接口见 Postman + `apps/api/internal/handler/*`"

- [ ] **M11. `docs/LabelHub.postman_collection.json`** 单 token 不分角色
  - 修：拆 3 个 login 写 `ownerToken/labelerToken/reviewerToken`，folder per-role auth

- [ ] **M12. `docs/` 14 个文件**（PLAN-S2/S4/S5/S6-IMPL, REVIEW_S0_S1, S1_ACCEPTANCE...）评委看到一堆 interim
  - 修：挪 `docs/archive/`，根 `docs/` 只留 ARCHITECTURE.md / DEPLOY.md / PLAN.md / openapi.yaml / postman / CODE-REVIEW-FINAL.md

- [ ] **M13. `apps/web/README.md`** 还是 Vite 模板原文 — 删或改一行指回根 README

- [ ] **M14. README 缺**：CI badge + LICENSE reference + demo 凭据表（owner1/labeler1/reviewer1 / `pass`，藏在 `cmd/seed/main.go:55`）

---

## 🟦 Minor — 评委大概率注意不到

- [ ] **L1. `apps/api/internal/handler/golden_sample.go:618-625`** — `isDuplicateGoldenSamplePayload` 走 `strings.Contains(err.Error(), "duplicate")` fallback，跨 MySQL driver 版本脆弱。同模式 `ai_prompt.go:425-432`

- [ ] **L2. `pkg/llmreview/review.go:398, 552`** — `raw, _ := json.Marshal(...)` 错误吞，应该 log 或注释为何空字符串安全

- [ ] **L3. `apps/api/internal/handler/reviewer.go:160-161`** — `httpx.PageOK` 用空 `httpx.Page{}`，envelope 总是 total=0，UX 误导

- [ ] **L4. `apps/api/cmd/server/main.go:55`** — `gin.Default()` 注册 Logger middleware，prod 噪音 + 路径泄漏
  - 修：`gin.New()` 显式加 middleware

- [ ] **L5. `apps/web/src/renderer/SchemaRenderer.tsx:221-226`** — tab border shorthand 修过了，CLAUDE.md Known Follow-Ups 可以划掉

- [ ] **L6. `apps/web/src/shared/security/url.ts:3-22`** — `isSafeURL` SSR path 允许 `file:`，运行时不可达但留个 hardening note

- [ ] **L7. `apps/api/internal/handler/upload.go:276`** — `Content-Disposition` 反射 `OriginalName` 含控制字符（CRLF stdlib 已挡，null byte 没挡）

- [ ] **L8. `deploy/Caddyfile`** — `CADDY_SITE_ADDRESS=:80` 时无 HSTS，没启动校验

---

## 🟩 整体亮点（保住，不要破坏）

### 后端

- Outbox publisher 用 `SELECT FOR UPDATE SKIP LOCKED` + `RowsAffected` 守 claim — 正确的并发轮询模式（`publisher.go:111`）
- AI worker `complete()`/`failover()` 双锁 + `RowsAffected != 1` 守每个状态跃迁，5 处一致（`ai_review.go:282,297,307,317,328`）
- `pkg/llmreview/review.go:576-583` LLM prompt 显式防注入指令 + 严格 schema/threshold 校验
- `export.go:safeExportPath` + `mustAbsExportDir` 组合防 path traversal

### 前端

- 0 个 `any` / 0 个 `!` non-null / 0 个 `as unknown as` cast — TS strict mode 真守不是 worked around
- 序列号 + generation counter 防竞态（`loadSeq` / `taskActionGeneration` / `goldenSampleRunSeq` / `autoSaveSeq`）一致正确
- `Plaza.tsx` 自动保存（`answerDraftKey` + `autoSaveSeq` + 防抖 `setTimeout` + `clearTimeout`）textbook 实现
- SchemaRenderer Tabs 完整 ARIA `tablist/tab/tabpanel` + auto-jump first error tab — 竞赛项目里少见的 a11y 投入（`SchemaRenderer.tsx:129-162`）

### 安全

- 无任何 hardcoded secret 进仓（`git ls-files` 验证）
- 所有敏感值 `${VAR:?}` 强制（JWT_SECRET, MYSQL_PASSWORD, ASYNQMON_*）
- JWT 算法守卫防 algorithm confusion + refresh token 服务端可撤销 + JTI 轮转
- Asynqmon basic_auth 正确（无 ports mapping，仅 Docker 内网）
- 所有 SQL 走 `?` 参数化，3 处 `Raw()/Exec()` 也是；零 shell exec
- Upload magic byte 校验（PNG/JPEG/PDF/WebP signature），MIME 白名单排除 HTML/SVG/script，存储 path 用 32 byte random hex（不含用户文件名）
- IDOR 守卫一致（reviewer 通过 `task_reviewers` 关联，`canReviewTask` 全路径覆盖）
- Markdown 走 JSX 不 `dangerouslySetInnerHTML`，URL 过 `isSafeURL`
- Login 端点 IP token bucket（burst 10, refill 1/6s）防暴力 + 防用户名时序枚举
- CORS 严格 exact-string origin 匹配，不带 credentials

### 交付物

- `docs/ARCHITECTURE.md`（3.4K，3 Mermaid + 决策表）已 submission-grade
- `docs/DEPLOY.md` 完整（compose 路径、env 表、asynqmon auth、backup、verify curls）
- 50 条 conventional commit 按 S0→S6 自然分章成稿
- CI workflow 在跑（`.github/workflows/ci.yml`）

---

## 📋 优先级动作清单（按时间倒推）

### Day 1（4-6h, 拆雷 + 砸 Critical 代码 bug）

- [ ] commit S7 seed（`apps/api/cmd/seed/main.go` + `tools/seed/templates/preference_compare_review.json`），验证 `make seed` 不挂
- [ ] Makefile 让 `EXPORT_DIR` 自动 `$(PWD)/data/exports`（解 C3）
- [ ] C5 `retryAIReview` 加 RowsAffected
- [ ] C6 `ReviewerQueue` 加 `.Limit(200)`
- [ ] C7 Queue 决策按钮 loading guard
- [ ] C8 `pollGoldenSampleRun` cleanup
- [ ] `.env` 改 `EXPORT_DOWNLOAD_SECRET=change-me-...`（解 H4）

### Day 2（4-6h, 安全 + 交付物组装）

- [ ] H1 三 handler raw error 脱敏
- [ ] H2 `zap.NewProduction()` 切换
- [ ] H3 `MaxBytesReader` 改 upload
- [ ] 建 `submission/`：copy `ARCHITECTURE.md` / `DEPLOY.md` / `openapi.yaml` / `postman_collection.json` / `LICENSE`
- [ ] 写 `submission/README.md`（5 步 quickstart + 3 角色凭据表 + 5 min demo script）
- [ ] 写 `submission/DEMO_SCRIPT.md`（视频 + judge walkthrough 共用）
- [ ] 根 README 重写门面，老内容挪 `docs/CHANGELOG.md`

### Day 3（6-8h, 录制 + PDF）

- [ ] 跑通 demo 流程，录 5-10 min 视频（同时截 demo 截图）
- [ ] 写 AI Coding 8-15 页 PDF（按 S0→S6 分章 + git log 提关键决策点）
- [ ] 渲染架构图 PNG：`mmdc -i docs/ARCHITECTURE.md -o architecture.png`

### Day 4（buffer / 抛光）

- [ ] H5 `window.confirm` → Semi Modal.confirm
- [ ] H6 Designer 英文标签改中文
- [ ] H7 嵌套字段 `_draftId`
- [ ] H8 / H10 真假数据展示策略对齐
- [ ] M5 Dashboard 硬编码 task description
- [ ] M11 Postman 拆 3 角色 token
- [ ] M12 `docs/archive/` 归档老 plan
- [ ] M14 README CI badge + LICENSE + demo 凭据表

---

## Review 元信息

| Reviewer | 范围 | LOC | 用时 | 主要发现 |
|---|---|---|---|---|
| go-reviewer | apps/api + worker + pkg | 15.5k | ~30 min | 2 Critical / 3 Important / 4 Minor |
| typescript-reviewer | apps/web | 14k | ~30 min | 2 Critical / 5 Important / 4 Minor |
| security-reviewer | 跨栈 auth/secrets/injection | 全栈 | ~30 min | 0 Critical / 4 High / 3 Medium / 4 Low |
| 交付物 readiness | submission/docs/deploy/judge UX | — | ~20 min | 5 Critical / 7 Important / 6 Polish |

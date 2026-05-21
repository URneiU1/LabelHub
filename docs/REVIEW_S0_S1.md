# LabelHub Code Review — Sprint 0 / Sprint 1(进行中)

**Review 时间**:2026-05-21
**Reviewer**:Claude(Opus 4.7)
**审查范围**:
- 已提交:`ba23fa2` Sprint 0 脚手架 → `c575aee` review fixes
- 未提交:migration (`001_initial_schema`) + `model/models.go` + `auth/` + `middleware/auth.go` + `handler/auth.go` + `httpx/response.go` + `cmd/seed`
**重点**:对照 `PLAN.md` §0–§7 评估**业务理解深度**

**结论**:`APPROVE WITH REQUIRED CHANGES`(主表结构对业务的吸收很深,但 §6.1 API envelope 实现走偏 + Sprint 1a 主线 handler / 状态机均未启动)

---

## 一、业务理解深度评估(用户重点关切)

### 1.1 抓到了 PLAN 里那些"反直觉"的业务设计 ✓✓

挑出 5 个最容易被忽视、但 migration / model 全部正确落地的点 —— 这部分**业务理解到位**:

| 业务洞察(PLAN 来源) | 实现位置 | 评价 |
|---|---|---|
| **`ai_verdict` 是字段不是状态**(§4.2)—— 这是 PLAN 里反复强调的"状态 vs 字段拆分"亮点 | `submissions.ai_verdict ENUM NULL` 独立字段;`submissions.status` 严格 7 态 enum,没出现 `ai_passed` 这种合并态 | ✓ 抓到了关键 |
| **`human_verdict` 是 denormalized snapshot,真相来源是 `human_reviews`**(§2 关键易忘点) | migration L132 SQL `COMMENT` 原话写出"source of truth is human_reviews table" | ✓ 把易忘点变成 schema 内自文档 |
| **`golden_samples` 独立表(不嵌 tasks)+ `payload_hash` 防重**(§2 新增表) | 独立表 `golden_samples` 含 `payload_hash CHAR(64)` + UK(task_id, payload_hash) | ✓ 没走"省事塞 JSON 字段"的弯路 |
| **`audit_logs.created_at TIMESTAMP(3)` 毫秒精度**(§2 复合索引一行带过) | `TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)` + `INDEX(entity_type, entity_id, created_at)` | ✓ 毫秒精度细节没漏 |
| **`user_roles` 多对多 + `owner1` seed 同时拿 `owner` + `reviewer`**(§2"Owner 兼 Reviewer 必须支持") | `cmd/seed/main.go:25` `roles: []string{"owner","reviewer"}` | ✓ 不是表设计对就行,**seed 数据真的演示了 overlap** |

**额外加分点**:
- `system` 角色在 `user_roles.role` enum 内 + `system_ai` 用户被实际 seed,呼应 §1.1"AI Agent 有独立 system 账户视角" —— 答辩讲解可直接指着 audit_logs 里 `actor_type='ai_worker' / actor_id=system_ai.id` 的记录讲。
- `ai_reviews.idempotency_key UNIQUE` 落地了 §5.3 幂等设计 —— 即使 Sprint 3 的 worker 还没写,**DB 层兜底防御已就位**。
- `outbox_events` 表已建,§4.5 单库 outbox 一致性方案有 schema 支撑。

### 1.2 业务理解上的盲点 / 走偏

#### B1. **`task_items.status` enum 是双数据源陷阱(HIGH)**

migration L92:
```sql
status ENUM('pending','claimed','submitted','approved','rejected','revising','archived') NOT NULL DEFAULT 'pending'
```

PLAN §2 对 `task_items` 只说 `status ENUM`,**没有定义具体值**。这里给出的 7 个值**几乎复用了 `submissions.status`**,这是危险信号:
- `submissions` 已经有完整 7 态状态机
- 如果 `task_items` 也有自己的生命周期,那"一个 item 已被 submit + approved",**谁负责更新 `task_items.status`?** PLAN §4 状态机表完全没提 item 状态迁移
- 一旦两边不同步(submission 已 approved 但 item 还在 claimed),Dashboard / 抢单 / 重新发放逻辑都会错乱

**建议**:`task_items.status` **只保留 `available / claimed / finished / archived`** 三态(或干脆删掉 status,用 `claimed_by IS NULL` + `EXISTS (SELECT 1 FROM submissions WHERE item_id=...AND status IN ('approved','rejected'))` 派生)。`archived` 没有业务场景(item 不会被独立 archive)—— 也建议删。

**对答辩的影响**:评委一问"submission approved 之后 task_items 这边怎么同步?"如果说"也要同步过去",就暴露了 dual source of truth;如果说"不同步",这个 enum 就是误导。**Sprint 1a 第一个 PR 改掉,migration 还没跑过没成本**。

#### B2. **`response.go` 不符合 PLAN §6.1 envelope 契约(HIGH)**

PLAN §6.1 第一条规则原话:
> **统一响应 envelope**:
> - 成功:`{"data": ..., "request_id": "..."}`
> - 失败:`{"error": {"code": "...", "message": "...", "details": {...}}, "request_id": "..."}`
>
> **错误码从 Sprint 1 开始定义,禁止前 4 个 Sprint handler 返回 ad-hoc JSON**

`httpx/response.go` 现状:
```go
func OK(c, data) { c.JSON(200, gin.H{"data": data}) }                  // 缺 request_id
func Error(c, status, code, message) { c.JSON(status, ErrorResponse{Code, Message}) }
// → 直接 flat {"code":"...","message":"..."},不是 {"error":{...},"request_id":"..."}
```

**和 PLAN 完全不符**。PLAN 明确说"禁止前 4 个 Sprint handler 返回 ad-hoc JSON",而现在的 `Error()` 返回的**就是 ad-hoc 形态**。前端 + 后续 handler 一旦按这个错的契约写出去,改回标准 envelope 就要**改所有 handler + 所有前端 fetch 拆包逻辑**。

**业务影响**:这是 Sprint 1 最大的"对 PLAN 反向理解"miss。**评委如果对 API 一致性扣分**,失分点就在这。

**修复**:
```go
type Envelope struct { Data any `json:"data,omitempty"`; Error *Err `json:"error,omitempty"`; RequestID string `json:"request_id"` }
type Err struct { Code string `json:"code"`; Message string `json:"message"`; Details any `json:"details,omitempty"` }
```
配合 `request_id` middleware(下条)。**今天改成本最低,Sprint 1a handler 还没批量写**。

#### B3. **`request_id` middleware 完全没装(HIGH)**

PLAN §6.1 原话:"**request_id middleware 从第一天落地**"。

`cmd/server/main.go` 只挂了 `gin.Default()` + `middleware.Auth`,**没有 request_id middleware**。`httpx.OK / Error` 也没办法吐 request_id 因为 context 里就没有。

**业务影响**:答辩讲"工程质量 25%"的可观测性时,无 request_id = 日志关联不上、前端排错无法定位后端 trace。Sprint 5 工程质量评分时,这是看得见的扣分项。

#### B4. **`Logout` 是 no-op,但叫 Logout(MEDIUM)**

`handler/auth.go:106` `Logout` 只返回 `{"ok": true}`,**不写 DB、不黑名单、不删 refresh token**。JWT 是无状态的 → 用户"登出"之后,他的 access token 还能用 2h、refresh token 还能用 14d。

PLAN 没明确要求 refresh token 黑名单(MVP 可以接受),但**叫 `Logout` 容易误导**:
- UI 显示"已退出",用户以为安全了
- 实际泄露的 token 仍在有效期内可用

**业务影响**:答辩"安全与机密管理"那 part(§0.1)被问到"那如果 refresh token 泄露呢"会卡住。

**修复**(三选一,按 MVP 接受度排序):
1. **最小成本**:把 `Logout` 改名为"前端清本地 token"的提示性 endpoint,文档明确"JWT 无服务端登出,需等待过期"
2. **简单方案**:加 `revoked_tokens` 表(jti UNIQUE + expires_at INDEX),Logout 写一条;Parse 时查;每天 cron 清过期
3. **彻底方案**:refresh token 改为 DB 持久化(`refresh_tokens` 表 token_hash + expires_at + revoked_at),Logout 标 revoked

**建议**:Sprint 1 不动它,但**改名 + 加 README 说明**当前语义。Sprint 5 工程质量收尾时补方案 2。

#### B5. **`distribution='assigned'` 下 `task_assignees` UK 允许多人同一 item(LOW)**

`task_assignees UK(task_id, user_id, item_id)`。语义上 `assigned` 模式下一个 item 该只能分给一个 labeler,但当前 UK 没禁止 user_A + user_B 都被 assign 到同一 item_id。PLAN §2 也没明说,可能 Sprint 4 用 quota 分发时反而需要这种灵活性 —— 留个 TODO 等 Sprint 4 落 quota 时确认。

#### B6. **`exports.format` 没有 `markdown`(LOW)**

PLAN §0 加分项第二条"Markdown 导出器(张满宣讲会强调对接模型训练)"是 ★★★ 优先级。当前 `format ENUM('json','jsonl','csv','xlsx')`,Sprint 6 加 markdown 必须 migration。能预留就预留 —— `ENUM('json','jsonl','csv','xlsx','markdown')`,加分项打开门。

#### B7. **`finished_items` 无原子更新约束 / 无对账机制(MEDIUM)**

`tasks.finished_items INT NOT NULL DEFAULT 0` 是冗余计数器。PLAN §6 提到 Dashboard 走 `GET /tasks/:id/dashboard`,会读这个字段。但:
- 没有 trigger
- 没有 SQL CHECK 约束
- 必须靠 app 代码在 submission 转 `approved` 的同一事务 `UPDATE tasks SET finished_items = finished_items + 1`

**风险**:一旦哪个 handler 漏写、或并发下事务隔离不够,**Dashboard 数字会偏**。

**建议**(Sprint 1a 写 submission handler 时):
- 同事务 `UPDATE tasks SET finished_items = (SELECT COUNT(*) FROM submissions WHERE task_id=? AND status='approved') WHERE id=?`(子查询而非 +1,避免漂移)
- 或者干脆**别存 finished_items**,Dashboard 读时 `JOIN COUNT(*)` —— 任务量不大 30 题级别完全够用

---

## 二、Sprint 1 完成度 vs PLAN 计划

Sprint 1a 在 PLAN §7 列出的 deliverables 对照:

| Sprint 1a 项目 | 状态 |
|---|---|
| 17 张表 migration | ✓ 全做完 |
| JWT 鉴权(login/refresh/me/logout) | ✓ 4 个 endpoint 全有 |
| user_roles 多对多 | ✓ |
| 角色守卫 middleware (`RequireRoles`) | ✓ |
| **统一 API envelope / 错误码 / CORS / request_id middleware** | ✗ envelope 错的,**没 request_id**,**没 CORS**(虽然 dev 走 Vite proxy 可以拖) |
| **7 态状态机引擎** `statemachine/machine.go` + **100% 单测** | ✗ `internal/statemachine/` 空目录 |
| audit_log 写入 / human_reviews 与 submissions.human_verdict 同事务更新 | ✗ 表有了,没 service 代码 |
| **API 最小集**:tasks 只读/创建、items JSON 导入、`first_come` claim(行锁)、draft、submit、review、upload、`/me/submissions`、同步 JSON export | ✗ **全部未启动** |
| `/llm/inline` mock 返回 | ✗ 未启动 |
| **官方 qa_quality 主线 seed**(30 条 + 模板 JSON + baseline) | ✗ 只 seed 了 5 个 user,没 seed task 和 qa_quality 数据 |
| Seed 账号 2 owner + 3 labeler + 2 reviewer(1 个 Owner 兼 Reviewer) | △ 部分 —— 当前 1 owner + 1 labeler + 1 reviewer + 1 admin + 1 system,**owner1 兼 reviewer 这个关键点对了**,但人数不够;Sprint 1b 前端做"多 labeler 抢单"演示时会缺 labeler |

**Sprint 1a 完成度估算:~25%**(基础设施 + auth + 表结构已就位,**业务核心 0%**)。这跟 PLAN §7 "Sprint 1a 前 5 天"的预期严重不符 —— 如果按 5 天估,现在应该到 day 2-3 的进度。

**用户应注意**:这是个**前段非常扎实、但还没真正开干业务**的状态。下个 PR 必须开始落状态机 + tasks/items/submission handler,不然 10 天 Sprint 1 时间表会破。

---

## 三、通用代码质量(摘要)

### CRITICAL — 无

### HIGH

- **B1 / B2 / B3**(见上,业务理解)
- **JWT secret 有 dev default fallback,无生产校验**(`auth/service.go:45`)。`labelhub-dev-secret-change-me` 是 enumeration risk —— 任何拿到二进制的人都知道默认值。建议:`if os.Getenv("JWT_SECRET") == "" { log.Fatal("JWT_SECRET 必须设置") }`,**禁止 default**。
- **没有 token 过期测试**(`auth/service_test.go`)。`Parse` 依赖 jwt 库做 expiry 校验,但**没有测试验证"过期 token 被拒"** —— PLAN/rules"关键路径 100% 单测",auth 是关键路径,这条覆盖不能漏。再加一条:`alg: none` 攻击防御已经在 `Parse` 里写了(`SigningMethodHMAC` 类型断言),但没测试。

### MEDIUM

- **`AuthHandler` 直接持 `*gorm.DB`**,违反 PLAN 隐含的"Service / Repo 分层"(internal 目录已有 `repo/` `service/` 空文件夹,说明设计上就要分)。**现在只有 auth 一个 handler 还不痛,Sprint 1a 加完 8 个 handler 就来不及拆**。建议下个 PR 引入 `UserRepository` 接口。
- **`Logout` no-op + 命名误导**(B4)
- **`finished_items` 无原子保证**(B7)
- **`cmd/seed` 没有生产环境保护**:任何人 `make seed` 都会向当前 `DB_HOST` 的库写 5 个用户(`pass` 弱密码)。如果 `.env` 误指向 prod → 数据污染 + 弱密码用户进 prod。建议:`if os.Getenv("LABELHUB_ENV") != "dev" { log.Fatal("seed 只能在 dev 跑") }`。
- **`main.go:43` 端口硬编码 `":8080"`**,但 `.env.example` 定义了 `API_PORT=8080` 不被读 —— 配置不一致。

### LOW

- `Login.tsx` 用 `<a href>` 跳路由,造成 SPA 全页刷新。改 react-router `<Link>`。
- `task_items.status='archived'`(B1 一部分)
- `audit_logs.entity_type` ENUM 闭合,未来加 `template` / `prompt_config` 审计要 migration(B5 之外另一处)
- `db.Init()` 没 conn pool 限制 `SetMaxOpenConns / SetMaxIdleConns`(Sprint 5 上线前补)
- `cmd/seed` 每次 rerun 都 `bcrypt.GenerateFromPassword`,即使 user 已存在 —— 浪费 CPU(idempotent rerun 时跳过 hash)

---

## 四、Validation Results

| Check | Result | Note |
|---|---|---|
| `go vet ./...` (api) | ✓ Pass | |
| `go vet ./...` (ai-worker) | ✓ Pass | |
| `go build ./...` | ✓ Pass | api + worker 都过 |
| `go test -race ./...` | ✓ Pass | `auth` 包 3.144s,其他无 test |
| `pnpm tsc --noEmit` | ✓ Pass | |
| `pnpm build` | ✓ Pass | lottie eval 警告来自 Semi Design 间接依赖,非自家代码 |
| 测试覆盖率 | △ | `auth` 只 2 个用例,**漏过期/`alg:none`/wrong-secret 校验**。rules 要求 80% 整体 + 关键路径 100%,当前距离很远(handler / middleware 完全无 test) |

---

## 五、必须 / 建议改动清单

### 合并下个 PR 前必改(HIGH)
1. **`httpx/response.go` 改成 PLAN §6.1 envelope 契约**(`{data,request_id}` / `{error:{code,message,details},request_id}`)
2. **加 `request_id` middleware**(uuid v4 或 ULID;响应 header 也带上)
3. **`task_items.status` 收敛为 `available/claimed/finished`** 三态(migration 还没跑可以原地改)
4. **JWT_SECRET 删除 dev default,启动校验**
5. **`auth` 包加过期 + 错签名密钥 + `alg:none` 用例**

### 启动 Sprint 1a 业务部分前补(HIGH-MEDIUM)
6. 引入 `UserRepository` 接口拆 handler / DB
7. `cmd/seed` 加 `LABELHUB_ENV=dev` 守卫;补 1 owner + 2 labeler + 1 reviewer 让人数对得上 PLAN
8. `Logout` 改名或加 revocation 表(选 B4 方案 1 或 2)

### 可推到 Sprint 5(LOW)
9. CORS middleware(prod);conn pool 限制;Login.tsx 改 `<Link>`;`exports.format` 加 markdown;`audit_logs.entity_type` 改 VARCHAR 或扩 enum

---

## 六、推荐(answering "业务理解深度")

**总评:7/10**。

- migration 抓住了 PLAN 里**最容易被忽视的 5 个反直觉点**(verdict 拆字段、human_verdict denormalized、golden 独立表、ms 精度审计、角色 overlap),seed 数据**还真的演示了** Owner 兼 Reviewer 这一点 —— 这是真的读懂了 PLAN 不是顺手抄表头。
- 但 §6.1 API envelope 直接走偏,而且这是 PLAN 里**最显眼、用粗体写"禁止 ad-hoc JSON"**的章节。这个 miss 说明**第一波读 PLAN 时跳过了 API 契约细节**,只看了数据模型部分。
- Sprint 1a 业务逻辑(状态机 / handler / qa_quality seed)未启动,所以"业务理解是否能在代码里跑起来"这部分**还无法验证** —— 当前是"schema 层显示出懂业务,但应用层是空的"。

**给评委讲故事时的可用 talking points**:
1. `submissions.ai_verdict` 是字段不是状态 → 直接展示一条 `status='human_reviewing' AND ai_verdict='uncertain'` 的记录,讲"状态 = 等谁做事,字段 = 事情做完是什么结论"(PLAN §4.4 原话)
2. `golden_samples.payload_hash` 防重 → 答辩展示 dry-run 列表里"同一题不会被重复跑两遍"
3. `owner1` 同时拿 owner + reviewer 角色 → 演示"任务负责人自己当审核员"的 corner case

**给评委讲故事时**绝对不能讲的当前 talking points:
1. 不要说"我们有统一 API envelope" —— 现在的不符合自己写的 PLAN
2. 不要说"我们有完整可观测性" —— request_id 还没接

---

## Files Reviewed(unstaged + recent commits)

| File | Type |
|---|---|
| `apps/api/internal/migration/001_initial_schema.up.sql` | New |
| `apps/api/internal/migration/001_initial_schema.down.sql` | New |
| `apps/api/internal/model/models.go` | New |
| `apps/api/internal/auth/service.go` + `service_test.go` | New |
| `apps/api/internal/handler/auth.go` | New |
| `apps/api/internal/middleware/auth.go` | New |
| `apps/api/internal/httpx/response.go` | New |
| `apps/api/internal/db/db.go` | Modified(加了 migrate) |
| `apps/api/cmd/seed/main.go` | New |
| `apps/api/cmd/server/main.go` | Modified(挂 auth handler + zap logger) |
| `apps/ai-worker/cmd/worker/main.go` | Existing(S0) |
| `apps/web/src/{App,main,modules/*,shared/*,styles/*}.tsx/css` | Existing(S0 style guide + 占位路由) |
| `deploy/docker-compose.yml` | Modified |
| `.env.example` / `Makefile` / `README.md` | Modified |


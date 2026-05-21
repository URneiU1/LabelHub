# LabelHub Code Review — Sprint 1(主线 MVP)

**Review 时间**:2026-05-21
**Reviewer**:Claude(Opus 4.7 1M)
**审查范围**:Sprint 1 增量 —— 状态机引擎 / API envelope / request_id middleware / CORS / pagination / 全套 S1 handler / qa_quality seed / 前端三角色页 / ShowItem 多媒体渲染
**重点**:对照 `PLAN.md` §1–§7 评估**业务理解深度**

**结论**:`REQUEST CHANGES` —— 业务模型理解相比 S0 又深了一档,**状态机 + 双事务一致性 + first_come `FOR UPDATE SKIP LOCKED` + qa_quality 端到端** 4 个 PLAN 难点真的落到了代码;但有 1 个 **CRITICAL**(`task_items.status` enum 与代码常量不匹配)+ 1 个 **HIGH**(upload 文件实际没落盘)会让主线 demo 第一次跑就挂

---

## 一、业务理解深度评估(用户重点关切)

### 1.1 S1 相比 S0 深入的 8 个亮点 ✓✓

S0 review 评 **7/10**,主要是 schema 层抓住业务但 application 层是空的。S1 把空层填上了,且代码里能看见对 PLAN 的反向理解:

| 业务点(PLAN 出处) | 代码落地位置 | 评价 |
|---|---|---|
| **7 态状态机引擎,迁移表 100% 单测**(§4.3 + §7 Sprint 1a) | `statemachine/machine.go` 10 条迁移 + `machine_test.go` 三类测试(合法 11 条 / 非法 5 条 / 数量校验 10) | ✓ 数量、target 多分支(`ai_done → [approved, human_reviewing]`)、表驱动都对上了 |
| **`first_come` claim 行锁 + SKIP LOCKED**(§4.5"first_come 抢单并发策略") | `s1.go:194` `clause.Locking{Strength:"UPDATE", Options:"SKIP LOCKED"}` + `ORDER BY id LIMIT 1` | ✓ 不是糊弄写一个 SELECT+UPDATE,**真的用了 PLAN 推荐的 SKIP LOCKED 方案** |
| **`saveRevision` 单库 outbox 单事务**(§4.5"单库 outbox 模式") | `s1.go:443-518` 同事务:create revision → update submission.status → INSERT outbox → INSERT audit_log | ✓ revision / status / outbox / audit 全在一个 `tx` 里,这是 PLAN §4.5 最核心的一致性要求 |
| **revising → submit 清空 ai_verdict / ai_score / human_verdict 重走**(§4.3) | `s1.go:491-493` `updates["ai_verdict"]=nil; ai_score=nil; human_verdict=nil` | ✓ PLAN 表格"新 revision_no,清空 ai_verdict 重走"那一行 —— 真的清空了,而且是三字段一起清。 |
| **审核同事务**:create HumanReview + update Submission.human_verdict + 把 task_items 标 finished + bump finished_items + audit_log(§2"human_verdict 必须在同一事务内更新") | `s1.go:292-324` 全部包在 `tx.Transaction(...)` | ✓ 上次 S0 review 提的 B7"finished_items 无原子保证" —— **修了**,在 approved 分支里同事务 `finished_items + 1` |
| **`/llm/inline` Sprint 1 用 mock 返回 + 模拟 800ms 延迟**(§7 Sprint 1a) | `s1.go:380-386` `time.Sleep(800ms) + Mock 文本` | ✓ 这条很微 — 但 PLAN 原文是"根据 prompt 长度返回 fixed string + 模拟 800ms 延迟",**800ms 这个数字是 PLAN 里的**,代码硬抄了进去说明 PLAN 真的被仔细读过 |
| **qa_quality 主线 seed 端到端**(§7 Sprint 1a"官方 qa_quality 主线 seed") | `cmd/seed/main.go:100-186` 读 `tools/seed/datasets/qa_quality/标注要求.md` 作 baseline + `tools/seed/templates/qa_quality_review.json` 作 schema(v1) + 30 条 items 全导入 + `total_items=30` | ✓ baseline / template / 30 题三件事都做了,而且 `baseline_description` 来源是 PLAN 指定的"标注要求.md 摘要" |
| **API envelope + request_id + CORS 真按 PLAN §6.1 落地** | `httpx/response.go` 用 `{data,request_id}` / `{error:{code,message,details},request_id}`;`middleware/request_id.go` 用 crypto/rand 16 字节 hex + `X-Request-Id` header roundtrip;`middleware/cors.go` allow-list 解析 + Origin 校验 + OPTIONS 204 | ✓ S0 review 提的 **B2/B3 都修了**,前端 client.ts 也按这个 envelope 解 |

**额外加分点**:
- `qa_quality_review.json` 模板里 **9 个核心物料全齐**(ShowItem / Radio×4 / Tags / Input / TextArea / RichText / JSONEditor / FileUpload / LLMTrigger),完全对应 PLAN §0"必拿基线 Designer 物料 9 个核心";Sprint 2 Designer 的目标"拖拽重建这份 schema"已经有了对照标的。
- `ShowItem.tsx` polymorphic 渲染了 `text / video / image / markdown` 4 种 mediaType 分支(PLAN §3.1 列了 5 种,这里覆盖 4 种,JSON object 兜底走 valueBlock 文本展示)—— 主流路径已通。
- seed 用户数补足到 **2 owner + 3 labeler + 2 reviewer + admin + system_ai**,**owner1 仍然拿 owner + reviewer 双角色**(S0 已抓住的双角色重叠 talking point 保留),3 个 labeler 也支持了"多人抢单"的 first_come 演示场景 —— PLAN 对 seed 账号的要求**逐字符达到**。
- `Plaza.tsx` 把"已 claimed 但未完成"的 item 自动 re-load 给当前 labeler(`s1.go:183` `claimed_by = ? AND status = 'claimed'` 优先于抢新题)—— 这是答辩"刷新页面不丢进度"的隐性需求,**主动想到了**。
- audit_log 在 ReviewSubmission / saveRevision 两个关键路径都 INSERT 了,answers PLAN §2"audit_logs 核心追溯表,所有状态迁移必须写一条"。

### 1.2 业务理解上的盲点 / 反向理解走偏

#### B1. **`task_items.status` enum 与代码常量不匹配(CRITICAL — 会让主线 demo 跑不起来)**

`migration/001_initial_schema.up.sql:92`:
```sql
status ENUM('pending','claimed','submitted','approved','rejected','revising','archived') NOT NULL DEFAULT 'pending'
```

`handler/s1.go:25-29` 定义并使用:
```go
itemStatusAvailable = "available"  // ← 不在 enum 内
itemStatusClaimed   = "claimed"    // ← OK
itemStatusFinished  = "finished"   // ← 不在 enum 内
```

调用点:
- `cmd/seed/main.go:174` 导入 qa_quality 30 题时 `Status: "available"`
- `s1.go:152, 202` 导入 + 抢单都用 `"available" / "claimed"`
- `s1.go:312-313` 审核完写 `"finished"`

MySQL 8 默认 sql_mode 含 `STRICT_TRANS_TABLES`(docker-compose 也没 override sql_mode)→ **每条 `INSERT task_items` 都会以 `ER_DATA_TRUNCATED` 失败**,seed 第一题就挂,Sprint 1 主线 demo 跑不起来。

**根因**:S0 review 的 B1 同时建议两件事:
- (A) **要么改 enum**:`('available','claimed','finished')`
- (B) **要么改代码常量**:`available→pending`,`finished→approved/rejected/submitted` 派生

S1 实际做了 **(B) 的常量定义但没改 enum**,**两个数据源现在直接打架**。需要立刻二选一:
- **推荐 (A)**:写一个 migration `002_*.sql`:`ALTER TABLE task_items MODIFY status ENUM('available','claimed','finished') NOT NULL DEFAULT 'available'`,把 finished 当复合终态(approved/rejected 派生)。
- **次选**:不动 enum,改 const 为 `pending/claimed/submitted/approved/rejected/revising/archived`,接受 dual source of truth 风险。

**怎么没被发现**:本地大概率还没真的跑过 `make seed` 验证 30 题端到端;或者跑过但 MySQL sql_mode 被改成非严格了。**Sprint 1 完成标准是"屏幕录制 3 分钟无报错"**,这条不修,完成标准过不了。

#### B2. **Upload handler 元数据写了,文件没落盘(HIGH — demo 即翻车)**

`s1.go:388-415`:
```go
file, err := c.FormFile("file")  // 拿到 multipart header
// ... 完全没调用 c.SaveUploadedFile(file, dest) ...
uploaded := model.UploadedFile{
    StorageKey: storageKey(file.Filename),  // 只是 sha256(filename+ns)
    ...
}
h.db.Create(&uploaded)  // 只 INSERT 元数据
```

前端 Plaza.tsx 显示"已上传 xxx.png",但磁盘上没文件 —— demo 时评委如果点预览/下载,**404**。

PLAN §3.1 明确"上传走 `POST /api/v1/uploads`,限制 `maxSize=10MB`(图片 5MB),MIME 白名单";即使按 Sprint 1 砍项表第 1 条"FileUpload 只保后端接口 + 前端字段保存 file id,预览推到 Sprint 2",**"保存接口"也是要真的保存文件的**,不能只 INSERT metadata。

**修复**:
1. `UPLOAD_DIR=./data/uploads` 已经在 `.env.example` 定义,读出来 + `c.SaveUploadedFile(file, filepath.Join(uploadDir, taskID, storageKey+ext))`
2. MIME 白名单 + size 上限 同时补(PLAN 砍项表第 1 条没砍 MIME / size 校验,只砍预览)
3. `storageKey` 应该加文件后缀(当前是纯 hash 没扩展名,下载时 MIME 推断会乱)

#### B3. **ReviewSubmission 没强制 reject/revise 时 reason 必填(HIGH)**

PLAN Sprint 1 §7 "联动/校验只做主线必需:必填、长度、**打回时详细评语必填**"。前端 Plaza.tsx 标注员 `comment.trim().length >= 10` 校验过了,但:

- Reviewer 前端 `Queue.tsx:17` 默认 reason=`"人工复核通过"`,空 reason 可以直接按"打回修改"按钮
- 后端 `s1.go:267-275` 只校验 `verdict` 在 `approve/reject/revise` 三选一,**reason 完全没校验**
- 一条 `reject` / `revise` 的 audit_log payload 是 `{"reason": ""}` —— answer 拿到的 reason 字段也是空 —— labeler 不知道为什么被打回

PLAN 强调"reviewer 写 reason 让 labeler 看上一轮意见"是核心修订流逻辑(§7 Sprint 1a "上一轮意见"),没 reason 整条链断。

**修复**:`s1.go:271` 拿到 verdict 之后,如果 `verdict in (reject, revise) && req.Reason == ""` → 400 VALIDATION_ERROR。

#### B4. **revising → submitted → ai_reviewing/human_reviewing 跳 2 态只写 1 条 audit_log(MEDIUM)**

`s1.go:467-516` 当 labeler 在 `revising` 状态 submit 新 revision:
1. `Apply(revising, submit, submitted)` — 第一步迁移
2. 立刻 `Apply(submitted, enqueue/skip_ai, ai_reviewing/human_reviewing)` — 第二步迁移
3. 最后 audit_log 只写了一条:`from=revising, to=human_reviewing`(L513)

**问题**:`submitted` 这个**中间态**没写 audit_log。PLAN §11 验证方案隐含"audit_log 覆盖所有迁移",答辩讲"完整审计轨迹"时 timeline 上会缺 `submitted` 那一节,讲不通"状态在 5/21 14:33 经过 submitted"。

**修复**:`saveRevision` submit 分支写 2 条 audit_log(`submit` 事件 + `enqueue/skip_ai` 事件),都在同事务里。

#### B5. **ReviewerQueue `status` query 不校验,可被任意覆盖(MEDIUM 信息泄露)**

`s1.go:252` `status := c.DefaultQuery("status", "human_reviewing")` —— 任何 reviewer 角色用户可以 `GET /api/v1/reviewer/submissions?status=draft` 看到**全部 labeler 草稿**;`?status=approved` 看历史。

PLAN §6 写"`GET /reviews/queue`",**没说要支持任意 status query**。草稿、submitted 中间态、approved/rejected 终态 都不该出现在审核队列。

**修复**:白名单 query —— 只允许 `human_reviewing`(默认)+ 可选 `revising`,其他直接 403。

#### B6. **`ExportJSON include_reviews=true` 但 SELECT 没真 join reviews(MEDIUM)**

`s1.go:370` 写入 `model.Export{IncludeReviews: true, ...}`,但 L347-352 的 SELECT JOIN 只有 `submissions + task_items + submission_revisions`,**没有 `ai_reviews` / `human_reviews` JOIN**。落到 `exports.include_reviews` 是 true,实际响应 JSON 里没 reviews。**字段值与实际行为撒谎**,答辩讲"我们的导出支持包含审核记录"会被验证翻车。

**修复**:`/tasks/:id/export/json?include_reviews=true` 走另一个 SELECT,LEFT JOIN ai_reviews + human_reviews,前端按需展示。MVP 范围下 `include_reviews=false` 默认值更安全 —— Sprint 4 真做 4 格式导出再上 include_reviews。

#### B7. **Owner `GET /tasks` 不过滤 `owner_id`(MEDIUM)**

`s1.go:85-92` `ListTasks` 没 WHERE owner_id —— Owner A 看到 Owner B 的所有任务。

PLAN §6 "Task | GET /tasks ... | Owner 全套" 隐含 Owner 看自己任务。当前**admin 与 owner 共用同一 handler 但权限边界没分**。

**修复**:`claims.Roles` 包含 `admin` 时 `SELECT *`;包含 `owner` 但不含 admin 时 `WHERE owner_id = ?`。

#### B8. **`sql.NullString` 透传到前端 JSON 是泄露内部结构(MEDIUM)**

`model.Task.Description sql.NullString` → Go json encoding → `{"description":{"String":"...","Valid":true}}`。

`client.ts:86` 跟着写 `description?: { String: string, Valid: boolean }`,Owner Dashboard L65 `selected.baselineDescription?.String` —— **每个前端用 description 字段的地方都得写 `.String` 兜底**。这是**业务 contract 漏了 Go database/sql 内部结构**。

**修复**:model 上加 JSON marshaler,把 `NullString` 拆成 `*string`(null when invalid,string when valid):
```go
func (t Task) MarshalJSON() ([]byte, error) { ... description: nullStringToPtr(t.Description) ... }
```
Sprint 1 改 cost 低,Sprint 2 之后 handler 增多到 8+,改一遍前端 + 后端就要翻倍。

#### B9. **`Logout` 仍是 no-op + `JWT_SECRET` 仍有 dev default(HIGH/HIGH,**重复 S0**)**

S0 review B4 + C13 没改。`auth/service.go:45` 默认 `labelhub-dev-secret-change-me`;`handler/auth.go:106-108` Logout 直接返回 `{ok:true}`。已经在 S0 报告里详述,这里只标记**未修**。

---

## 二、其他代码质量问题

### CRITICAL
- **B1 enum 不匹配** —— 第一次跑 seed 就挂

### HIGH
- **B2 upload 不落盘** —— demo 即翻车
- **B3 reject/revise reason 必填校验缺失**
- **测试覆盖不足**:`handler/s1.go` 673 行 **0 测试**。关键路径:`first_come` 抢单的 SKIP LOCKED 行为、`saveRevision` 跨态迁移、`ReviewSubmission` 同事务 5 件事、`ExportJSON` JOIN。PLAN/rules 要求"关键路径 100% 单测",当前覆盖在状态机 + middleware + auth,**handler 层完全裸跑**。建议至少加:
  1. `TestClaimItem_PicksOldestAvailableUnderConcurrency`(用 `t.Parallel()` 起两个 goroutine 同时 claim,只能一个拿到)
  2. `TestSubmitFromRevisingClearsAIVerdict`(seed 一个 revising state,submit 后 verify `ai_verdict IS NULL`)
  3. `TestReviewApproveBumpsFinishedItems`(approve 后 `tasks.finished_items` 必须 +1)
- **`Logout` no-op** + **`JWT_SECRET` 有 dev default**(B9,S0 已提)

### MEDIUM
- **B4 跳 2 态只 1 audit_log**
- **B5 ReviewerQueue 任意 status query**
- **B6 include_reviews 真不 include**
- **B7 ListTasks 不按 owner 过滤**
- **B8 `sql.NullString` 透传**
- **`client.ts` 不处理 401**:token 过期 → Toast 失败 → 用户不知道要重登。至少要 401 → clearToken + navigate `/auth/login`
- **`InlineLLM` 用 `time.Sleep`** 阻塞 goroutine + 不接 `c.Request.Context()`,client cancel 时 server 还在 sleep
- **Plaza.tsx 没 auto-save**(PLAN §6 写 3s debounce),目前是手动按"保存草稿"
- **`task_items.payload` 双重编码**:DB 存 JSON 字符串,前端 `JSON.parse` 再用 —— `model.TaskItem.Payload` 改成 `json.RawMessage` 后前端就不用 parse 第二次

### LOW
- `ExportJSON` SELECT 无分页(MVP 范围 30 题可接受)
- `Upload` MIME 白名单 + size 上限缺失(PLAN §3.1 明确;Sprint 1 砍项没砍校验只砍预览)
- `outbox_events` 没 publisher goroutine,INSERT 后没人消费(Sprint 3 再写也 OK)
- `client.ts` token 存 localStorage(MVP 接受;Sprint 5 工程质量补)
- `audit_logs.entity_type` ENUM 闭合(S0 已提,未改)

---

## 三、Validation Results

| Check | Result | Note |
|---|---|---|
| `go vet ./...` (api + ai-worker) | ✓ Pass | |
| `go build ./...` | ✓ Pass | |
| `go test -race ./...` | ✓ Pass | `statemachine` 3 用例 + `middleware/request_id` 2 用例 + `auth` 2 用例;**`handler/s1.go` 0 用例** |
| `pnpm tsc --noEmit` | ✓ Pass | |
| 测试覆盖率 | △ Fail criteria | 关键路径(handler/s1.go)0 测试,与 PLAN/rules"关键路径 100%"差距大 |

**注意**:验证全过 ≠ 业务跑通。**B1 enum 不匹配 在静态检查里看不出来,但 runtime 会炸**;如果之后 CI 加了 `make seed` 步骤,会立刻在 seed 第一题挂。

---

## 四、必改清单(按优先级)

### CRITICAL — 下个 commit 必须修
1. **`task_items.status` enum 与 const 二选一对齐**(B1):推荐 `('available','claimed','finished')` 的 migration 002

### HIGH — Sprint 1 完成标准前必修
2. **`Upload.SaveUploadedFile` 真写盘**(B2)
3. **`ReviewSubmission` reject/revise 时 reason 必填校验**(B3)
4. **`Logout` 改名或加 revocation 表**(B9,S0 重复)
5. **`JWT_SECRET` 删除 dev default,启动校验**(B9,S0 重复)
6. **`handler/s1.go` 加 3 个关键路径测试**(claim 并发 / submit-from-revising 清 verdict / approve 触发 finished_items)

### MEDIUM — Sprint 2 启动 Designer 前补
7. ReviewerQueue 白名单 status query(B5)
8. ListTasks 按 owner_id 过滤(B7)
9. `sql.NullString` MarshalJSON 拆 `*string`(B8)
10. saveRevision 跨态写 2 条 audit_log(B4)
11. ExportJSON 真做 include_reviews JOIN(B6)
12. client.ts 401 → clearToken + 跳登录

### LOW — Sprint 5 工程质量再扫
13. Upload MIME + size;ExportJSON 分页;InlineLLM ctx;localStorage → httpOnly cookie

---

## 五、业务理解深度评分(answering "重点关切")

**S1 总评:8.5/10**(S0 是 7,**进步明显**)。

**亮点**:
- 上次 review 提的 **API envelope / request_id / CORS / state machine / qa_quality seed / finished_items 原子** **6 项 HIGH/MEDIUM 全部修了**,这是真"吸收 review feedback"的迭代,不是表面修复。
- 状态机 module 拆得干净(类型 + 常量 + map[Key]Transition + Can/Apply/Transitions 三个 API),完全可以拿到答辩讲"状态机是模块化设计,Sprint 3 worker 直接复用"。
- `first_come` 的 `SKIP LOCKED` 是 PLAN §4.5 推荐方案,**没用糊弄的 `UPDATE ... WHERE ... LIMIT 1`** —— 这一处是评委如果细看会**专门加分**的点。
- `saveRevision` 一个事务里 5 件事(create revision / update submission / clear verdicts / INSERT outbox / INSERT audit_log),这就是 PLAN §4.5"单库 outbox 一致性"的教科书实现。

**扣分 1.5 在**:
- **B1 enum mismatch** —— 上一轮 review 提了同样症状的债务(`task_items.status` 复用 submission 7 态),作者改了一半(const),没改另一半(enum),**这是没把 review 当系统问题来吸收**;
- **B2 upload 不落盘** —— 业务漏洞看着像"赶时间忘了",反映对"sprint 1 砍项表里 FileUpload 只砍预览,不砍写盘"这条 PLAN 细则**没有 line-by-line 读到底**。

**给评委讲故事时可用 / 不能用的 talking points 更新**:

✅ 可以讲:
1. 7 态状态机模块化 + 100% 迁移表单测 → 指 `statemachine/machine.go` 的 10 条 transition map
2. first_come SKIP LOCKED 并发抢单 → 指 `s1.go:194` 的 `clause.Locking{Options: "SKIP LOCKED"}`
3. revising 重提自动清 ai_verdict → 指 PLAN §4.3 表那一行 + `s1.go:491-493`
4. owner1 同时 owner + reviewer → seed 数据演示
5. 单库 outbox 一致性 → 指 `s1.go:507` outbox INSERT 在同事务内
6. **不再可以讲**:S0 那个"我们有统一 envelope"原本是不能讲(B2 当时没修),**现在可以讲了**(envelope 已修)

❌ 不能讲(否则被验证翻车):
1. 不要演示 FileUpload 预览(B2,文件根本没存)
2. 不要演示"打回时审核员必须填理由"(B3,前端 default + 后端不校验)
3. 不要演示"导出包含审核记录"(B6,SQL 不真 join)
4. 不要在 demo 前没修 B1 enum 就跑 `make seed`(会挂)

---

## Files Reviewed(S1 增量)

| File | Type | Lines |
|---|---|---|
| `apps/api/internal/statemachine/machine.go` | New | 77 |
| `apps/api/internal/statemachine/machine_test.go` | New | 55 |
| `apps/api/internal/handler/s1.go` | New | 673 |
| `apps/api/internal/httpx/response.go` | Modified(整改 envelope) | 54 |
| `apps/api/internal/httpx/pagination.go` | New | 38 |
| `apps/api/internal/middleware/request_id.go` | New | 31 |
| `apps/api/internal/middleware/request_id_test.go` | New | 50 |
| `apps/api/internal/middleware/cors.go` | New | 47 |
| `apps/api/cmd/server/main.go` | Modified(挂 s1 + CORS + request_id) | 63 |
| `apps/api/cmd/seed/main.go` | Modified(qa_quality seed + 9 用户) | 202 |
| `tools/seed/templates/qa_quality_review.json` | New | 20 |
| `apps/web/src/shared/api/client.ts` | New | 128 |
| `apps/web/src/shared/components/ShowItem.tsx` | New | 193 |
| `apps/web/src/shared/components/payload.ts` | New | 13 |
| `apps/web/src/modules/auth/Login.tsx` | Rewrote(真实登录) | 52 |
| `apps/web/src/modules/labeler/Plaza.tsx` | Rewrote(完整作答页) | 435 |
| `apps/web/src/modules/reviewer/Queue.tsx` | Rewrote(审核队列 + 详情) | 197 |
| `apps/web/src/modules/owner/Dashboard.tsx` | Rewrote(任务列表 + 导出) | 113 |


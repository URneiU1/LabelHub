# LabelHub S1 Acceptance — 稳定 Checkpoint

**版本基线**:`082b6d6` (`refactor: split s1.go (935→188 lines) + fix code review issues`)
**冻结时间**:2026-05-22
**目标**:把 S1(任务标注审核主线)定格为可演示、可继续开发的稳定基线,进入 S2 Designer/Renderer 前的最后一次自我验收。

---

## 1. S1 已交付能力

| 能力 | 实现位置 | 状态 |
|---|---|---|
| JWT 登录 / refresh / me / logout | `internal/handler/auth.go` + `internal/auth/` | ✓ |
| 三角色路由守卫(owner / labeler / reviewer / admin) | `internal/middleware/auth.go` + `RequireRoles` | ✓ |
| 前端登录守卫(无 token 跳 `/auth/login`,不打 API) | `apps/web/src/App.tsx` 的 `RequireAuth` | ✓ |
| 官方 `qa_quality` 任务 seed(30 条 item + baseline + template v1) | `cmd/seed/main.go` + `tools/seed/datasets/qa_quality/` | ✓ |
| Owner:List / Get / Import items / Export JSON | `internal/handler/s1_task.go` | ✓ |
| Labeler:List published / Claim(`FOR UPDATE SKIP LOCKED`)/ Save draft / Submit / My submissions | `internal/handler/s1_labeler.go` | ✓ |
| Reviewer:Queue(仅 `human_reviewing`)/ Approve / Reject / Revise | `internal/handler/s1_reviewer.go` | ✓ |
| 7 态状态机引擎 + 表驱动测试 | `internal/statemachine/` | ✓ |
| 单库 outbox + audit_log 同事务一致性 | `s1_labeler.go:147-223` `tx.Transaction(...)` | ✓ |
| Inline LLM mock(800ms 模拟延迟) | `internal/handler/s1.go` `InlineLLM` | ✓ |
| Upload(MIME / size 限制) | `s1.go` `Upload` + `UPLOAD_MAX_SIZE_MB` | ✓ |
| API envelope `{data, request_id}` / `{error:{code,message}, request_id}` | `internal/httpx/response.go` | ✓ |
| CORS allow-list + `X-Request-Id` round-trip | `internal/middleware/` | ✓ |

---

## 2. 权限边界(S1 当前真相)

| 路由 | 允许角色 | 说明 |
|---|---|---|
| `POST /auth/login` `POST /auth/refresh` | 公开 | — |
| `GET /me` `POST /auth/logout` | 任意已登录 | — |
| `GET /tasks` `POST /tasks` `POST /tasks/:id/items/import` `GET /tasks/:id/export/json` | `owner`, `admin` | **未做 owner 跨人越权检查 —— 任何 owner 都能看/操作所有 task**,见 §5 |
| `GET /tasks/:id` | `owner`, `admin`, `labeler`, `reviewer` | — |
| `GET /labeler/tasks` `POST /tasks/:id/claim` `POST /tasks/:id/items/:itemId/draft|submit` `GET /me/submissions` | `labeler` | submit 仅本人 `claimed_by` 才能动 |
| `GET /reviewer/submissions` `POST /submissions/:id/review` | `reviewer`, `owner`, `admin` | queue **只**返回 `human_reviewing` 状态,其他状态请求会 403 |
| `POST /llm/inline` `POST /upload` | 全部已登录角色 | — |

**Owner ↔ Reviewer overlap**:`owner1` 同时挂 `owner` + `reviewer` 角色,可作"任务负责人也能上线审核"的 demo;`reviewer1` 仅 `reviewer`。

---

## 3. 状态机现状

**Submission 7 态**:`draft → submitted → ai_reviewing → human_reviewing → approved | rejected | revising`
- 实现:`internal/statemachine/machine.go` + 表驱动测试(合法 11 / 非法 5 / 数量 10)。
- `ai_review_enabled=false` 的任务(包括官方 `qa_quality`)submit 后直接跳过 `ai_reviewing` 进 `human_reviewing`,不依赖 worker。
- `revising → submit` 会清空 `ai_verdict / ai_score / human_verdict` 重走流程。

**TaskItem 简化为 3 态**:`available → claimed → finished`
- 真相来源是 submission;`task_items.status` 仅作 claim 抢单互斥用,不再做审核状态镜像。
- approved/rejected 都置 `finished`(reject 也算"做完了"不再下发)。

---

## 4. Demo 账号 & 路径

| 用户 | 密码 | 角色 |
|---|---|---|
| `owner1` | `pass` | owner + reviewer(双角色 demo) |
| `owner2` | `pass` | owner |
| `labeler1` `labeler2` `labeler3` | `pass` | labeler |
| `reviewer1` `reviewer2` | `pass` | reviewer |
| `admin1` | `pass` | admin |
| `system_ai` | `pass` | system |

**官方任务**:`官方 qa_quality 质检标注`(id=1,published,30 题,`ai_review_enabled=false`)。

---

## 5. 已知 warning / 非阻塞遗留

| 项 | 严重度 | 处理时间窗口 |
|---|---|---|
| `lottie-web` 直接 `eval()`(Semi Design 间接依赖)导致 vite build 时一行警告 | LOW | S2 polish,可加 `build.rollupOptions.external` 或换 lottie-light |
| 主 bundle 712 kB(>500 kB chunk 警告) | LOW | S2 起做 dynamic import + route-level code split |
| Frontend 401 处理:目前 `getToken()` 为空才跳登录,**过期 token** 拿到 401 后 UI 会停在加载态(不会显示"missing bearer token",但也不会自动跳登录) | MEDIUM | S2,加 axios interceptor |
| `Upload` 不校验 `task_id` 是否归属当前用户(只查 task_id != 0),理论上可在他人 task 目录下落 orphan 文件;无 FK,DB 也不挡 | LOW(demo 无外部用户,S3 多租户前修) | S3 |
| `ai-worker` 还没接入真实 Doubao 调用,只有 mock provider | INFO | S3 主线 |

### Step 4 Stability Review 审计结论(2026-05-22)

| 关切点 | 审计结果 |
|---|---|
| Owner 跨人越权 | ✓ **已正确防护**。`ListTasks` 给非 admin 加 `WHERE owner_id = claims.UserID`(`s1_task.go:21`);`loadOwnedTask` 同时 admin OR `task.OwnerID == claims.UserID`(L212);`canReadTask` 对 owner 角色同样校验 OwnerID。所有写入路径 / Export / Import 都经过 `loadOwnedTask`。 |
| Reviewer 看不可处理状态 | ✓ `ReviewerQueue` 显式 allow-list 只放 `human_reviewing`(`s1_reviewer.go:17-28`);其它 status 返回 403。`ReviewSubmission` 经 `statemachine.Apply` 强校验状态转移,非法状态返 422。 |
| Export NULL 字段 | ✓ `external_id` 经 `nullStringJSON` 处理 NULL;`payload` 数据库 NOT NULL;`answer` 通过 JOIN `current_revision_id` 保证非空;`mustJSON` 反序列化失败兜底返回原字符串。 |
| Upload 守护 | ✓ MIME allow-list(6 类)+ 图片 5 MB + 总 10 MB 双重 size 校验 + `task_id` 必填 + 落盘失败回滚 DB metadata。**唯一缺口**:不校验 `task_id` 归属(见上表 LOW)。前端守卫上传需要 token,401 由 RequireAuth 拦截。 |
| Seed 幂等 | ✓ users / roles / task / template / items 全部 `FirstOrCreate`(或 count-check-insert),实测连跑两次 count 不变(9 users / 10 role rows / 1 task / 30 items / 1 template)。 |

---

## 6. 验收脚本(可重复)

```bash
cd /Users/dadadineiyou/Desktop/LabelHub

# 1. 基础环境
docker compose -f deploy/docker-compose.yml up -d   # MySQL 13306 + Redis 6379

# 2. 编译 + 单元测试
( cd apps/api && go test ./... )                    # 全部 ok
pnpm --dir apps/web lint                            # 0 errors
pnpm --dir apps/web build                           # ok(含 §5 已知 warning)

# 3. seed(幂等)
make seed                                           # "seed ready; demo password is pass"

# 4. E2E smoke(需 API 起在 18181,或改 BASE)
#    见 docs/S1_ACCEPTANCE.md §7 完整脚本
```

---

## 7. E2E smoke 实测记录(2026-05-22)

API 起在 `:18181`(本机 8080 被另一个 Python 服务占用),完整链路:

| 步骤 | 调用 | 结果 |
|---|---|---|
| 1 | `POST /auth/login` owner1 | accessToken / refreshToken / user(roles=[owner,reviewer]) |
| 2 | `GET /tasks` (owner) | `qa_quality` id=1, status=published, totalItems=30, finishedItems=1(上次 smoke 残留) |
| 3 | `POST /tasks/1/claim` (labeler1) | item id=2, status=claimed, claimedBy=2 |
| 4 | `POST /tasks/1/items/2/submit` (labeler1, payload=4 评分+summary+comment) | submission id=2, status=human_reviewing |
| 5 | `GET /reviewer/submissions` (reviewer1) | count=1, [{id:2, status:human_reviewing, itemId:2, labelerId:2}] |
| 6 | `POST /submissions/2/review` (reviewer1, verdict=approve) | `{status:"approved", submission_id:2}` |
| 7 | `GET /tasks/1/export/json` (owner1) | rows=2, statuses=[approved, approved], item_ids=[1,2] |
| 8 | 401 守护:`/tasks` `/labeler/tasks` `/reviewer/submissions` `/me` 无 token | 全部 HTTP 401 `UNAUTHORIZED` |

**前端守卫**(静态核对 `apps/web/src/App.tsx:30`):`RequireAuth` 在 `getToken()` 为空时直接 `<Navigate to="/auth/login" replace />`,**未登录访问 `/owner` `/labeler` `/reviewer` 不会触发任何 API 调用**,不会显示 "missing bearer token"。

---

## 8. 进入 S2 的先决条件(均已满足)

- [x] go test 全绿
- [x] 前端 lint + build 通过(已知 warning 入档)
- [x] seed 幂等,30 条 item + 4 个 demo 用户齐全
- [x] 7 角色路由 + 7 态状态机 + 单库 outbox + audit 全跑通
- [x] E2E smoke 5 步链路实测过
- [x] 未登录跳登录,不暴露 API 错误
- [x] 主线 git 干净,单 commit 即可标 S1 baseline

下一步:S2 Template list/detail + 9 核心物料 schema + Renderer 优先于 Designer。Designer 先做静态三栏骨架。

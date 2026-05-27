# S5 Implementation Plan — 工程质量 25%

> 配套 `PLAN.md` §Sprint 5(467-475 行)。给 **code session(Codex)** 的可执行落地版,粒度对齐 `docs/PLAN-S4-IMPL.md`:逐天 / 逐任务 / 先写测试再实现 / 文件路径 + 代码骨架 / 每步带验证命令。
>
> **S5 是评分占比最大的单项(25%)**,本质是"把已有功能补成可交付工程":单测覆盖、真容器集成测试、TS 严格化、自动 API 文档、生产部署模板、文档输出。**不写新业务功能。**
>
> **技术选型已由 PLAN.md §8 定死,本文不再重选**:testcontainers-go / swaggo(swag)/ TS `strict:true` / openapi-typescript / Caddy。唯一标注风险点见 §0.2(swag 注解工作量 + fallback)。

---

## 0. 现状盘点(S5 开工前)

### 0.1 后端单测覆盖率现状(`go test -cover`,2026-05-28)

| 包 | 当前 | S5 目标 | 缺口 |
|---|---|---|---|
| `internal/statemachine` | **92.9%** | ~100% | 全迁移表少数非法迁移分支 |
| `internal/service/review` | **13.0%** | 关键路径满覆盖 | `review.Apply` approve/reject/revise + 锁顺序 + RowsAffected 竞态几乎没测 |
| `internal/service/outbox` | **29.1%** | 关键路径满覆盖 | publish 失败/重试/DLQ、stale 重置、claim race 未覆盖 |
| `apps/ai-worker/cmd/worker` | 60.1% | verdict 决策满覆盖 | `handleAIReview` 的 verdict→状态映射、熔断、failover |
| `pkg/exporter` | (S4 已覆盖核心) | 保持 | 4 格式 + Run + token + loadrows 已测 |
| `internal/handler` | 61.4% | 不强求 100% | 关键 handler 已有;补缺的高价值分支即可 |
| `internal/service/submission` | 57.4% | 关键路径满覆盖 | claim/save/submit 状态机交互 |
| `internal/service/aireview`(sweeper) | 60.8% | 满覆盖 | 超时转人工分支 |

> **"单测 100%"按 PLAN.md 原文 = 关键路径**(状态机全迁移表 + AI verdict 决策 + 4 导出器 + outbox publisher),**不是每个包 100%**。`httpx`/`model`/`db`/`cmd/server` 这类薄包不强求。

### 0.2 基建缺口(S5 要新建的)

| 项 | 现状 | S5 动作 |
|---|---|---|
| testcontainers-go | **未引入**;`integration_test.go` 实为 sqlmock(gorm over sqlmock) | 新依赖,起真 mysql+redis 跑端到端主流程,build tag 隔离 |
| swag / OpenAPI | **未引入**,无注解 | swaggo 注解 + `swag init` 生成 OpenAPI + Swagger UI |
| Postman Collection | 无 | 从 OpenAPI 导出 / 手维护一份 |
| openapi-typescript | 无,`shared/api/client.ts` 全手写 | 从 OpenAPI 反生成 `schema.d.ts`,client 类型对齐 |
| TS `strict` | **关闭**(`tsconfig.app.json` 只有 `noUnusedLocals/Parameters`);src 里几乎无显式 `any` | 开 `strict:true`,修空安全 fallout |
| ErrorBoundary | **无**(组件崩溃→整页白屏) | 全站 ErrorBoundary 包 `<App/>` |
| 错误文案 | `httpx.Error(status, code, message)` 体系在(S1),前端散落 | 覆盖补齐 + 文案统一 |
| `docker-compose.prod.yml` | **无**(只有 dev `deploy/docker-compose.yml`) | api+worker+web+mysql+redis+asynqmon+caddy |
| Caddy | 无 | Caddyfile 自动 HTTPS |
| `ARCHITECTURE.md` / `DEPLOY.md` | **无**(只有 README) | 新建 |
| `make dev` | 无(只有 up/api/worker/web/seed) | 加一键起步 target |
| CI | `.github/workflows/ci.yml` 跑 `./apps/api/... ./apps/ai-worker/... ./pkg/llmreview` | **漏了 `./pkg/exporter`**,补上;集成测试加独立 job |

### 0.3 已就位、不要重做

- `httpx` 统一响应封装(`OK` / `Error(status,code,message)` / `ErrorWithDetails` + `{code,message}` 信封)——错误码体系已在,S5 只做覆盖统一。
- `pkg/exporter` 4 格式 + Run + token 单测(S4 完成)。
- `deploy/docker-compose.yml`(dev)、`deploy/asynqmon/`、`deploy/nginx/`(asynqmon basic auth)。
- GitHub Actions CI 骨架。
- `outbox/publisher.go` 已有 `publisher_test.go`(覆盖率低,补分支即可,不重写)。

---

## Pre-flight(Day 0,≤45 min)

### Task 0.1:基线 + 新依赖

```bash
cd ~/Desktop/LabelHub && git checkout -b s5-engineering-quality
go test ./apps/api/... ./apps/ai-worker/... ./pkg/exporter ./pkg/llmreview && pnpm -F web test && pnpm -F web lint && pnpm -F web build   # 基线全绿

# Go:testcontainers(集成测试,Day2)
cd apps/api && go get github.com/testcontainers/testcontainers-go@latest \
  github.com/testcontainers/testcontainers-go/modules/mysql@latest \
  github.com/testcontainers/testcontainers-go/modules/redis@latest && cd ../..

# Go:swag(Day4)—— 注解扫描 + Swagger UI
cd apps/api && go get github.com/swaggo/gin-swagger@latest github.com/swaggo/files@latest && cd ../..
go install github.com/swaggo/swag/cmd/swag@latest   # 装 swag CLI(生成器,不进 go.mod)

# 前端(Day3/4)
pnpm -F web add -D openapi-typescript @testing-library/jest-dom   # 后者若已装跳过
```

> ⚠️ testcontainers 需要本地 **Docker 可用**(用户机已有 colima/docker)。集成测试默认用 build tag 隔离,普通 `go test` 不跑。

### Task 0.2:CI 补 `./pkg/exporter`

`.github/workflows/ci.yml` 的 `Test Go workspace` 步骤补上 exporter:
```yaml
- name: Test Go workspace
  run: go test ./apps/api/... ./apps/ai-worker/... ./pkg/exporter ./pkg/llmreview
```

---

## Day 1 — 后端关键路径单测(纯 TDD 补齐,不改业务逻辑)

> 目标:把 PLAN.md 点名的关键路径补到接近满覆盖。**这是 25% 的主力得分项,优先做扎实。**

### Task 1.1:状态机全迁移表(`statemachine` 92.9%→~100%)

`apps/api/internal/statemachine/machine_test.go` 补:
- **合法迁移全表**:对 `map[from][event]→to` 的每一条都断言一次(table-driven)。
- **非法迁移**:对每个状态喂一个不允许的 event,断言返回错误 / 原状态不变。
- 7 态对照 §4.2:`draft / submitted / ai_reviewing / human_reviewing / approved / rejected / revising`,含 S4 新增 `ai_auto_approved` 路径。
- 覆盖率验证:`go test -cover ./internal/statemachine` 应 ≥ 99%。

### Task 1.2:AI verdict 决策(worker)

`apps/ai-worker/cmd/worker/ai_review_test.go`(或新建)table-driven 覆盖 `handleAIReview` 的 verdict→状态映射:
- `pass` + `human_review_enabled=false` → `approved`(`ai_auto_approved` audit)
- `pass` + `human_review_enabled=true` → `human_reviewing`
- `fail` / `uncertain` → `human_reviewing`(+ `ai_verdict` 落字段)
- verdict 与 score threshold 不一致 → 拒绝(non-retryable failover,S3 已有逻辑,补断言)
- 熔断 open / provider 5xx → failover 人工
- 幂等:重复消费 finalized submission → no-op

### Task 1.3:`review.Apply`(13%→关键路径满覆盖)

`apps/api/internal/service/review/review_test.go` 补(sqlmock + ExpectBegin/Commit):
- approve:`human_reviewing → approved`,`task_items.finished`,`tasks.finished_items+1`
- reject:`→ rejected`
- revise:`→ revising`,带 reason 写 audit
- **锁顺序 + RowsAffected**:并发写 `RowsAffected=0` → 返回竞态错误(对照 review code-review 提的"锁顺序与 submission.Save 相反"潜在死锁,**测试暴露即可,修不修单列**)
- 非法状态(非 `human_reviewing`)→ 422

### Task 1.4:outbox publisher(29%→关键路径满覆盖)

`apps/api/internal/service/outbox/publisher_test.go` 补:
- `PublishOnce`:pending → enqueue 成功 → `published`
- enqueue 失败 → `RetryCount+1`,< 5 回 `pending`,≥ 5 → `failed`
- 幂等 `TaskID` 冲突(`asynq.ErrTaskIDConflict`)视为成功
- `resetStaleProcessing`:超 `processingTimeout` 的 `processing` 回 `pending`
- `claimPending` SKIP LOCKED 批量认领 + `RowsAffected != len` 的 lost-update 错误

### Task 1.5:覆盖率门禁(可选但加分)

`make cover` target:`go test -cover ./...` 汇总;关键包低于阈值则非 0 退出。CI 可加 `-coverprofile` 上传。

**Day 1 验证**:`go test -race -cover ./apps/api/... ./apps/ai-worker/...`;关键四项(statemachine / verdict / review / outbox)覆盖率明显抬升,贴进 `S5_ACCEPTANCE.md`。

---

## Day 2 — testcontainers-go 端到端集成测试

> PLAN.md 明确:**真 mysql+redis** 跑"提交→AI→审核→Labeler 修订→终审→导出"主流程。这是和现有 sqlmock 单测互补的真集成。

### 文件落点

```
apps/api/internal/integration/        # 新目录, 与 handler 的 sqlmock "integration_test.go" 区分
  main_e2e_test.go                     # //go:build integration
  containers_test.go                   # 起 mysql + redis 容器 + 跑 migration + seed 的脚手架
```

### Task 2.1:容器脚手架(`//go:build integration`)

```go
//go:build integration

func startStack(t *testing.T) (dsn string, redisAddr string) {
	ctx := context.Background()
	myc, err := mysql.Run(ctx, "mysql:8.0",
		mysql.WithDatabase("labelhub"), mysql.WithUsername("labelhub"), mysql.WithPassword("test"))
	// require.NoError; t.Cleanup(func(){ myc.Terminate(ctx) })
	rdc, err := redis.Run(ctx, "redis:7-alpine")
	// 取 myc.ConnectionString(ctx, "multiStatements=true&parseTime=true") 跑 migration
	// 返回 dsn / redisAddr 给测试装配 server + worker + outbox publisher
}
```

### Task 2.2:主流程 E2E(`LLM_PROVIDER=mock`,断网可跑)

`main_e2e_test.go` 一条龙(用真 HTTP server + 真 DB + 真 asynq/redis,worker 在 goroutine 里跑):
1. seed owner/labeler/reviewer + 官方 qa_quality 任务(`ai_review_enabled=true`,mock provider)
2. labeler 领题 → 提交 → outbox publisher 投递 → worker 写 `ai_verdict` → 状态进 `human_reviewing`
3. reviewer 打回(revise + reason)→ labeler 修订(revision_no+1)→ 再提交 → 终审 approved
4. owner `POST /exports`(csv)→ worker `exporter.Run` 落盘 → `GET exports` 见 succeeded → 签名 URL 下载校验文件行数
5. 断言每步 DB 状态 + 最终导出文件内容

> 用 `testing.Short()` 或 build tag 双保险:`go test -tags=integration ./apps/api/internal/integration`。普通 `go test` 不跑(慢 + 需 docker)。

### Task 2.3:CI 集成 job

`.github/workflows/ci.yml` 加独立 job(GHA runner 自带 docker):
```yaml
integration:
  runs-on: ubuntu-latest
  steps:
    - uses: actions/checkout@v5
    - uses: actions/setup-go@v6
    - run: go test -tags=integration ./apps/api/internal/integration -timeout 300s
```

**Day 2 验证**:`go test -tags=integration ./apps/api/internal/integration -v` 本地绿(docker 起);普通 `go test ./...` 不受影响仍快。

---

## Day 3 — 前端 Renderer/Designer 测试 + TS `strict:true`

### Task 3.1:Renderer 关键组件单测(vitest + RTL)

`apps/web/src/renderer/widgets/__tests__/`(或就近 `*.test.tsx`):
- **ShowItem 多态**:`text / video / image / markdown / json` 各渲染一次,断言对应 DOM(markdown 转 HTML、json 高亮、image/video 标签)。
- **LLMTrigger**:点击触发 `/llm/inline`(mock fetch),replace/append 模式写回 target 字段。
- **联动 / 校验**:`requiredWhen` 显隐、`regex` / 长度校验报错渲染(对照 `packages/schema-spec` 与后端 parser 已支持)。

### Task 3.2:Designer 集成测试(拖拽 → 保存 → Renderer round-trip)

`apps/web/src/modules/owner/__tests__/designer-roundtrip.test.tsx`:
- 渲染 Designer → 添加 ShowItem + Radio + 一个容器子字段 → 调整顺序 → `buildTemplatePayload`
- 把产出的 schema 喂给 `SchemaRenderer` → 断言渲染出对应 widget(round-trip 一致)
- dnd-kit 拖拽用 `fireEvent` 模拟或直接调 reorder 逻辑(拖拽 e2e 留给 Playwright,这里测数据 round-trip)

### Task 3.3:TS `strict:true` 迁移

- `apps/web/tsconfig.app.json` 加 `"strict": true`(连带 `noImplicitAny` / `strictNullChecks` 等)。
- `pnpm -F web build`(`tsc -b`)→ 逐个修 fallout。`any` 几乎没有(已查),主要是 **null/undefined 收窄** 和 **fetch 响应类型**。
- 外部/不可信输入用 `unknown` + 收窄(对照全局 TS 规则),不要用 `any` 压制。
- 验证:`pnpm -F web build` 0 error;`pnpm -F web test` 全绿。

**Day 3 验证**:`pnpm -F web test`(新增 Renderer/Designer 用例)+ `pnpm -F web build`(strict)全绿。

---

## Day 4 — swag OpenAPI + Postman + openapi-typescript + 架构/部署文档

### Task 4.1:swag 注解 + 生成 OpenAPI

- `apps/api/cmd/server/main.go` 顶部加 general API info 注解(`// @title LabelHub API` / `@version` / `@securityDefinitions.apikey BearerAuth` 等)。
- **逐 handler 加注解**(`@Summary / @Tags / @Param / @Success / @Failure / @Router`)——**工作量最大的一步,见 §风险 R1**。优先覆盖主流程端点(auth / tasks / templates / labeler / reviewer / exports / stats),次要端点(golden-sample / ai-dry-run)时间够再补。
- `swag init -g apps/api/cmd/server/main.go -o apps/api/docs` 生成 `swagger.json` / `swagger.yaml` / `docs.go`。
- 挂 Swagger UI:`r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))`(仅 dev / 非 release 暴露)。
- 注解示例:
```go
// CreateExport godoc
// @Summary  创建异步导出
// @Tags     export
// @Security BearerAuth
// @Param    taskId path int true "任务ID"
// @Param    body body createExportRequest true "格式与字段映射"
// @Success  200 {object} httpx.Envelope
// @Failure  400 {object} httpx.Envelope
// @Router   /tasks/{taskId}/exports [post]
func (h ExportHandler) CreateExport(c *gin.Context) { ... }
```

### Task 4.2:openapi-typescript 反生成前端类型

```bash
pnpm -F web exec openapi-typescript ../../apps/api/docs/swagger.yaml -o src/shared/api/schema.d.ts
```
- `shared/api/client.ts` 的响应泛型逐步换成 `schema.d.ts` 里的 `components['schemas'][...]`,消手写类型漂移。
- 加 `package.json` script:`"gen:api": "openapi-typescript ../../apps/api/docs/swagger.yaml -o src/shared/api/schema.d.ts"`。

### Task 4.3:Postman Collection

- 从 OpenAPI 转:`openapi2postmanv2 -s apps/api/docs/swagger.json -o docs/LabelHub.postman_collection.json`(`npx -p openapi-to-postmanv2`),或手维护一份覆盖主流程 + 登录拿 token 的 collection。
- 放 `docs/LabelHub.postman_collection.json`,README 注明导入方式 + 演示账号。

### Task 4.4:`ARCHITECTURE.md` + `DEPLOY.md`

- `docs/ARCHITECTURE.md`:从本 plan + PLAN.md §3/§8 整理;**mermaid 图**(模块依赖 / 标注全流程状态机 / outbox→asynq 数据流);关键决策表(monorepo、状态机引擎、outbox、Designer、Function Calling、Editorial)。
- `docs/DEPLOY.md`:dev 起步(`make dev`)+ prod(`docker-compose.prod.yml` + caddy 域名/HTTPS)+ 环境变量清单(`.env.example` 对照)+ 备份/回滚。

**Day 4 验证**:`swag init` 无 error;`/swagger/index.html` 本地可开;`schema.d.ts` 生成且 `pnpm -F web build` 通过;Postman collection 导入能登录跑通一条主流程。

---

## Day 5 — ErrorBoundary + 错误文案 + 生产部署模板 + make dev + 验收

### Task 5.1:全站 ErrorBoundary

`apps/web/src/shared/components/ErrorBoundary.tsx`(class component):
```tsx
class ErrorBoundary extends React.Component<{children: React.ReactNode}, {error: Error | null}> {
  state = { error: null }
  static getDerivedStateFromError(error: Error) { return { error } }
  componentDidCatch(error, info) { /* 上报/console.error,生产可接 sentry */ }
  render() { return this.state.error ? <FallbackUI onReset={...}/> : this.props.children }
}
```
- 在 `main.tsx` / 路由根包 `<App/>`;Fallback 用 Editorial 风格(Serif 大字 + 重载按钮)。
- 测试:抛错子组件 → 断言渲染 Fallback 而非整页崩。

### Task 5.2:API 错误文案统一

- 审计所有 `httpx.Error(...)` 的 `code` 一致性(`VALIDATION_ERROR` / `NOT_FOUND` / `FORBIDDEN` / `INTERNAL_ERROR` / `GONE` ...),建一张码表写进 `httpx/response.go` 顶部注释或 `docs/ARCHITECTURE.md`。
- 前端 `shared/api/client.ts` 统一把 `{code,message}` 抛成 `Error(message)`,各页 `Toast.error` 文案中文化、去裸英文 code。

### Task 5.3:`docker-compose.prod.yml` + Caddy

`deploy/docker-compose.prod.yml`:`api` + `worker`(同镜像不同 command)+ `web`(静态 build 由 caddy 托)+ `mysql` + `redis` + `asynqmon` + `caddy`。
- 各服务需 `Dockerfile`:`apps/api/Dockerfile`(多阶段 go build,api 和 worker 共用,`command` 区分)、`apps/web/Dockerfile`(pnpm build → 产物给 caddy)。
- `deploy/Caddyfile`:
```
yourdomain.com {
  encode gzip
  handle /api/* { reverse_proxy api:8080 }
  handle { root * /srv/web; try_files {path} /index.html; file_server }
}
```
- 所有密钥走 `deploy/.env`(gitignored)+ `deploy/.env.example`(tracked),**绝不硬编码**(对照 S0-S3 review 的 compose 密码教训)。
- `EXPORT_DIR` 在容器里设**绝对路径**且 api/worker 挂同一 volume(对照 S4 review H1)。

### Task 5.4:`make dev` 一键起步

```makefile
dev: up           ## 起基础设施 + 装依赖 + seed,然后提示用户分别跑 api/worker/web
	$(MAKE) install
	sleep 3 && $(MAKE) seed
	@echo "✅ 基础设施就绪。请在三个终端分别运行: make api / make worker / make web"
```
> 不在 `make dev` 里后台 spawn api/worker/web(会留僵尸进程)。dev 只做"基础设施 + seed + 指引",长跑进程由用户开终端跑。

### Task 5.5:验收文档 `docs/S5_ACCEPTANCE.md` + 全绿

对照 PLAN.md §Sprint5 完成标准逐条勾:
- [ ] `go test ./...`(含 `pkg/exporter`)+ `pnpm test` 全绿
- [ ] `go test -tags=integration` 真容器主流程绿
- [ ] 关键路径覆盖率(statemachine / verdict / review / outbox / exporter)贴数字
- [ ] `pnpm -F web build` 在 `strict:true` 下 0 error
- [ ] `docker-compose -f deploy/docker-compose.prod.yml up` 在本机能起(截图/日志)
- [ ] 新人 `make dev` 5 分钟内跑起来
- [ ] Swagger UI 可开 + Postman collection 跑通主流程
- [ ] ARCHITECTURE.md / DEPLOY.md 完成

---

## 验收 Checklist(对照 PLAN.md §Sprint5 完成标准)

- [ ] 后端关键路径单测满覆盖(状态机全表 + AI verdict + 4 导出器 + outbox publisher)
- [ ] testcontainers 真 mysql+redis 端到端主流程
- [ ] 前端 Renderer 关键组件单测 + Designer round-trip 集成测试
- [ ] TS `strict:true`、无 `any`、openapi-typescript 反生成 client 类型
- [ ] 全站 ErrorBoundary + API 错误文案统一
- [ ] ARCHITECTURE.md + DEPLOY.md + Swagger/OpenAPI + Postman Collection
- [ ] `docker-compose.prod.yml`(api+worker+web+mysql+redis+asynqmon+caddy)本机可起
- [ ] `make dev` 5 分钟跑通;`pnpm test` + `go test ./...` 全绿

---

## 风险 & Fallback

| # | 风险 | 缓解 / Fallback |
|---|---|---|
| R1 | **swag 逐 handler 注解工作量大**(~10 个 handler 文件) | 优先注解主流程端点(auth/tasks/templates/labeler/reviewer/exports/stats);次要端点时间紧砍。**Fallback**:若 swag 注解严重超时,手写一份覆盖主流程的 `docs/openapi.yaml` 喂 openapi-typescript + Postman,acceptance 注明"未全量注解,主流程已覆盖" |
| R2 | testcontainers 需 docker + 慢 | build tag `integration` 隔离,普通 `go test` 不跑;CI 独立 job;本地 colima 已可用 |
| R3 | TS `strict` fallout 比预期多 | 已查 src 几乎无显式 `any`,主要是 null 收窄;逐文件修,真卡壳的单点用 `unknown`+收窄,不要 `any` 压制;timebox 半天 |
| R4 | prod compose 多服务调试耗时(Dockerfile + caddy + volume) | 先把 api/worker 单镜像跑通,web+caddy 次之;`EXPORT_DIR` 绝对路径 + 共享 volume(S4 H1 教训);只要求"本机能起",不要求真上 VPS |
| R5 | "单测 100%" 被字面理解→无限投入 | 严格按 PLAN.md 列举的关键路径,薄包(httpx/model/db)不强求;覆盖率抬升即可交付 |

---

## Execution Handoff

- 每个 code-writing turn 后按 `CODEX.md` 更新 `README.md`(完成/待改进/下一步/验证命令)+ 相关 md。
- 提交粒度:Day1 单测一组、Day2 集成测试一组、Day3 前端测试+strict 一组、Day4 swag/docs 一组、Day5 部署+ErrorBoundary 一组;信息用仓库现有 `test(s5):` / `feat(s5):` / `docs(s5):` / `chore(s5):` 风格,**无 AI 署名**。
- 顺序 Day1→Day5;集成测试(Day2)和部署模板(Day5)依赖 docker,放在单测之后。
- 别后台 spawn dev/worker 进程(全局规则):长跑让用户开终端,你只 `curl`/测试验证。
- 用 `git add <具体文件>`,绝不 `git add -A`。

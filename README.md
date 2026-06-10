# LabelHub · 数据标注平台

[![CI](https://github.com/URneiU1/LabelHub/actions/workflows/ci.yml/badge.svg)](https://github.com/URneiU1/LabelHub/actions/workflows/ci.yml)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/Go-1.22-00ADD8.svg?logo=go)](go.work)
[![React](https://img.shields.io/badge/React-18-61DAFB.svg?logo=react)](apps/web/package.json)
[![TypeScript](https://img.shields.io/badge/TypeScript-strict-3178C6.svg?logo=typescript)](apps/web/tsconfig.app.json)

> 字节 AI 全栈挑战赛课题 — 覆盖「数据生产 → AI 预审 → 人工审核 → 多格式导出」全生命周期的 Web 数据标注平台。

**评委友好链接**:
- [`submission/`](submission/) — 评委交付包(完整 walkthrough、演示视频脚本、截图)
- [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) — 架构图与决策表
- [`docs/DEPLOY.md`](docs/DEPLOY.md) — 单机部署 SOP
- [`docs/openapi.yaml`](docs/openapi.yaml) · [`docs/LabelHub.postman_collection.json`](docs/LabelHub.postman_collection.json) — API 契约与请求集
- [`docs/CODE-REVIEW-FINAL.md`](docs/CODE-REVIEW-FINAL.md) — 交付前 4 路并行 code review 汇总
- [`docs/CHANGELOG.md`](docs/CHANGELOG.md) — 8 周开发流水(Sprint 0 → Sprint 7)

## 快速启动

```bash
# 前置: Docker Desktop / Colima、Go 1.22+、Node 24+、pnpm 10+

# 0. 复制环境文件(默认 LLM_PROVIDER=mock,无需真实豆包 key)
cp .env.example .env

# 1. 一键起基础设施 + 装依赖 + seed
make dev

# 2. 三个终端分别跑长进程
make api       # 终端 A — http://localhost:8080/health
make worker    # 终端 B — AI 预审 worker
make web       # 终端 C — http://localhost:5173
```

若本机 `:8080` 被占用:`API_PORT=18080 make api` 起 API,再 `VITE_API_PROXY_TARGET=http://localhost:18080 make web` 起前端。

### 演示账号

`make seed` 会创建以下账号(密码均为 `123456`):

| 用户名 | 角色 | 用途 |
|---|---|---|
| `owner1` | Owner | 任务/模板/AI Prompt/Golden Sample 配置 |
| `labeler1` | Labeler | 任务广场领题、作答、提交 |
| `reviewer1` | Reviewer | 审核队列、verdict、规则查看 |
| `admin1` | Admin | 全局管理 |
| `system_ai` | System | AI 预审 Agent(后台,不需登录) |

### 5 分钟评委路径

1. 浏览器打开 http://localhost:5173 → 登 `owner1/123456`
2. 进 **Owner Dashboard** → 选官方 `qa_quality` 任务 → 看模板/AI Prompt/Golden Sample/Stats Board(官方任务 seed 后已启用 AI review)
3. 退出登 `labeler1/123456` → **任务广场** → 领取一题 → 作答并提交,触发 AI 预审
4. 退出登 `reviewer1/123456` → **审核队列** → 看 AI verdict + 维度评分 → 通过/打回
5. 回 `owner1` → **导出**(JSON/JSONL/CSV/XLSX 任选)→ 下载

完整 walkthrough 见 [`submission/DEMO_SCRIPT.md`](submission/DEMO_SCRIPT.md)。

## 架构

```
apps/web        React 18 + TypeScript(strict)+ Semi Design(单一 SPA,角色路由)
apps/api        Go + Gin + GORM REST API(:8080)
apps/ai-worker  Go + Asynq AI 预审 Worker(豆包 Function Calling)
pkg/exporter    共享多格式导出器(JSON / JSONL / CSV / XLSX / Markdown)
pkg/llmreview   共享 LLM provider(mock / OpenAI-compatible / 豆包)
```

| 基础设施 | 端口 |
|---|---|
| MySQL 8 | localhost:13306(user=labelhub, pass=labelhub_dev) |
| Redis 7 | localhost:6379 |
| Adminer(DB GUI) | http://localhost:18080 |
| Asynqmon(queue 监控,ops profile) | http://localhost:18081 |

完整架构图与设计决策见 [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md)。

## 关键技术亮点

- **状态机驱动** — 任务 4 态 + 提交 7 态,所有跃迁走 `internal/statemachine.Transitions()`,100% 单测覆盖,违法事件直接 reject
- **Outbox 一致性** — 业务事务同写 `outbox_events`,后台 publisher 用 `FOR UPDATE SKIP LOCKED` + deterministic Asynq TaskID 防双投
- **AI 预审幂等** — Worker `complete()/failover()` 双锁 + `RowsAffected != 1` 守每个状态跃迁,保证 5xx 重试不破坏状态
- **熔断 + 限流** — Provider 5xx 连续 20 次/5 分钟自动熔断,dry-run 走 task-scoped quota,登录走 IP token bucket
- **类型契约** — `docs/openapi.yaml` 是真相源,`pnpm -F web gen:api` 生成 `schema.d.ts`,前端 TypeScript strict mode + 0 个 `any`
- **测试纪律** — testcontainers 真 MySQL+Redis 集成测试覆盖主链路(submit→outbox→Redis→AI→review→export);CI 跑 Go workspace + web test+lint+build + 独立 `-tags=integration` job
- **首屏性能** — VChart 通过 `React.lazy` + Vite vendor split,首屏 eager vendor 从 2.2MB(gzip 616KB)降到 412KB(gzip 125KB)
- **大数据量压测(线上实测)** — 单任务导入 5000 题:`import-file` 5.4s、`publish` 0.24s、`GET labeler/items`(全量 5000 行 ≈468KB)0.7s、领题/打开题目 0.2–0.4s,API 侧均无瓶颈。前端题目导航当前**全量渲染**(5000 题 → DOM ~20K 节点、导航列表 5005 直接子节点),4× CPU 节流下首屏 Layout 220ms + Style 重算 237ms,有可感卡顿。**已知优化项**:题目导航上虚拟滚动(react-window / 视口切片)可把渲染节点压到常数级;常规任务规模(数十~数百题)无此问题
- **编辑级 UI 系统** — 自建 Editorial design tokens(serif headings + 7 态 status + skeleton/top-progress),非套模板默认
- **A11y** — Tabs 完整 ARIA `tablist/tab/tabpanel` + auto-jump first error tab,全局 `:focus-visible` ring,Labeler `Ctrl/Cmd+Enter` 提交快捷键

## 命令速查

```bash
make dev          # 一键起基础设施 + 装依赖 + seed
make up / down    # 起/停基础设施
make install      # 装 pnpm + Go 依赖
make seed         # 初始化两个官方任务(qa_quality 30 条 + preference_compare 12 条)
make api          # 跑 API
make worker       # 跑 AI worker
make web          # 跑前端

# 测试
go test ./apps/api/... ./apps/ai-worker/... ./pkg/...
pnpm -F web test                    # vitest
pnpm -F web lint                    # eslint
pnpm -F web build                   # 类型检查 + 打包

# 集成测试(需 Docker)
go test -tags=integration ./apps/api/internal/integration -count=1

# 生产部署
docker compose -f deploy/docker-compose.prod.yml --env-file deploy/.env.example up -d
```

## 目录结构

```
.
├── apps/
│   ├── web/              React 前端(单一 SPA,角色路由)
│   ├── api/              Gin REST API(handler / service / model)
│   └── ai-worker/        Asynq AI 预审 worker
├── pkg/
│   ├── exporter/         JSON / JSONL / CSV / XLSX 导出器
│   └── llmreview/        LLM provider(mock + OpenAI-compatible)
├── tools/seed/           官方数据集(qa_quality + preference_compare)+ 模板
├── deploy/               生产 docker-compose + Caddyfile + Dockerfile
├── docs/                 架构 / 部署 / API / Code Review / CHANGELOG
├── submission/           评委交付包
└── .github/workflows/    CI(单测 + 集成测试)
```

## License

[MIT](LICENSE)

---

> 开发日志(Sprint 0 → Sprint 7 逐次改动、技术决策、验证记录)请见 [`docs/CHANGELOG.md`](docs/CHANGELOG.md)。

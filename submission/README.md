# LabelHub · 评委交付包

> 字节 AI 全栈挑战赛 · 数据标注平台 LabelHub
> 提交人:Zhang Youchen(URneiU1) · 提交日期:2026-07-XX

本目录是评委评估的入口。所有交付物都在这里,**评委无需阅读 `docs/` 下的开发文档**(那是 8 周开发流水)。

---

## 📦 交付清单

| # | 交付物 | 文件 | 评分维度 |
|---|---|---|---|
| 1 | 源码 Monorepo | 仓库根目录(`apps/` + `pkg/`) | 功能完备性 60% + 工程质量 25% |
| 2 | 评委 README + 5 分钟评估路径 | 本文件 + [`DEMO_SCRIPT.md`](DEMO_SCRIPT.md) | 产品体验 15% |
| 3 | 演示视频脚本(5-10 分钟) | [`DEMO_SCRIPT.md`](DEMO_SCRIPT.md);视频二进制由最终提交平台上传,不纳入 Git | 产品体验 15% |
| 4 | 架构图 + 关键技术点 | [`ARCHITECTURE.md`](ARCHITECTURE.md)(含 Mermaid topology / 状态机 / outbox sequence) | 工程质量 25% |
| 5 | Demo 截图 | [`assets/screenshots/`](assets/screenshots/) — 10 张主流程截图 + 1 张修复前问题证据已入库 | 产品体验 15% |
| 6 | AI Coding 过程记录(8-15 页 PDF) | [`assets/AI-Coding-Process.pdf`](assets/AI-Coding-Process.pdf) + [`assets/AI-Coding-Process.md`](assets/AI-Coding-Process.md) | 工程质量 25%(附加) |
| 7 | 部署说明 | [`DEPLOY.md`](DEPLOY.md) | 工程质量 25% |
| 8 | API 文档 | [`api/openapi.yaml`](api/openapi.yaml) + [`api/postman_collection.json`](api/postman_collection.json) | 工程质量 25% |
| 9 | 交付前 Code Review | [`CODE-REVIEW-FINAL.md`](CODE-REVIEW-FINAL.md) | 工程质量 25%(附加) |
| 10 | 课题要求完成度与超额项对照 | [`BEYOND-REQUIREMENTS.md`](BEYOND-REQUIREMENTS.md) — 逐条要求 vs 交付 + 答辩取舍说明 | 功能完备性 60%(答辩材料) |
| 11 | License | [`LICENSE`](LICENSE) — MIT | — |

> 大体积视频文件不放入 Git 仓库;最终提交时按 [`DEMO_SCRIPT.md`](DEMO_SCRIPT.md) 录制并上传到比赛平台或外部附件。

---

## ⚡ 5 分钟评估路径(评委本机跑)

### 前置

| 工具 | 版本 |
|---|---|
| Docker Desktop / Colima | 任意现代版 |
| Go | 1.22+ |
| Node | 24+ |
| pnpm | 10+ |

> macOS 与 Linux 均验证通过。

### 启动(5 步)

```bash
# 0. 在仓库根目录
cd /path/to/LabelHub

# 1. 复制环境文件(默认 LLM_PROVIDER=mock,无需真实豆包 API key)
cp .env.example .env

# 2. 一键起基础设施 + 装依赖 + seed(MySQL + Redis + 两个官方任务)
make dev

# 3. 三个终端分别跑长进程
make api       # 终端 A — http://localhost:8080/health
make worker    # 终端 B — AI 预审 worker(mock provider)
make web       # 终端 C — http://localhost:5173
```

打开浏览器到 http://localhost:5173 即可。

### 评估账号(密码均为 `123456`)

| 用户名 | 角色 | 评估场景 |
|---|---|---|
| `owner1` | Owner | 任务配置 / 模板 Designer / AI Prompt / Golden Sample / Stats Board / 多格式导出 |
| `labeler1` | Labeler | 任务广场领题 / 答题(支持 Group + Tabs + ShowItem + Radio + Tags + RichText + JSONEditor + FileUpload + LLMTrigger 9+2 物料)/ 草稿自动保存 / 提交 |
| `reviewer1` | Reviewer | 审核队列 / AI verdict + 维度评分展示 / 规则查看 / 通过-打回-修订 / 批量审核 |
| `admin1` | Admin | 全局管理 |

### 5 分钟跑通三角色完整链路

详见 **[`DEMO_SCRIPT.md`](DEMO_SCRIPT.md)** — 含每一步的截图位置与预期。简版:

1. **Owner**(2 min) — 登 `owner1` → 选 `qa_quality` 官方任务 → 看模板 Designer 嵌套 Tabs/Group → 看 AI Prompt 三维度 + threshold → 跑 Golden Sample dry-run → 看 Stats Board(进度 / 通过率 / AI vs 人工 / 维度均分)
2. **Labeler**(1.5 min) — 退出登 `labeler1` → 任务广场领一题 → 看 Schema Renderer 真实物料 → 触发 LLM 辅助 → 提交
3. **Reviewer**(1 min) — 退出登 `reviewer1` → 审核队列 → 看 AI verdict + 维度评分 + 处理日志 → 通过
4. **Owner 导出**(0.5 min) — 回 `owner1` → 任务详情 → 选 JSONL/CSV/XLSX → 异步导出 → 下载

### 评委可能踩坑(已尽力清扫)

| 现象 | 原因 | 修复 |
|---|---|---|
| `make api` 报 `EXPORT_DIR must be absolute` | Makefile 已自动注入 `$(CURDIR)/data/exports`;若直接 `go run` 才会触发 | 用 `make api`,或手动 `export EXPORT_DIR=$(pwd)/data/exports` |
| `:8080` 已被占用 | 评委本机其它服务占用 | `API_PORT=18080 make api && VITE_API_PROXY_TARGET=http://localhost:18080 make web` |
| `make worker` 卡住 | mock provider 正常,如改 `LLM_PROVIDER=doubao` 但没填 `LLM_API_KEY` 才会假死 | 默认 mock,无需关心 |
| MySQL `13306` 冲突 | LabelHub 故意走 13306 避让本机 3306 | 见 [`DEPLOY.md`](DEPLOY.md) |

---

## 🏗 架构速览

```
React 18 + TS strict + Semi Design (SPA, 角色路由)
                  │
                  ▼ REST + WebSocket-free
              ┌───────────┐
              │  Gin API  │  ──► MySQL 8(状态机 + outbox)
              └─────┬─────┘
                    │ outbox publisher(FOR UPDATE SKIP LOCKED)
                    ▼
                  Redis 7
                    │ Asynq
                    ▼
           ┌─────────────────┐
           │  AI Worker(Go) │  ──► 豆包 Function Calling(OpenAI 兼容)
           └─────────────────┘
```

完整图(含状态机 + outbox sequence)见 [`ARCHITECTURE.md`](ARCHITECTURE.md)。

## 🎯 关键技术亮点

| 维度 | 实现 |
|---|---|
| **状态机驱动** | 任务 4 态 + 提交 9 态(含仲裁与共识证据),所有跃迁走 `internal/statemachine.Transitions()`,100% 单测覆盖 |
| **Outbox 一致性** | 业务事务同写 `outbox_events`,后台 publisher 用 `FOR UPDATE SKIP LOCKED` + deterministic Asynq TaskID 防双投 |
| **AI 预审幂等** | Worker `complete()/failover()` 双锁 + `RowsAffected != 1` 守每个状态跃迁 |
| **熔断 + 限流** | Provider 5xx 连续 20 次/5 分钟自动熔断;dry-run 走 task-scoped quota;登录 IP token bucket |
| **类型契约** | `openapi.yaml` 是真相源,`pnpm -F web gen:api` 生成 `schema.d.ts`,前端 strict + 0 个 `any` |
| **测试纪律** | testcontainers 真 MySQL+Redis 集成测试覆盖主链路;CI 跑 Go workspace + web test/lint/build + 独立 `-tags=integration` job |
| **首屏性能** | VChart `React.lazy` + Vite vendor split,首屏 eager vendor 2.2MB → 412KB |
| **编辑级 UI** | 自建 Editorial design tokens(serif + 7 态 status),非套模板默认 |
| **A11y** | Tabs 完整 ARIA + auto-jump first error tab + `:focus-visible` ring + Labeler `Ctrl/Cmd+Enter` |

## 🔒 安全

`CODE-REVIEW-FINAL.md` 含 4 路并行 review(Go / TypeScript / 安全 / 交付物 readiness)的全部发现与修复记录。安全侧已验证:

- 无 hardcoded secret 进仓(`git ls-files` 验证)
- 所有敏感值 `${VAR:?}` 强制(JWT_SECRET / MYSQL_PASSWORD / ASYNQMON_*)
- JWT 算法守卫 + refresh token 可撤销 + JTI 轮转
- Asynqmon 生产路径 Caddy basic_auth(无 ports mapping,仅 Docker 内网)
- 全部 SQL 走 `?` 参数化;0 处 shell exec
- Upload magic byte 校验 + MIME 白名单排除 HTML/SVG/script
- IDOR 守卫(reviewer 走 `task_reviewers`,`canReviewTask` 全路径覆盖)
- Markdown 走 JSX 不 `dangerouslySetInnerHTML`,URL 过 `isSafeURL`
- Login IP token bucket 防暴力 + 防用户名时序枚举

## 🛠 生产部署(可选评估)

```bash
# 复制并填生产 env
cp deploy/.env.example deploy/.env.prod
# 编辑 deploy/.env.prod,必填 ASYNQMON_USER/ASYNQMON_PASSWORD_HASH 等

# 起 prod 栈
docker compose -f deploy/docker-compose.prod.yml --env-file deploy/.env.prod up -d
```

SOP 见 [`DEPLOY.md`](DEPLOY.md)。

---

## 📂 本目录索引

```
submission/
├── README.md                 ← 你在这里
├── DEMO_SCRIPT.md            5/10 分钟演示脚本(视频与现场共用)
├── ARCHITECTURE.md           架构图与设计决策
├── DEPLOY.md                 单机/生产部署 SOP
├── CODE-REVIEW-FINAL.md      交付前 4 路并行 review 汇总
├── LICENSE                   MIT
├── api/
│   ├── openapi.yaml          OpenAPI 3 主流程契约
│   └── postman_collection.json   按 3 角色组织的请求集
└── assets/
    ├── README.md             素材说明与最终提交检查
    ├── AI-Coding-Process.md  AI Coding 过程源文件
    ├── AI-Coding-Process.pdf AI Coding 过程 PDF
    └── screenshots/          Owner/Designer/Labeler/Reviewer/Export 截图
```

## ❓ 联系

如有评估问题,请联系仓库 owner [`@URneiU1`](https://github.com/URneiU1)。

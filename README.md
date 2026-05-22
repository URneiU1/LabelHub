# LabelHub · 数据标注平台

字节 AI 全栈挑战赛课题 — 覆盖「数据生产 → AI 预审 → 人工审核 → 多格式导出」全生命周期的 Web 数据标注平台。

## 快速启动

```bash
# 1. 起基础设施(MySQL + Redis + Adminer)
make up

# 2. 安装依赖
make install

# 3. 初始化官方 qa_quality seed
make seed

# 4. 跑 API(终端 A,复用或新开)
make api                 # http://localhost:8080/health

# 5. 跑前端(终端 B,新开)
make web                 # http://localhost:5173

# MySQL(host):        localhost:13306  (user=labelhub, pass=labelhub_dev)
# Adminer(DB GUI):    http://localhost:18080  (server=mysql, user=labelhub, pass=labelhub_dev)
```

如果本机 8080 被占用,可用 `API_PORT=18080 make api` 启 API,再用 `VITE_API_PROXY_TARGET=http://localhost:18080 make web` 启前端。

## 架构

```
apps/web      — React 18 + TypeScript + Semi Design (单一 SPA,角色路由)
apps/api      — Go + Gin REST API (port 8080)
apps/ai-worker — Go Asynq AI 预审 Worker
```

详见 `docs/PLAN.md`。

## 开发记录

每次完成代码改动后,需要同步更新本节,并按实际情况维护 `CLAUDE.md` / `CODEX.md`:

- 本次完成: 写清楚做了哪些功能、修复或重构。
- 仍需提升: 写清楚当前已知风险、体验问题、技术债或 warning。
- 下一步: 写清楚下一轮最应该推进的任务。
- 验证记录: 写清楚已跑过的测试、lint、build 或手动验证。

### 最近完成

- 2026-05-22 `phase2-schema-runtime`: 完成阶段二动态模板运行时闭环:Labeler Plaza 和 Reviewer Queue 改为读取 `bundle.template.schemaJson` 并通过 `SchemaRenderer` 渲染;Reviewer 详情改走 `/reviewer/submissions/:submissionId`;补齐 RichText/JSONEditor/FileUpload/LLMTrigger 真实可交互 widget;前后端 parser/canonical 保存保留 `export_fields` 与 `x-*` 扩展;Vite 对 React/Semi/lottie 做分包并将 lottie 指到 light player,build 不再出现 eval/chunk warning。
- 2026-05-22 `fix(api): harden review and claim transactions`: 完成阶段一 P0 后端并发与越权重构:新增 `task_reviewers` 显式审核授权表;Reviewer Queue/Detail/Review 统一按 owner/admin/task_reviewers 做资源隔离;`review.Apply` 在事务内锁 task/submission 并校验 `human_reviewing`;Claim 在事务内锁 task、抢 item、立即创建 draft submission 并冻结 `template_version`;Save/Submit 在 submission 行锁保护下生成 revision_no。
- 2026-05-22 `0b871d5`: 修复安全和边界问题,包括 ShowItem URL 白名单、reviewer/owner 审核资源边界、upload task 权限与 MIME/content 校验、template/answer body 限制、CORS 显式 origin、submission template_version snapshot。
- 2026-05-22 `92f08ac`: 完成 S2 Day2 SchemaRenderer runtime,支持官方 schema 解析、核心 widget 渲染、答案校验和 renderer 单测。

### 仍需提升

- AI 预审异步闭环仍未完成:outbox publisher、pending `ai_reviews` 幂等占位、worker 失败降级到人工审核还需要阶段三落地。
- Save/Submit 的 item ownership 仍主要在 handler 事务外检查;后续若加入回收/转派,需要在 service 事务内锁 `task_items` 后二次校验。
- `ai_review_enabled` 数据库默认值仍需改成安全默认关闭,并补一次旧数据迁移,避免直接 SQL 创建任务进入 worker placeholder 死路。
- `task_reviewers` 目前通过 seed 赋予官方任务的 `reviewer1` 权限,Owner 后台的审核员分配 UI/API 还未实现。

### 下一步

- 推进 S2 后续:进入 Designer 的 append/delete/简易属性编辑能力。
- 补阶段一 P1:Save/Submit service 内锁 item ownership;DB default 关闭 AI。
- 继续阶段三前置:把 AI outbox topic、pending ai_reviews 行和 worker 降级状态机对齐。

### 验证记录

- 2026-05-22 阶段二: `cd apps/api && go test ./...` 通过;`pnpm -F web test` 通过;`pnpm -F web lint` 通过;`pnpm -F web build` 通过且无 lottie eval/chunk size warning。
- 2026-05-22 阶段一: `go test ./...` 通过;`make seed` 通过并确认本地 MySQL `task_reviewers` 中官方任务已分配 `reviewer1`。

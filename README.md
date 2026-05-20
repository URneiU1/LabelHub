# LabelHub · 数据标注平台

字节 AI 全栈挑战赛课题 — 覆盖「数据生产 → AI 预审 → 人工审核 → 多格式导出」全生命周期的 Web 数据标注平台。

## 快速启动

```bash
# 1. 起基础设施(MySQL + Redis + Adminer)
make up

# 2. 安装依赖
make install

# 3. 跑 API(终端 A,复用或新开)
make api                 # http://localhost:8080/health

# 4. 跑前端(终端 B,新开)
make web                 # http://localhost:5173

# Adminer(DB GUI):    http://localhost:18080  (server=mysql, user=labelhub, pass=labelhub_dev)
```

Seed 数据将在 Sprint 1 实现。

## 架构

```
apps/web      — React 18 + TypeScript + Semi Design (单一 SPA,角色路由)
apps/api      — Go + Gin REST API (port 8080)
apps/ai-worker — Go Asynq AI 预审 Worker
```

详见 `docs/PLAN.md`。

# LabelHub · 数据标注平台

字节 AI 全栈挑战赛课题 — 覆盖「数据生产 → AI 预审 → 人工审核 → 多格式导出」全生命周期的 Web 数据标注平台。

## 快速启动

```bash
# 1. 起基础设施
make up

# 2. 安装依赖
make install

# 3. 跑 API
make api

# 4. 跑前端
make web

# 5. 种子数据(初次使用)
make seed
```

## 架构

```
apps/web     — React 18 + TypeScript + Semi Design (单一 SPA,角色路由)
apps/api     — Go + Gin REST API
apps/ai-worker — Go Asynq AI 预审 Worker
```

详见 `docs/PLAN.md`。

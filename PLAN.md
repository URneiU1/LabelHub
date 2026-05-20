# LabelHub 数据标注平台 · 全栈实现 Plan

## Context

**Why**:**字节 AI 全栈挑战赛**课题,业务方张满 + 主持人李金曦,优秀者通校招绿色通道。从零构建一套 Web 数据标注平台 LabelHub,覆盖「数据生产 → AI 预审 → 人工审核 → 多格式导出」全生命周期,三端 + 一个 AI 审核 Agent。验收 功能完备性 60% + 工程质量 25% + 产品体验 15% 三段加权,目标冲完整 100%。

**真实业务背景**:大模型公司(Manus、智慧医疗等)用基础模型 + 垂直领域数据训练垂直能力,数据标注是关键供应链环节。平台 = 这个生产线的工厂。

**用户已确认约束**:
- 后端 **Go + Gin + GORM**(Python/Java 主力,Go 是新栈,预留 3-5 天学习曲线)
- LLM **豆包 Doubao-Seed-2.0-lite**(EP / APIKEY 由参赛方提供,只放本地 `apps/ai-worker/.env`,**禁止入 Git / 文档 / 截图**;走 OpenAI 兼容协议 + Function Calling;只接豆包不抽象多 provider)
- 仓库 **Monorepo,pnpm workspace + go.work**,**GitHub 私有**
- 时间预算 **7-8 周**(deadline 2026-07-08 ~ 07-15),投入 **25-30h/周**,冲完整 60+25+15%(含动效 + 移动适配)
- 数据库 **MySQL 8**,队列 **Redis + Asynq**
- 前端 React 18 + TS + Semi Design + **Formily 2** + **@dnd-kit/core** + Zustand + TipTap
- **UI 风格 Editorial / Magazine**(浅米色背景 + Serif 标题 + 黑色细线 + 现代粘接);**UI 文案中文**(中国比赛)
- **独狼项目**(只有用户 + Claude),不需要 PR review 环节
- **演示环境**:课题方不提供 VPS,用户自行解决;Sprint 5 末交付 `docker-compose.prod.yml`,VPS 上线由用户后续决定
- **测试覆盖**:关键路径(状态机 / verdict 决策 / 导出器)100% 单测 + 主流业务闭环集成 test;不追求样本覆盖率数字
- **AI Coding 过程文档**:以 git commit 为主轴,提炼"重要决策点 + Claude 怎么参与"成 8-15 页 PDF
- **Designer 物料**:**核心 9 + 加分 2 分批**(下文 §0 分层)。核心 9:ShowItem / Input / TextArea / Radio / Checkbox+Tags / RichText TipTap / **FileUpload** / **JSONEditor** / **LLMTrigger**。加分 2:Tabs / Group

**主线场景**:**官方 qa_quality 质检标注数据集**(30 条,11 个 category,含 text / video / image / markdown 题目;反向锁定 schema / AI 维度 / ShowItem 多媒体渲染 / 导出字段 / demo 剧本)。官方数据比自造 AIGC 视频更贴合评委预期,仍能覆盖嵌套 JSON + 多维度评分 + 富文本理由 + 媒体展示

**Source**:`~/Desktop/LabelHub_materials/LabelHub 数据标注平台 · AI全栈课题实现要求.pdf`(13 页,Day 1 前从 repo 目录移出)+ 宣讲会逐字记录(2026-05-20)+ 官方测试数据 `tools/seed/datasets/{qa_quality,preference_compare}`

**最终交付物**(放仓库 `submission/` 目录下):
1. 源码 Monorepo(前端 + 后端 + Agent)
2. README:架构 / 模块划分 / 本地启动 / 关键取舍
3. 演示视频 5-10 分钟,覆盖三角色完整链路
4. 相关文档:架构图 + 关键技术点 + Demo 截图 + AI Coding 过程记录 + 基础技术文档
5. 可访问演示环境说明文档(用户自行 VPS)
6. API 文档(Postman Collection / 飞书文档 / Markdown 任选)

---

## 0. 交付分层:必拿 vs 加分

> **核心理念**:先做能拿满 60+25%(共 85 分)的必拿闭环 + 必备工程质量;15% 体验和"任选其一"的加分项放到后期,**进度紧时可砍**。这样最差情况也保 85 分,不会因为做加分项把必拿闭环搞断。

### 必拿基线(Sprint 0-5 完成)— 目标 85+ 分

| 模块 | 必拿内容 |
|---|---|
| Owner 后台 | 任务 CRUD + 模板 Designer 搭建 + 题目导入(JSON 一种)+ 发布/暂停/结束 + 数据看板基础(进度 / 通过率)+ AI Prompt 配置 + baseline 配置 + **黄金数据集 dry-run**|
| Labeler 工作台 | 任务广场 + 领取(`first_come` 一种)+ 作答页 + 草稿自动保存 + LLM 触发 + 提交校验 + 修订流 + 我的贡献 |
| Reviewer 审核 | 审核队列 + 详情 + 通过/打回/修订 + 打回理由 + 上一轮意见 + 审计时间线 |
| AI Agent | 异步队列 + Function Calling + 维度评分 + 幂等 + 重试 + DLQ + 人工兜底 + mock provider |
| 状态机 | 任务 4 态 + 提交 **7 态(简化版,见 §4)** + `ai_verdict` 拆为字段 + 审计日志 + outbox 一致性 + 超时清扫 |
| 导出 | JSON / JSONL / CSV / XLSX **4 种全做**(PDF 明文必做)+ 字段映射 + 异步 + 下载历史 |
| 工程质量 | 关键路径单测 100% + 主流程集成 test + README + ARCHITECTURE.md + DEPLOY.md + API 文档(swag + Postman) + docker-compose.prod.yml |
| 产品体验 | Editorial 主风格 + Skeleton + 错误友好 + 1280×800 / 1920×1080 适配 + 三栏可折叠 + **信息密度优先 表格效率 状态清晰**(不堆动效) |
| Designer 物料 | **9 个核心**:ShowItem / Input / TextArea / Radio / Checkbox+Tags / RichText TipTap / FileUpload / JSONEditor / LLMTrigger(PDF 明文"至少实现以下物料"含 JSON 编辑器 / 文件图片上传 / LLM 交互组件,必拿)|
| 主线样例 | **官方 qa_quality 质检标注**(30 条,含 text / video / image / markdown;ShowItem 多媒体展示 + 多维度评分 + JSONEditor 修正答案 + 富文本修订建议 + LLM 触发,1 个完整任务即证明平台跑通)|

### 加分项(Sprint 6-7 选做,**进度紧时按下面优先级砍**)

| 优先级 | 项目 |
|---|---|
| ★★★ | **Designer 加分物料**:Tabs(多 Tab 容器)/ Group(分组容器)— 共 2 个,Sprint 6 补齐 |
| ★★★ | **Markdown 导出器**(张满宣讲会强调对接模型训练)|
| ★★ | **三种分发策略**:`assigned` + `quota` 模式(`first_come` 必拿,这两个加分)|
| ★★ | **JSONL / Excel 题目导入**(JSON 必拿,这两个加分)|
| ★★ | **第二官方任务样例 seed**:`preference_compare` 偏好对比标注(12 条,纯文本 A/B/tie 对比,作为 Sprint 6/7 加分任务)|
| ★ | **暗色模式**(Semi 自带,改色板即可)|
| ★ | **复杂动效**(Framer Motion stagger / 抽屉滑入)|
| ★ | **移动端响应式**(Labeler 作答页)|
| ★ | **asynqmon 暴露给评委**(加密码后)|
| ★ | **键盘快捷键**(J/K/A/R/B)|

**砍项规则**(进度紧时从下往上砍):动效 → 移动端 → 暗色模式 → 第二官方任务样例(只留 qa_quality)→ 三种分发(只保 first_come)→ JSONL/Excel 导入 → Markdown 导出 → 加分物料(Tabs / Group)。**黄金数据集 dry-run 不砍**(AI Agent 亮点),**4 种导出格式不砍**(PDF 明文必做),**JSONEditor 不砍**(官方两份任务都用到结构化批注 / 修正答案)。

---

## 0.1 安全与机密管理(执行强约束)

**红线**(违反 = 课题资料外泄风险):
- 豆包 APIKEY 和 EP **不进 Git**,只放本地 `apps/ai-worker/.env` + `apps/api/.env`,必须列入 `.gitignore`
- 仓库 root 放 `.env.example`,用占位符 `DOUBAO_API_KEY=your-key-here`
- README / ARCHITECTURE / DEPLOY 文档**不写真实 key**,只写"key 由参赛方提供"
- 演示视频录屏时 terminal 不显示包含 key 的命令(用 `direnv` 自动加载 `.env`)
- **不上传 PDF 原文到仓库**(原文含 key 截图)
- VPS 部署 `.env` 用 sftp 上传后 `chmod 600`,不放 Git
- 演示用户名/密码用 seed 弱密码,但 UI 提示"演示账号"

---

## 1. 总体架构

### 1.1 Monorepo 布局

```
~/Desktop/LabelHub/
├── apps/
│   ├── web/                    # 单一 React SPA,角色路由分区
│   │   └── src/
│   │       ├── modules/{owner,labeler,reviewer,auth}/
│   │       ├── designer/       # ★ Schema Designer(物料 / 画布 / 属性面板)
│   │       ├── renderer/       # ★ Schema Renderer(运行时表单)
│   │       └── shared/
│   ├── api/                    # Go + Gin REST API server
│   │   └── internal/{handler,service,repo,model,middleware,statemachine,exporter,mq}/
│   └── ai-worker/              # Go AI 预审 worker(独立进程)
│       └── internal/{consumer,llm,prompt,scoring}/
├── packages/
│   ├── shared-types/           # TS 类型(API、Schema、枚举)
│   ├── schema-spec/            # ★ JSON Schema 规范 + ajv 校验 + 版本迁移
│   ├── ui-kit/
│   └── api-client/             # OpenAPI 生成的 fetch 客户端
├── tools/{openapi,seed,scripts}/
├── deploy/{docker-compose.yml, Dockerfile.api, Dockerfile.worker, Dockerfile.web}
├── docs/{PLAN.md, ARCHITECTURE.md, API.md, DEPLOY.md}
├── pnpm-workspace.yaml
├── go.work                     # apps/api + apps/ai-worker
└── README.md
```

**关键设计取舍**:
- **单 SPA 而非拆三端 web**:节省 60% 工作量;通过路由前缀 + 角色守卫 + Sidebar 切换隔离;三模块共享 80% 物料(表格/筛选/详情)。
- **worker 独立进程**:LLM 调用阻塞 IO + 重试场景,放在 api 进程会拖慢 HTTP;独立后可水平扩。
- **不引入 RabbitMQ/Kafka**:演示环境 docker-compose 一把梭最重要,Redis + Asynq 足够。
- **AI Agent 有独立 system 账户**:每次审核写 `audit_logs.actor_type='ai_worker'`,可追溯,符合 PDF "AI Agent 需独立账户视角"要求。

### 1.2 服务通信

| 组件 | 入 | 出 |
|---|---|---|
| `web` (Vite SPA) | - | HTTP REST → api;前端 5s 轮询拿 AI 进度(不上 WS,够用且好讲) |
| `api` (Gin) | web | MySQL + Redis(Asynq pub)+ 静态文件 fs |
| `ai-worker` (Asynq consumer) | Redis | 豆包 HTTP + MySQL(直写 ai_reviews + 触发状态机) |

---

## 2. 数据模型(MySQL 8,**17 张表**)

完整字段定义见 Sprint 1 第一步要创建的 `apps/api/internal/migration/*.sql`,这里给关键关系。

| 表 | 关键字段 / 索引 | 备注 |
|---|---|---|
| `users` | id, username UK, password_hash, display_name, email, status ENUM('active','disabled') | **不再有单一 role 字段** — 角色用下一行多对多表 |
| `user_roles` | id, user_id, role ENUM('owner','labeler','reviewer','admin','system'); UK(user_id, role) | **多对多**。张满明确说"人工审核员可能跟任务负责人是有重叠的",Owner 兼 Reviewer 必须支持。AI agent 是 `system` 角色 |
| `tasks` | id, owner_id, title, description TEXT, rich_description JSON(TipTap), tags JSON, reward_config JSON, **baseline_description TEXT**, status ENUM('draft','published','paused','archived'), template_id, distribution ENUM('first_come','assigned','quota'), ai_review_enabled, human_review_enabled, ai_prompt_id, total_items, finished_items, deadline | **新增 `baseline_description`** 作为 AI Agent 审核上下文(张满宣讲会强调);**golden 样例不在此表,走下方 `golden_samples` 多对一**(避免双数据源)。`idx_published(status, published_at)` 给广场用。`ai_review_enabled` 发布后变更只影响**新提交 / 新 revision**,不 retroactively 重排已在 `ai_reviewing/human_reviewing/approved/rejected` 的 submission |
| `task_templates` | id, task_id, version, schema_json, schema_hash, field_map; UK(task_id, version) | **版本化**:已发布后改模板创建新 version |
| `task_items` | id, task_id, external_id, payload JSON, status ENUM, claimed_by, claimed_at; UK(task_id, external_id), `idx_claim(task_id, status, claimed_by)` | 原始题目数据 |
| `task_assignees` | task_id, user_id, item_id NULL; UK | assigned/quota 模式分发关系 |
| `submissions` | id, task_id, item_id UK, template_version, labeler_id, current_revision_id, status ENUM(**7 种**,见 §4), **ai_verdict ENUM('pass','reject','uncertain') NULL**, ai_score DECIMAL NULL, human_verdict ENUM('approve','reject','revise') NULL | item 与 submission 一对一,修订走 revisions。**ai_verdict 是字段不是状态**;**导出不改 status**,只新增 exports 记录。`human_verdict` 是当前快照字段,真相来源是 append-only `human_reviews`;二者必须在同一事务内更新 |
| `submission_revisions` | id, submission_id, revision_no, answer JSON, draft BOOL; UK(submission_id, revision_no) | append-only,每轮快照 |
| `ai_reviews` | id, submission_id, revision_id, **idempotency_key CHAR(64) UNIQUE**, prompt_version, verdict, overall_score, dimensions JSON, raw_response, retry_count, status, error_msg | append-only |
| `human_reviews` | id, submission_id, revision_id, reviewer_id, stage ENUM('first','second','final'), verdict, reason, patch JSON | append-only |
| `audit_logs` | id, entity_type, entity_id, from_state, to_state, actor_type, actor_id, event, payload JSON, created_at TIMESTAMP(3); **INDEX(entity_type, entity_id, created_at)** | **核心追溯表**,所有状态迁移必须写一条。复合索引给审核时间线和实体详情页用 |
| `uploaded_files` | id, task_id, submission_revision_id NULL, storage_key UK, original_name, mime_type, size_bytes, status ENUM('temp','attached','deleted'), created_by, created_at, attached_at NULL; INDEX(status, created_at), INDEX(task_id) | FileUpload 元数据表。上传先 `temp`,提交 revision 时标 `attached`,每日 cron 清理超过 24h 的 temp 孤儿文件 |
| `exports` | id, task_id, format, filter, field_map, include_reviews, status, file_path, row_count | 异步导出任务 |
| `ai_prompt_configs` | id, task_id, version (即 prompt_version), prompt_template, dimensions JSON, pass_threshold, uncertain_min, model | 维度与阈值 |
| `golden_samples` | id, task_id, ai_prompt_id NULL(可选关联到某版 Prompt), payload JSON, expected_answer JSON, expected_verdict ENUM('pass','reject','uncertain'), notes TEXT, created_by, created_at; **INDEX (task_id), UK (task_id, payload_hash)** | **新增表** — Owner 上传的"已知答案"样例,供 dry-run 时跑 AI 看与 expected_verdict 的偏差。`payload_hash` 是 sha256(payload) 防重复。配 §3 Sprint 3 的 dry-run-with-golden API |
| `ai_dry_runs` | id, task_id, ai_prompt_id, status ENUM('queued','running','succeeded','failed'), result JSON, error_msg, created_by, created_at, finished_at NULL; INDEX(task_id, created_at) | Golden dry-run 结果表。API 只入队并返回 dry_run_id,worker 调豆包后写回 result,避免 LLM 直接跑在 api 进程 |
| `outbox_events` | id, topic, payload, status ENUM('pending','published','failed'), retry_count | 事务一致性出口表 |

**外键策略**:`ON DELETE RESTRICT`,审计表禁删。

---

## 3. JSON Schema 设计(★ 核心难点 1)

### 3.1 Schema 结构
- 顶层是"页面 schema"而非单一字段,支持 Tab/分组嵌套。
- `widget` 字段对应物料 ID,与 Formily `x-component` 1:1 映射。
- 校验 / 联动用 **JSON 表达式**(Formily 原生 `x-reactions` + `x-validator`),禁止字符串 eval。
- **ShowItem**:`widget: 'ShowItem'`,`x-show-path` 指向 `task_item.payload.*`,`x-exportable: false`,不写回 form values。**必须支持 polymorphic 原始数据渲染**:
  - `media_type='text'` 或普通 string → 纯文本 / 代码块展示
  - `media_type='video'` → `<video controls>` 播放器(官方 `qa_quality` 含公开视频 URL)
  - `media_type='image'` → `<img>` 预览 + 链接兜底
  - `media_type='markdown'` → Markdown 渲染(允许内嵌 image/video 链接)
  - object / array → 树形折叠或紧凑 JSON 视图
  ShowItem 内部用 `mode='auto'` 自动判断,也允许 Designer 显式配置 `mode='text'|'video'|'image'|'markdown'|'json'`。复用 `@uiw/react-json-view` + `react-markdown`。
- **LLMTrigger**:`widget: 'LLMTrigger'`,运行时点按钮 → 前端调 `/api/llm/inline` → `form.setFieldState(target, ...)`。
- **FileUpload**:上传走 `POST /api/v1/uploads`,限制 `maxSize=10MB`(图片 5MB),MIME 白名单 `image/png,image/jpeg,image/webp,application/pdf,text/plain,application/json`。后端写 `uploaded_files.status='temp'`;表单提交时把 file id 写入 answer 并在同一事务标 `attached`;cron 每日清理超过 24h 未 attached 的 temp 文件。下载/预览必须校验当前用户对 task/submission 的访问权限。

完整示例以官方 `qa_quality` 为主:Sprint 1 手写 `tools/seed/templates/qa_quality_review.json`,Sprint 2 Designer 的目标是拖拽重建它。Schema 覆盖:ShowItem 多媒体展示 + Radio 评分 + Tags 问题类型 + Input 一句话总评 + TextArea 详细评语 + RichText 修订建议 + JSONEditor 修正后答案 + FileUpload 证据素材 + LLMTrigger AI 预评分 + 字段联动(`x-reactions.dependencies + when`)+ 自定义校验(`requiredWhen`)。

### 3.2 Designer 三栏
- **左侧物料区**:`materialRegistry.list()`,每个物料预定义默认 schema 片段。
- **中间画布**:`@dnd-kit/core` sortable 递归渲染 schema 树,选中节点高亮。
- **右侧属性面板**:**meta-circular** — 用 Renderer 自己渲染每个物料的"配置 schema"来编辑业务 schema。这是减少 30% 代码量的关键技巧,**若 Sprint 2 中段实现卡住,降级为每物料一份硬编码 React 表单**(代码量翻倍但不会失败)。

### 3.3 Renderer
- `@formily/react` 的 `createSchemaField({ components })` 注册所有物料 + 自定义 ShowItem/LLMTrigger。
- 拉 `task_item.payload` + `task_template.schema_json` + 已有 `submission_revisions.answer`(草稿)→ 初始化 form。
- 提交时序列化跳过 `type=void` 与 `x-exportable=false`。

### 3.4 Schema 版本管理
- 已发布任务 → Owner 改模板 → 创建新 `task_templates.version`,**旧 submissions.template_version 不变**。
- Renderer 渲染历史 submission 时按其 `template_version` 找 schema(保证审核员看到当时的形态)。
- 新认领的 item 用最新 version。
- `packages/schema-spec` 提供 `migrate(oldSchema, newSchema, oldValues) → newValues`,MVP 只做"同名字段保留,新增空,删除丢弃"。

---

## 4. 状态机 + 一致性

### 4.1 Task 状态机
`draft ──publish──► published ──pause──► paused ──resume──► published ──archive──► archived`

### 4.2 Submission 状态机(**7 态简化版**)

> **设计要点**(吸纳用户 review):AI 预审的结果(pass/reject/uncertain)是**审核意见**而非流程状态,所以**从状态机里拆出来**,放到 `submissions.ai_verdict` 字段。**`exported` 也不是状态** — 一个 approved submission 可能被多次导出成不同格式 / 不同字段映射,导出历史完全由 `exports` 表记录,submission 终态停在 `approved`。

**7 个状态**:`draft` / `submitted` / `ai_reviewing` / `human_reviewing` / `revising` / `approved` / `rejected`

```
draft ──submit──► submitted
                      │
            ┌─────────┴──────────┐
            │ ai_enabled         │ !ai_enabled
            ▼                    ▼
       ai_reviewing  ──────► human_reviewing
       (ai 写完 verdict 后)        │
       根据 verdict + threshold:    │ approve / reject / revise
       - pass + !human_review        │
         → approved (终态)           ▼
       - pass + human_review     approved (终态) / rejected (终态) / revising
       - reject / uncertain          │
         → human_reviewing           │
                                     │
revising ──submit──► submitted(新 revision_no,清空 ai_verdict 重走)
```

**终态**:`approved` 和 `rejected`。导出动作不改 submission 状态,只在 `exports` 表新增一条记录。

### 4.3 完整迁移表(7 态)

| from | event | to | 触发方 | 守卫 / 副作用 |
|---|---|---|---|---|
| draft | save | draft | labeler | 写新 revision draft=true |
| draft | submit | submitted | labeler | 校验通过,revision draft=false |
| submitted | enqueue | ai_reviewing | api(submit 同事务出 outbox) | task.ai_review_enabled=true |
| submitted | skip_ai | human_reviewing | api | task.ai_review_enabled=false |
| ai_reviewing | ai_done | approved / human_reviewing | ai-worker | 写 ai_verdict + ai_score,根据 verdict + threshold + task.human_review_enabled 决策:`verdict='pass' && !human_review` → approved;其他 → human_reviewing |
| ai_reviewing | ai_fail_max | human_reviewing | ai-worker | 写 `ai_verdict='uncertain'` + error;强制人工兜底 |
| human_reviewing | approve | approved | reviewer | 写 human_verdict='approve';**终态** |
| human_reviewing | reject | rejected | reviewer | 写 human_verdict='reject' + reason;**终态** |
| human_reviewing | revise | revising | reviewer | 写 human_verdict='revise' + reason |
| revising | submit | submitted | labeler | 新 revision_no,清空 ai_verdict 重走 |

完整迁移表在 `apps/api/internal/statemachine/machine.go` 以 `map[StateEvent]Transition` 形式落地,**100% 单测覆盖** + 非法迁移返回 error。导出动作不经过状态机,直接走 `exports` 表 + `audit_logs.event='exported'`。

### 4.4 状态 vs 字段拆分的好处
- 状态机从 11 → 7 个状态,测试用例减少 ~35%
- AI 结果可以独立查询:`SELECT * FROM submissions WHERE ai_verdict='uncertain' AND status='human_reviewing'`(过去要分别查多个状态)
- Dashboard 指标更直接:AI 通过率 = `COUNT(ai_verdict='pass') / COUNT(*)`
- 答辩讲解:状态 = "等谁做事",字段 = "事情做完是什么结论",exports 表 = "数据被怎么用了"

### 4.5 事务一致性:**单库 outbox 模式**
- `submitted → ai_reviewing` 在 **一个 MySQL 事务** 内完成三件事:更新 submissions.status + 写 audit_log + 写 outbox_events(`topic='ai:review'`)。
- 独立 outbox publisher goroutine(api 进程内启动),轮询 `outbox_events.status='pending'` → 发 Asynq → 标记 `published`。
- ai-worker 用 `idempotency_key` 兜重复消费(`INSERT ... ON DUPLICATE KEY UPDATE`)。
- **超时清扫 cron**:api 内每分钟扫 `status='ai_reviewing' AND updated_at < now()-10m` 的 submission,重排或在 ai_verdict 写 'uncertain' 后转 human_reviewing,防止 worker crash 后卡死。
- **first_come 抢单并发策略**:claim API 必须使用 MySQL 事务 + 行锁,优先方案为 `SELECT id FROM task_items WHERE task_id=? AND status='available' ORDER BY id LIMIT 1 FOR UPDATE SKIP LOCKED`,随后更新 `claimed_by/claimed_at/status='claimed'`;若 MySQL 环境不支持 `SKIP LOCKED`,降级为 `UPDATE ... WHERE status='available' LIMIT 1` 后检查 affected rows。禁止先查后写的非原子抢单。
- **AI 审核开关变更规则**:`task.ai_review_enabled` 发布后可切换,但只影响 toggle 之后的新提交 / 新 revision;已进入 `ai_reviewing` 的 submission 继续完成 AI 流程,已进入 `human_reviewing/approved/rejected` 的 submission 不回滚、不重排,所有 toggle 写 `audit_logs`。

---

## 5. AI Agent 异步队列(★ 核心难点 2)

### 5.1 选型:**Asynq**(Redis-backed)
- Go 原生 + asynqmon dashboard(答辩可视化加分项)+ unique task / retry / DLQ 开箱即用。
- 部署只需 Redis(已用)。

### 5.2 Function Calling 工具定义

**不能硬编码评分维度 enum**。Owner 可在 `ai_prompt_configs.dimensions` 中配置任意维度(如"完整性"),所以 worker 每次调用前必须从当前 prompt config 动态生成 Function Calling tool schema:

```json
{
  "name": "submit_review",
  "parameters": {
    "type": "object",
    "required": ["dimensions", "overall_score", "verdict", "reason"],
    "properties": {
      "dimensions": {
        "type": "array",
        "items": {
          "required": ["name", "score", "reason"],
          "properties": {
            "name": {
              "type": "string",
              "enum": "<dynamic: ai_prompt_configs.dimensions[].name>"
            },
            "score": {"type": "number", "minimum": 0, "maximum": 10},
            "reason": {"type": "string", "maxLength": 200}
          }
        }
      },
      "overall_score": {"type": "number", "minimum": 0, "maximum": 100},
      "verdict": {"enum": ["pass", "reject", "uncertain"]},
      "reason": {"type": "string", "maxLength": 500}
    }
  }
}
```

- 调用时强制 `tool_choice: {"type":"function","name":"submit_review"}`。
- `payload.expected_dimensions` 只作为**本题重点维度提示**和 prompt 上下文;返回值仍必须落在 `ai_prompt_configs.dimensions[].name` 动态 enum 内。若 expected dimension 不在 Owner 配置里,worker 在 prompt 中提示但不加入 schema enum,避免污染配置源。
- 拿到结构化 JSON 后用动态 schema 做 `gojsonschema` 校验;不合规写入 raw response + schema version + error,然后 retry。
- Worker 端用 `dimensions × 权重` 再算一遍 `overall_score`,并以 **worker 重算结果 + Owner 阈值**作为最终 verdict 来源。若 LLM 自报 `verdict='pass'` 但 worker 重算低于通过阈值,以 worker 判定为准转 `uncertain/reject`;若偏差 > 15% 直接标 `uncertain` 并记录 `verdict_overridden=true`。

### 5.3 幂等 + 重试

> **幂等的真实含义**(张满宣讲会原话):"幂等不是指完全相等,但从方向向量上一定要保持一致" — 即同一份输入不能产生两条独立的入库记录、verdict 不能反复横跳;**不是要求 LLM 输出 byte-equal**(LLM 本身有 temperature)。

- `idempotency_key = sha256(submission_id + ":" + revision_id + ":" + prompt_version)`。
- 入队 Asynq 用 `asynq.Unique(24h)` + 同 key 去重。
- DB 层 `ai_reviews.idempotency_key UNIQUE` 兜重复消费。
- max retries = 5,backoff 指数(1s/5s/30s/2m/10m/1h);超出 → DLQ + `ai_reviews.status='dead'` + submission 写 `ai_verdict='uncertain'` 并迁移到 `human_reviewing` 强制人工兜底。
- 单次 LLM 调用 30s timeout,handler 总 60s。
- **熔断**:连续 20 个 5xx → 短路 5 分钟,期间任务直接 ack 为 `uncertain`(写 audit_log),防雪崩。
- **离线 fake LLM**:`LLM_PROVIDER=mock` 环境变量,演示日断网时切本地 mock(按答案长度 + 关键词随机评分),保 demo 不挂。

---

## 6. 核心 API(全部前缀 `/api/v1`,JWT Bearer)

### 6.1 API 基础约定(Sprint 1 起执行)

- **统一响应 envelope**:
  - 成功:`{"data": ..., "request_id": "..."}`
  - 失败:`{"error": {"code": "VALIDATION_ERROR", "message": "中文可读提示", "details": {...}}, "request_id": "..."}`
- **错误码从 Sprint 1 开始定义**,禁止前 4 个 Sprint handler 返回 ad-hoc JSON。首批 code:`VALIDATION_ERROR / UNAUTHORIZED / FORBIDDEN / NOT_FOUND / CONFLICT / INVALID_STATE / RATE_LIMITED / INTERNAL_ERROR / LLM_UNAVAILABLE`。
- **分页策略**:列表 API 默认 cursor pagination,请求参数 `?cursor=&limit=20`,最大 `limit=100`;响应 `{"data":[...],"page":{"next_cursor": "...","has_more": true},"request_id":"..."}`。后台管理表格可以支持 `sort` / `order` / `filters`,但不使用 offset 作为主分页策略。
- **CORS**:dev 允许 `http://localhost:5173` → `http://localhost:8080`,允许 `Authorization, Content-Type`,credentials=false;prod 只允许正式域名。

| 模块 | endpoint | 备注 |
|---|---|---|
| 鉴权 | `POST /auth/login`、`POST /auth/refresh`、`GET /auth/me`、`POST /auth/logout` | JWT HS256,access 2h + refresh 14d |
| Task | `GET /tasks`、`POST /tasks`、`GET/PATCH /tasks/:id`、`POST /tasks/:id/{publish,pause,resume,archive}`、`GET /tasks/:id/dashboard` | Owner 全套 |
| Template | `GET/POST /tasks/:id/templates`、`GET /templates/:id`、`POST /templates/:id/validate`、`POST /templates/:id/preview-with-item` | Designer 用 |
| Items | `POST /tasks/:id/items/import`(multipart, json/jsonl/xlsx)、`GET /tasks/:id/items`、`GET /tasks/:id/items/:itemId`、`PATCH /tasks/:id/items/batch` | 题目导入 + 题目详情 |
| 广场 / 领取 | `GET /plaza/tasks`、`POST /tasks/:id/claim`、`POST /items/:id/release` | Labeler |
| Labeler 作答 | `GET /tasks/:id/next-item`、`GET /items/:id/answer`、`PUT /items/:id/draft`(3s debounce)、`POST /items/:id/submit` | |
| 我的数据 | `GET /me/submissions` | Labeler 查看已提交 / 通过 / 打回 / 待修改列表 |
| 上传 | `POST /uploads`、`GET /uploads/:id` | FileUpload 通用接口;maxSize/MIME/权限/孤儿清理见 §3.1 |
| LLM 内联 | `POST /llm/inline` body=`{promptRendered, model, taskId, itemId}` → string | 限流 + 计费日志 |
| AI Prompt | `GET/POST /tasks/:id/ai-prompts`、`POST /tasks/:id/ai-prompts/:promptId/dry-run`、`GET /ai-dry-runs/:id` | Owner 配置 + 异步试运行;POST 返回 `dry_run_id`,worker 写结果 |
| **Baseline / Golden** | `PATCH /tasks/:id/baseline`(更新 `baseline_description`)、`GET/POST /tasks/:id/golden-samples`、`PATCH/DELETE /golden-samples/:id`、`POST /tasks/:id/ai-prompts/:promptId/dry-run-with-golden`、`GET /ai-dry-runs/:id` | **AI Agent 亮点** — API 只入队,worker 跑 golden 样例,前端轮询 expected vs actual 对比表 |
| AI Review | `GET /submissions/:id/ai-reviews`、`POST /submissions/:id/retry-ai` | 只读 + 手动重试 |
| Human Review | `GET /reviews/queue`、`GET /submissions/:id`、`POST /submissions/:id/review`、`POST /reviews/batch` | 队列 + 批量 |
| Audit | `GET /audit-logs?entityType&entityId` | 时间线渲染 |
| 导出 | `POST /tasks/:id/exports`、`GET /exports?taskId`、`GET /exports/:id`、`GET /exports/:id/download` | 异步 + 历史 |
| 用户 | `GET/POST /users` (admin) | MVP 简化 |

详细 OpenAPI 在 `tools/openapi/openapi.yaml`,Sprint 5 末用 swag 反向生成 / 手写补全。

---

## 7. Sprint 切分(8 周 × 30h)

> **每个 Sprint 周五必须能跑 demo**(答辩准备的复利来源)。

### Sprint 0 — Go 学习 + 脚手架 + Editorial 风格定型(Week 1 前半,3-5 天)
- Day 1-3:Tour of Go + Gin + GORM + Asynq Hello-world(timebox,第 4 天起强制写业务代码)。
- Day 4-5:monorepo 结构落地;`pnpm init -w`、`go work init`;docker-compose 起 mysql:8 + redis:7 + adminer + asynqmon;api `GET /health`;ai-worker 消费空任务;web Vite + Semi + 路由骨架;GitHub 私有 repo + CI。
- **Editorial 风格定型**(半天,与脚手架并行):在 `apps/web/src/styles/tokens.css` 定义 design token(色 / 字 / 间距 / 圆角)+ 写 1 个 `MagazineCard` 样例组件 + 在 Storybook(可选)或一个独立 demo 页 `/style-guide` 上展示;落定后 Sprint 1-7 不再大改风格。
- **完成标准**:`docker-compose up` 一把起;能登录(写死账号)看到三角色空白首页;`/style-guide` 页面展示 Editorial 风格的 Button / Card / Form / Table / Modal / Tag 6 个核心组件,无默认 Semi 蓝。

### Sprint 1 — 两段式 MVP + **官方 qa_quality 主线样例** + **Renderer 先行**(Week 1 后半 + Week 2,~10 天)

> **关键调整**:主线样例任务使用课题方官方 `qa_quality` 数据集,从 Sprint 1 开始就反向锁定 schema 结构 / AI 评分维度 / ShowItem 多媒体展示 / 导出字段 / demo 剧本。Renderer 先于 Designer,**先用手写 JSON schema 跑通整个端到端**,Sprint 2 再做拖拽 Designer。这样即使 Designer 卡住,平台主链路不会断。

**Sprint 1a — 后端闭环(前 5 天,先把数据流打通)**:
- **17 张表** migration、JWT 鉴权(login/refresh/me/logout)+ 角色守卫 + user_roles 多对多
- 统一 API envelope / 错误码 / CORS / request_id middleware 从第一天落地
- **7 态状态机引擎**(`statemachine/machine.go`,迁移表 100% 单测)、audit_log 写入、human_reviews 与 submissions.human_verdict 同事务更新
- API 最小集:tasks 只读/创建、items JSON 导入、`first_come` claim(带行锁)、draft、submit、review、upload、`/me/submissions`、同步 JSON export
- `/llm/inline` 用 mock 返回(根据 prompt 长度返回 fixed string + 模拟 800ms 延迟),Sprint 3 替换为真实豆包调用
- **官方 qa_quality 主线 seed**:
  - 数据源:`tools/seed/datasets/qa_quality/json/qa_quality.json`(30 条),导入为 1 个完整任务
  - handcoded JSON Schema 在 `tools/seed/templates/qa_quality_review.json`
  - **单页顺序布局**(不用 Tabs / Group 这种加分容器物料,只用核心 9 物料):ShowItem 渲染 prompt / model_answer / media_url / markdown;Radio 4 维度 1-5 分评分;Checkbox+Tags 标问题类型;Input 一句话总评;TextArea 详细评语(打回必填);RichText 修订建议;JSONEditor 修正后答案;FileUpload 证据素材;LLMTrigger AI 预评分(Sprint 1 用 mock /llm/inline,Sprint 3 切真实豆包)
  - Owner 配 baseline:按 `tools/seed/datasets/qa_quality/标注要求.md` 摘要生成,并允许后续编辑
  - 保留每题 `payload.expected_dimensions` 原样入库,供 AI Prompt 模板和 Dashboard 使用
- Seed 账号:2 owner + 3 labeler + 2 reviewer(含 1 个 Owner 兼 Reviewer,验证 user_roles 多对多)

**Sprint 1b — 前端 Renderer 闭环(后 5 天,不碰 Designer)**:
- 前端:登录页 + Owner 任务详情(基于 seed,只需展示/基础编辑 baseline)+ Labeler 广场/作答页(**Renderer 渲染手写 schema**)+ Reviewer 队列/详情
- Renderer MVP:ShowItem polymorphic 展示、Radio、Checkbox+Tags、Input、TextArea、JSONEditor(简单 JSON textarea + parse 校验)、RichText(TipTap 最小 toolbar)、FileUpload(上传/attach,先不做复杂预览)、LLMTrigger(mock)
- 联动/校验只做主线必需:必填、长度、打回时详细评语必填;复杂 requiredWhen/正则可推到 Sprint 2

**Sprint 1 显式不做**:Designer 拖拽、真实 AI 预审、异步导出、4 格式导出、复杂上传预览、任务批量编辑、JSONL/Excel 导入、Tabs/Group。

**Sprint 1 砍项表(若 10 天吃紧,从上到下砍,但主链路不断)**:
1. FileUpload 只保后端接口 + 前端字段保存 file id,预览推到 Sprint 2。
2. RichText 降级为 TextArea 存 HTML/string,TipTap 工具栏推到 Sprint 2。
3. ShowItem markdown 先渲染为文本,`react-markdown` 推到 Sprint 2。
4. Owner 任务创建 UI 只保 seed 任务详情;完整任务创建表单推到 Sprint 2。

> 注意:Sprint 1 的砍项只影响 **Renderer 运行时完善度 / 校验深度 / 预览体验**,不影响 Sprint 2 的 Designer 物料定义范围。Sprint 2 仍必须注册 RichText、FileUpload 等核心 9 物料的 schema 片段和属性面板;若 Sprint 1 降级实现过这些物料,Sprint 2 负责补齐对应 Renderer 能力后再通过完成标准。

**完成标准**:
- 纯 web 端跑通主线:Owner 看到官方 qa_quality 任务 + baseline → Labeler 领 1 题答 1 题(能看到 text/video/image/markdown 中至少一种 ShowItem 展示 + 评 4 维度 + 写理由 + 调 LLM)→ 提交 → Reviewer 看到 → approve → 导出 JSON
- 屏幕录制 3 分钟无报错
- qa_quality 任务的 schema 落地,后续不再大改;Sprint 2 Designer 输出的目标就是能用拖拽搭出这份 schema

### Sprint 2 — Designer 拖拽搭建(Week 3,~5 天)★ 核心难点 1

> **物料分两批**:Sprint 2 必做核心 **9 个**(PDF 明文物料清单 + 官方数据字段全覆盖),加分 2 个推到 Sprint 6。

**Sprint 2 必拿 — 核心 9 个物料**:
- **ShowItem**(★ 最重要,支持 text / video / image / markdown / json polymorphic 渲染)
- **Input** / **TextArea**(单/多行文本)
- **Radio**(单选)
- **Checkbox+Tags**(多选 / 标签)
- **RichText**(TipTap)
- **JSONEditor**(修正后答案 / 结构化批注,官方两份任务都用到)
- **FileUpload**(图片 / 文件上传,后端走 `apps/api/internal/handler/upload.go` + 本地 fs `./uploads/<task_id>/<uuid>.<ext>` + nginx 静态 / Vite proxy)
- **LLMTrigger**(独立物料,挂载 prompt 模板 + target 字段 + 模式 replace/append;Sprint 1 已写好 mock 后端 `/llm/inline`,这里只接前端 widget)

**Sprint 2 任务**:
- Designer 三栏布局(dnd-kit/core + sortable)
- meta-circular 属性面板(若卡 → 降级硬编码,**Sprint 2 第 3 天没跑通就降级,不死磕**)
- `packages/schema-spec`:ajv 校验 + 版本号 + ShowItem path 解析 + migrate 工具
- 模板版本化(`task_templates.version`)+ 历史浏览
- 联动(Formily x-reactions 原生)+ 校验(必填 / 长度 / 正则 / requiredWhen)
- 单测:schema 序列化往返 + Renderer 快照 + 状态机迁移表 100% 覆盖
- **回归测试**:Sprint 1 的 qa_quality schema 能在 Designer 里拖拽重建,导出 JSON 与手写版字段一致

**完成标准**:Owner 全程拖拽搭出 qa_quality 任务 schema(**单页顺序布局,不含 Tabs**:ShowItem 多媒体展示 + 4 维度 Radio + Tags + Input/TextArea + RichText + JSONEditor + FileUpload + LLMTrigger);保存发布;Labeler 答题与 Sprint 1 手写版表现一致。**Tabs / Group 容器物料延后到 Sprint 6 加分**,本 Sprint 仅验证核心 9 物料和顺序布局重建能力。

### Sprint 3 — AI Agent + 完整审核流 + **baseline 落产品**(Week 4,~5 天)★ 核心难点 2

> **关键调整**(吸纳 review):baseline / golden dataset 落成产品功能,不只是 DB 字段。

**任务清单**:
- `ai_prompt_configs` 表 + **Owner AI 评测配置页面**(★ AI Agent 亮点):
  - Prompt 模板编辑(TipTap 富文本,支持 `{{payload.*}}` `{{payload.expected_dimensions}}` `{{task.baseline_description}}` `{{answer.*}}` 占位变量)
  - 评分维度配置(name + weight + description,可增删改)
  - 题目级维度覆盖:如果 `payload.expected_dimensions` 存在,AI Prompt 渲染时优先把它作为本题重点评测维度;Owner 配置的全局维度作为兜底
  - 通过 / 不确定阈值滑块
  - **baseline_description** 文本框(默认从 task.description 同步,可编辑)
  - **golden 样例编辑器**:Owner 上传 3-5 条已知答案的样例(payload + expected_answer + expected_verdict)
  - **dry-run 按钮**:点 → API 创建 `ai_dry_runs` 记录并入队 → worker 调豆包跑所有 golden 样例 → 前端轮询 `GET /ai-dry-runs/:id` 展示 AI 实际 verdict 与 expected_verdict 的对比表格 + 偏差比例 → Owner 可以调 Prompt / 阈值 / 维度权重再试。LLM 不直接跑在 api 进程里
- outbox publisher goroutine
- ai-worker:Asynq handler + 豆包 client + Function Calling + score 合并 + verdict 决策(写 `submissions.ai_verdict` 字段 + 触发状态转移到 `human_reviewing` 或 `approved`)+ retry/DLQ + 熔断 + mock provider
- Reviewer:批量操作 + 打回带 reason + 看上一轮意见 + AI verdict 显示(字段而非状态)
- Labeler:修订模式(看 reason → 改 → 再提交,revision_no+1,清空 ai_verdict 重走)
- 审核历史时间线(用 `audit_logs` 渲染,展示 AI 介入与人工动作交错)
- asynqmon 部署 + nginx 加密码保护(加分,看时间)

**完成标准**:Labeler→AI→Reviewer→Labeler→Approved 全链路 web 端可见;**Owner 在配置页能 dry-run 看到 AI 对 golden 样例的打分稳定性 + 一致性**;手动 kill ai-worker 再起,任务能恢复。

### Sprint 4 — 多格式导出 + Owner 数据看板(Week 5,~5 天)

**必交任务**:
- 异步导出(Asynq `export` queue)+ **JSON / JSONL / CSV / XLSX 四种**(excelize StreamWriter 流式)
- 字段映射 UI:导出字段选择 + 重命名 + 含审核记录开关
- 导出历史列表 + 签名下载 URL
- Owner Dashboard:进度 / 通过率 / AI vs 人工差异率 / 各维度均分(antd-charts)
- 题目预览(基于 Renderer 干跑)

**加分任务**(进度紧时砍,按下方优先级):
- ★★★ **Markdown 导出器**(对接模型训练,张满宣讲会强调)
- ★★ **JSONL / Excel 题目导入**(JSON 导入已在 Sprint 1 做完)
- ★★ **分发策略 `assigned` + `quota`**(`first_come` 已在 Sprint 1 做完)— 课题"任选其一"

**完成标准(必交)**:JSON / JSONL / CSV / XLSX 四格式导出文件用 jq + excel 打开字段正确;Dashboard 数字与 DB 直查一致;`first_come` 分发可独立验收;Owner 仅用 JSON 题目导入也能完整跑通新建任务流程。

**完成标准(加分,做了的要测过)**:Markdown 导出能被 typora 打开 + 表头一致;JSONL / Excel 导入与 JSON 导入字段对齐;`assigned` + `quota` 各跑一遍可独立验收。

### Sprint 5 — 工程质量 25%(Week 6,~5 天)
- 后端关键路径**单测 100%**(状态机全迁移表 + AI verdict 决策 + **4 个必交导出器**(JSON/JSONL/CSV/XLSX)+ outbox publisher;Markdown 导出做了再补单测)
- 集成测试:testcontainers-go 起 mysql+redis,跑端到端"提交→AI→审核→Labeler 修订→终审→导出"主流程
- 前端 Renderer 关键组件单测(vitest + RTL):ShowItem 嵌套展示 / LLMTrigger / 联动响应;Designer 至少一个集成 test(拖拽 → 保存 → Renderer 渲染 round-trip)
- TS `strict: true`,消除 any,api-client 用 openapi-typescript 从后端 swag 反向生成
- 全站 error boundary + API 错误提示抛光(错误码体系已在 Sprint 1 落地,本 Sprint 做覆盖补齐和文案统一)
- 文档输出:README + ARCHITECTURE.md(架构图 + 关键决策)+ DEPLOY.md(docker-compose 起步) + API 文档(swag 自动生成 OpenAPI + 导出 Postman Collection)
- 出 `docker-compose.prod.yml`(api + worker + web + mysql + redis + asynqmon + caddy 自动 HTTPS),**交付给用户后续自行部署到 VPS**(课题方不提供 server)
- **完成标准**:`pnpm test` + `go test ./...` 全绿;`docker-compose -f deploy/docker-compose.prod.yml up` 在用户笔记本能起;新人 `make dev` 5 分钟跑起来;Postman Collection 能跑通主流程。

### Sprint 6 — Editorial 抛光 + **加分物料补齐** + 选做加分项(Week 7,~5 天)

> **吸纳 review**:这是后台生产工具,核心应该是**信息密度、表格效率、状态清晰**;暗色模式 + 复杂动效 + 移动适配作为可砍的加分项。1280×800 下不强塞三栏 Designer,默认采用"物料侧栏 + 画布 + 属性抽屉"或可折叠右栏;1920×1080 再展开完整三栏。

**必拿(信息密度 + 状态清晰)**:
- Semi 主题 token override 覆盖 Button / Table / Modal / Form / Tag / Sidebar
- Sidebar / Header magazine 风:大标题 Serif + 细线下划 + 段落留白
- 任务 / 题目列表用密集表格而非松散 card grid(信息密度优先)
- Skeleton + 按钮 loading + 顶部 NProgress
- 空 / 错误状态:Serif 大字 + 单色线描插画(自画 SVG 5 张)
- 1280×800:Designer 默认两栏 + 右侧属性抽屉/折叠栏,审核台表格列可配置;1920×1080:Designer 展开完整三栏并显示更多审核/导出列
- 友好错误:状态机非法迁移 422 + 中文提示;LLM 失败给"重试 / 切到人工"两个按钮
- A11y:label + 焦点环
- 状态徽章配色稳定可识(草稿灰 / submitted 蓝 / ai_reviewing 紫 / human_reviewing 橙 / approved 绿 / rejected 红 / revising 黄)— **7 态**,与 §4.2 对齐

**Sprint 2 推迟的 2 个加分物料**(本 Sprint 补齐,按时间顺序砍):
1. **Tabs** 独立物料(可拖拽多 Tab 容器,允许子节点嵌套到 TabPane;Sprint 1 / 2 都没用 Tabs,本 Sprint 补齐后才支持 preference_compare 的并排/分区布局)
2. **Group**(分组容器)

**选做加分项**(从下往上砍):
- 动效:Framer Motion 列表 stagger / Tab 切换 / 抽屉滑入(★ 加分,半天)
- 快捷键:作答页 `J/K` 上下题 + `⌘/Ctrl+Enter` 提交;Reviewer `A/R/B`(★ 加分,半天)
- 暗色模式(Semi 自带,Editorial 暗色 `#1A1814` 背景)(★ 加分,半天)
- 移动端 Labeler 作答页响应式(底部固定操作栏 + 单列布局)(★ 加分,1 天)

**完成标准**:Lighthouse Best Practices ≥ 90;无 console error;视觉 100% 统一在 Editorial 风格里(不出现 Semi 默认蓝);**Designer 11 物料全可用**(9 核心 + Tabs / Group;若加分物料砍,plan 里明确写"砍掉 X 物料,理由 Y")。

### Sprint 7 — 答辩冲刺(Week 8,~5 天)
- 演示脚本(5/10 分钟双版,秒级时间轴):登录 → Owner 建任务 + 配 baseline + 拖拽 Designer → 发布 → Labeler 领题 + 答题(含 LLM 触发)→ AI 预审动画 → Reviewer 打回 + Labeler 修订 → 终审通过 → **4 种必交导出**(JSON/JSONL/CSV/XLSX)+ Dashboard 看板;**Markdown 导出做了的话演示日加 +30s 单独展示**
- 录制视频(Loom 5 分钟主版 + 10 分钟详版,带中文字幕)
- **AI Coding 过程文档(8-15 页 PDF)**:以 git commit 为主轴,提炼"重要决策点 + Claude 怎么参与"。结构建议:
  1. 项目概览 + AI 协同范式说明(1 页)
  2. 6 个关键决策点提炼(各 1-2 页),每个含「决策背景 / Claude 给的备选 / 最终选择 + 理由 / 实际执行命中度」— 候选:Monorepo 拆分、状态机引擎设计、outbox 模式、Designer meta-circular、Editorial 风格、Function Calling JSON 设计
  3. AI Coding 工作流(Plan 模式 / TDD / 代码审查 / 长上下文管理)(2-3 页)
  4. 反思:Claude 用得好 / 不好的场景(1 页)
- API 文档最终校对:基于 Sprint 5 已生成的 OpenAPI / Postman / Swagger UI 做接口示例、错误码、鉴权说明和演示账号补全;不在 Sprint 7 重新设计 API
- 架构 PDF(5-8 页,从本 plan 整理 + 加 mermaid 图)
- **第二官方任务样例 seed**(主线 qa_quality 已在 Sprint 1 完成,这里追加 preference_compare 让 Dashboard 有第二类任务曲线,可砍):
  1. ~~qa_quality 质检标注~~(已在 Sprint 1 作为主线)
  2. **preference_compare 偏好对比标注**(12 条,并排 ShowItem 渲染 prompt / response_a / response_b;Radio 偏好结论 A/B/tie + margin + safety_flag;Tags 判断依据维度;TextArea 判断理由;JSONEditor 结构化批注;FileUpload 佐证截图;LLMTrigger AI 预判)— 加分项,优先替代自造任务
- **答辩前 24h 代码冻结**,只改文案
- 三份备份:用户 VPS + 本地 docker-compose + U 盘离线镜像(`docker save` 镜像 + sql dump + 文件 + nginx 配置)
- 关键路径每步前手动备份 mysqldump,翻车时 10s 还原

---

## 8. 关键技术决策

| 决策 | 选择 | 理由 |
|---|---|---|
| 鉴权 | JWT HS256 + Bearer(access 2h + refresh 14d) | 无状态、答辩好讲 |
| 文件上传 / 导出存储 | 本地 fs + nginx 静态 | 零成本、可离线演示 |
| 富文本 | **TipTap** | TS 优秀、输出 HTML 干净 |
| 表单核心 | **Formily 2** + ajv | Schema 驱动天然契合 |
| 拖拽 | `@dnd-kit/core` + sortable | 现代 / Hook API / a11y |
| 状态管理 | **Zustand** | 极简、TS 友好、无 boilerplate |
| UI 库 | **Semi Design**(主)+ AntD(备) | Semi 字节出品与豆包同源;AntD 兜底 |
| 后端框架 | **Gin** | 文档全 / 中文资料多 / Go 新手友好 |
| ORM | **GORM v2** | Go 新手最熟 / 钩子方便写 audit |
| Migration | golang-migrate | 标准、CI 友好 |
| 队列 | **Asynq** | Go 原生 / dashboard 答辩加分 / 部署轻 |
| Schema 校验 | ajv(前)+ gojsonschema(后) | 跨端一致 |
| Excel | excelize/v2 StreamWriter | 大文件流式、纯 Go |
| 路由 | React Router v6 | 主流 / 嵌套 / Lazy 简单 |
| API 请求 | TanStack Query + axios | 缓存 / 重试 / 状态完美 |
| 日志 | Go zap structured | 答辩友好 |
| 测试 | Go: testify + testcontainers / Front: vitest + RTL | 主流 |
| CI | GitHub Actions | 免费 |
| 部署 | docker-compose + caddy(自动 HTTPS) | 5 分钟搞定 |

---

## 9. Top 5 风险与缓解

| # | 风险 | 概率 / 影响 | 缓解 |
|---|---|---|---|
| 1 | **Go 学习曲线吃掉时间** | high / high | 严格 timebox 3 天基础,边学边写业务;Claude 一次性生成 Go 骨架(handler/service/repo 分层);状态机用 100 行自研 map[from][event]→to 不引入 Temporal;Sprint 0 末若严重卡壳,降级保 60% 弃动效/移动适配 |
| 2 | **Formily / Designer 复杂度爆炸** | mid / high | Sprint 1 用官方 qa_quality 手写 schema 跑通 Renderer,Designer 推到 Sprint 2;Designer 首轮只保证核心 9 物料的顺序布局,联动/Tab/Group 后半段叠加;属性面板若 meta-circular 卡 → 降级硬编码;Sprint 0 周末 2h Formily Playground spike 验证 |
| 3 | **状态机一致性事故**(卡死 / 并发重复) | mid / mid-high | outbox + idempotency_key 双保险;submissions.uk_item 唯一约束;超时清扫 cron 1min 扫一次;Owner/Reviewer "强制重置状态" 救场按钮(写 audit_log);状态机 100% 单测 |
| 4 | **豆包不稳定 / FC 输出不合格** | mid / mid | 强制 tool_choice + gojsonschema 校验 + retry 5 次;失败 → 写 `ai_verdict='uncertain'` + 转 `human_reviewing` 走人工兜底;限流 + 熔断;**LLM_PROVIDER=mock** 演示日断网兜底;前 1 天充值 + 100 次回放测试 |
| 5 | **答辩 demo 现场翻车** | mid / critical | 三份环境(线上 VPS + 本地 docker + U 盘镜像);前 1 天完整 dry run + 全程视频备份;24h 代码冻结;关键路径前 mysqldump 备份;**至少 1 个完整生命周期 seed 任务**(官方 qa_quality,Sprint 1 必拿)演示不依赖现场操作,Sprint 7 有余量再补 preference_compare;演讲稿秒级时间轴 |

---

## 10. 关键文件(实现时主要落点)

执行时主要修改的文件:

- `~/Desktop/LabelHub/apps/api/internal/statemachine/machine.go` — 状态机引擎与全迁移表(§4 落点)
- `~/Desktop/LabelHub/apps/api/internal/service/submission_service.go` — `Transition()` 事务 + outbox 写入
- `~/Desktop/LabelHub/apps/api/internal/migration/*.sql` — **17 张表** migration(含 `user_roles` 多对多 + `tasks.baseline_description` + `uploaded_files` + `golden_samples` + `ai_dry_runs` + `outbox_events`)
- `~/Desktop/LabelHub/apps/api/internal/handler/upload.go` — FileUpload 上传限制、权限校验、temp/attached 状态转换入口
- `~/Desktop/LabelHub/apps/ai-worker/internal/consumer/review_handler.go` — AI 预审主流程(FC + 幂等 + 重试 + 熔断)
- `~/Desktop/LabelHub/apps/ai-worker/internal/prompt/renderer.go` — Prompt 模板渲染,**注入 task.baseline_description + task.description**
- `~/Desktop/LabelHub/apps/ai-worker/internal/llm/doubao_client.go` — 豆包 OpenAI 兼容 client(含 mock provider)
- `~/Desktop/LabelHub/packages/schema-spec/src/index.ts` — JSON Schema 规范、ajv 校验、版本 migrate
- `~/Desktop/LabelHub/apps/web/src/designer/Designer.tsx` — 三栏布局 + 物料注册 + meta-circular 属性面板
- `~/Desktop/LabelHub/apps/web/src/designer/materials/registry.ts` — 物料注册表(Sprint 2 注册 9 核心 + Sprint 6 注册 2 加分,共 11 个目标)
- `~/Desktop/LabelHub/apps/web/src/renderer/SchemaRenderer.tsx` — Renderer + ShowItem/LLMTrigger 自定义组件
- `~/Desktop/LabelHub/apps/web/src/renderer/widgets/ShowItem.tsx` — **支持 text / video / image / markdown / JSON polymorphic 展示**
- `~/Desktop/LabelHub/apps/web/src/styles/tokens.css` — **Editorial 风格 design token**(色 / 字 / 间距 / 圆角)
- `~/Desktop/LabelHub/apps/api/internal/exporter/{json,jsonl,csv,xlsx}.go` — **4 种格式必交**异步导出;`markdown.go` 是加分项,做了再加
- `~/Desktop/LabelHub/deploy/docker-compose.yml` — 本地开发环境
- `~/Desktop/LabelHub/deploy/docker-compose.prod.yml` — **生产部署模板**(用户后续自行 VPS 上线用)

---

## 11. 验证方案

每个 Sprint 末跑下面对应的验收:

### 端到端主线(每 Sprint 末必绿)

> **Sprint 1-2 走"无 AI 预审闭环"**(task.ai_review_enabled=false,/llm/inline 用 mock);**Sprint 3 起走"含 AI 预审完整闭环"**(ai-worker 真实消费 + 豆包调用)。主线 seed 始终使用官方 `qa_quality`。

#### Sprint 1-2 回归脚本(无 AI 预审版本)
```bash
# 1. 起环境
cd ~/Desktop/LabelHub && docker-compose -f deploy/docker-compose.yml up -d

# 2. 跑 seed(创建用户 + 官方 qa_quality 任务 ai_review_enabled=false)
docker exec labelhub-api ./tools/seed

# 3. 浏览器手测(每 Sprint 5 分钟回归)
open http://localhost:5173
# - owner1/pass 登录 → 看到官方 qa_quality 任务 → (Sprint 2 起:进 Designer 拖拽重建 schema 并保存 v2)
# - labeler1/pass 登录 → 广场领任务 → 答 3 题(ShowItem text/video/image/markdown/JSON 展示 + Radio + Tags + JSONEditor + RichText + mock LLMTrigger)→ 提交
# - 提交后 submission 直接进 human_reviewing(跳过 ai_reviewing)
# - reviewer1/pass 登录 → 队列 → 1 个 approve / 1 个 reject / 1 个 revise(带 reason)
# - labeler1 看到 revise 打回 → 修改 → 再提交(revision_no=2)
# - reviewer1 终审 approved
# - owner1 → 导出 JSON/JSONL/CSV/XLSX 各一份(Sprint 1 只支持 JSON 同步导出,Sprint 4 起支持 4 种异步),jq + excel 校验
```

#### Sprint 3+ 回归脚本(含 AI 预审版本)
```bash
# 1. 同上起环境

# 2. seed 改为 ai_review_enabled=true + 配 AI Prompt + baseline + 3 条 golden_samples
docker exec labelhub-api ./tools/seed --with-ai

# 3. 浏览器手测
# - owner1 → AI 评测配置页 → 试运行 dry-run-with-golden → 看到 expected vs actual 对比
# - labeler1 答题提交 → asynqmon (http://localhost:8081) 看到任务进队
# - 等 5-10s,ai-worker 写完 ai_verdict + 触发状态迁移到 human_reviewing
# - reviewer1 看 AI verdict + dimensions 评分 + 决策 approve/reject/revise
# - 完整 Labeler→AI→人审→修订→终审→导出 全链路录屏
```

### 自动化测试(Sprint 5 起必须全绿)
```bash
# Go 后端
cd apps/api && go test ./... -cover
cd apps/ai-worker && go test ./... -cover
# 目标:核心包(statemachine, service, exporter)≥ 60%

# 前端
cd apps/web && pnpm test
# 目标:Renderer + Designer 集成 test 通过

# 集成测试(用 testcontainers 起 mysql+redis)
cd apps/api && go test -tags=integration ./test/...
```

### 状态机正确性(Sprint 2 起必须 100% 覆盖)
```bash
cd apps/api && go test ./internal/statemachine/... -run TestAllTransitions -v
# 覆盖 §4.3 完整迁移表的每一行 + 所有非法迁移返回 error
```

### LLM 调用兜底(Sprint 3 起)
```bash
# 正常路径
LLM_PROVIDER=doubao docker-compose restart ai-worker
# 跑 3 题,观察 asynqmon retry count = 0

# 兜底路径(模拟 LLM 挂)
LLM_PROVIDER=mock docker-compose restart ai-worker
# 跑 3 题,观察 verdict 仍能产生 + audit_log 标记 mock
```

### 答辩前最终回归(Sprint 7)
- 完整端到端脚本走两遍(本地 + 线上 VPS)
- Lighthouse Best Practices ≥ 90
- 录屏 5 分钟无 console error / 无网络 4xx5xx
- mysqldump 备份就位

---

## 12. Day 1 可立即执行的动作

进入实现阶段后,第一天做这些事:

1. **先做 Git 安全隔离,再初始化仓库**(防 PDF / key / 上传文件误入库):
   ```bash
   mkdir -p ~/Desktop/LabelHub ~/Desktop/LabelHub_materials
   mv ~/Desktop/LabelHub/*.pdf ~/Desktop/LabelHub_materials/ 2>/dev/null || true
   cd ~/Desktop/LabelHub
   printf "*.pdf\n.env\n.env.*\n.DS_Store\n.~*\nuploads/\n" > .gitignore
   git init
   ```
2. 建 monorepo 顶层结构(`apps/`、`packages/`、`tools/`、`deploy/`、`docs/`)
3. `pnpm init -w` + 写 `pnpm-workspace.yaml`(列入 `apps/*`、`packages/*`)
4. `go work init && go work use ./apps/api ./apps/ai-worker`(各自 `go mod init`)
5. 写 `deploy/docker-compose.yml`:mysql:8 + redis:7 + adminer:latest + asynqmon
6. apps/api 写 `GET /health` 返回 `{"ok":true}`,跑通 `go run cmd/server/main.go`
7. apps/ai-worker 写一个空 Asynq handler `noop`,跑通 `go run cmd/worker/main.go`
8. apps/web `pnpm create vite labelhub-web --template react-ts`,引 Semi + react-router,做空白三角色路由
9. 提交 Sprint 0 第一个 commit:`chore: initial monorepo scaffolding`
10. 开 GitHub repo + push,接 GitHub Actions(lint + test + build)

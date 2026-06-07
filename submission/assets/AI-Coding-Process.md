# LabelHub AI Coding Process

提交人: Zhang Youchen  
项目: LabelHub 数据标注平台  
日期: 2026-06-05

## 1. 目标与验收标准

LabelHub 的目标不是做一个静态管理后台,而是交付一条可被评委独立跑通的数据标注生产链路:

- Owner 能完成建任务、搭模板、发布、查看统计、导出结果。
- Labeler 能完成领任务、作答、提交、看到打回、修改重提。
- AI Agent 能在提交后自动预审,并把 verdict、score、维度评分、prompt version、token 与延迟信息留痕。
- Reviewer 能基于 AI 预审结果做人工初审、复核、通过或打回。
- 导出结果能覆盖 JSON、JSONL、CSV、XLSX、Markdown,供下游继续消费。

AI Coding 的使用原则是:让模型帮助快速盘点实现面、生成测试与发现遗漏,但关键状态机、权限、幂等、导出安全等决策由工程约束反推,不让模型凭空扩展需求。

<div style="page-break-after: always;"></div>

## 2. 从原型到可运行主链路

早期实现先收敛最小主链路:用户角色、任务、题目、模板、提交、审核、导出。AI 辅助在这一阶段主要用于拆解接口边界与生成初版测试用例。

关键取舍:

- 用 Go + Gin + GORM 承担 API 与事务边界,避免把核心状态机散落在前端。
- 前端保留 React + TypeScript SPA,按 Owner / Labeler / Reviewer 角色组织工作台。
- 模板系统先做自有 Schema Renderer 与 Designer,覆盖 ShowItem、Radio、Tags、Input、TextArea、RichText、JSONEditor、FileUpload、LLMTrigger,再扩展 Group/Tabs。
- 所有提交与审核动作都落库,不依赖浏览器临时状态。

AI 参与的主要价值是把散乱需求转成可执行检查表:哪些状态需要锁、哪些角色可见、哪些路径需要测试,以及哪些演示步骤会被评委直接碰到。

<div style="page-break-after: always;"></div>

## 3. 状态机与事务纪律

LabelHub 把任务与提交状态集中在 `apps/api/internal/statemachine`,所有业务服务只通过合法 event 推进状态。

提交主路径:

- `draft` -> `submitted`
- `submitted` -> `ai_reviewing` 或 `human_reviewing`
- `ai_reviewing` -> `human_reviewing` / `manual_review` / `revising`
- `manual_review` -> `human_reviewing` / `rejected` / `revising`
- `human_reviewing` -> `approved` / `rejected` / `revising`
- `revising` -> `submitted`

AI Coding 在这部分主要用于补齐非法转移测试矩阵。后续人工 review 又发现旧文档仍保留 `ai_auto_approved`;最终实现改为 AI 不自动入库,无论 AI pass 还是 uncertain 都需要人工路径完成最终决策。

工程约束:

- 审核动作事务内锁 submission 与 task item。
- 并发审核使用 `RowsAffected` 检查防双写。
- 打回后 labeler 修改会创建新 revision,旧 AI 与人工结果保留可追溯。

<div style="page-break-after: always;"></div>

## 4. AI Agent 预审设计

AI 预审不是同步 HTTP 调用,而是 durable outbox + Asynq worker。

API 提交流程:

- 保存 submission revision。
- 写入 `ai_reviews` pending 记录。
- 同事务写 `outbox_events(ai:review)`。
- outbox publisher 把事件投递到 Redis / Asynq。

Worker 消费流程:

- 读取当前 submission、revision、task baseline、active AI prompt。
- 调用 mock 或 OpenAI-compatible / Doubao provider。
- 校验 verdict、overall_score、dimensions schema。
- 写回 `ai_reviews` 结果和 audit log。
- 按 verdict 推进 submission 状态。

默认本地环境使用 deterministic mock,便于评委无 API key 跑通;真实环境只需切换 `LLM_PROVIDER`、`LLM_BASE_URL`、`LLM_API_KEY`、`LLM_MODEL`。

<div style="page-break-after: always;"></div>

## 5. Prompt、Golden Sample 与 Dry-run

Owner 侧 AI Prompt 不是硬编码规则,而是任务级配置:

- Prompt template
- 维度列表
- pass threshold
- uncertain min
- model
- active prompt id

Golden Sample 用来在正式标注前试跑 prompt。每条样例包含 payload、expected answer、expected verdict,worker 写回 actual verdict 与 matched result。

AI Coding 在这部分帮助快速生成了接口边界与前端轮询状态,但最终实现保留了几个明确限制:

- dry-run 有 task scoped quota / circuit guard。
- prompt version 固化在 AI review 与 dry-run 记录里。
- Reviewer 只能查看规则,不能直接修改 Owner 的 prompt。
- Seed 官方任务现在自带 active prompt 与 golden samples,保证 5 分钟评委路径无需额外手工配置。

<div style="page-break-after: always;"></div>

## 6. 多格式导出与下游消费

导出由 `pkg/exporter` 作为共享模块实现,API 与 worker 复用同一套编码逻辑。

支持格式:

- JSON
- JSONL
- CSV
- XLSX
- Markdown

关键安全与稳定性处理:

- 导出是异步 job,状态从 queued -> running -> succeeded / failed。
- worker 使用临时文件写入,完成后原子 rename。
- 下载走 HMAC 签名 URL 与过期时间。
- CSV 对 `= + - @` 等开头单元格加前导单引号,防公式注入。
- API 启动时要求 `EXPORT_DIR` 是绝对路径,避免 api/worker CWD 不同导致下载失败。

AI 辅助主要用于 review exporter 边界:错误是否可重试、CSV 注入、签名下载、path traversal、前端 stale polling。

<div style="page-break-after: always;"></div>

## 7. 测试与工程质量

测试层级覆盖三类风险:

- 纯逻辑单测:状态机、review service、outbox publisher、exporter encoder、AI verdict mapping。
- Handler 单测:权限、参数校验、错误码、owner/labeler/reviewer 主接口。
- Testcontainers 集成测试:真实 MySQL + Redis 覆盖 submit -> outbox -> AI -> revise -> resubmit -> approve -> export。

前端开启 TypeScript strict mode,并用 Vitest 覆盖:

- Schema Renderer 的 ShowItem、Tabs、LLMTrigger。
- Designer 保存 schema 后再由 Renderer 解析的 round-trip。
- Labeler 自动保存、快捷提交、打回修改。
- Owner AI Prompt、Golden Sample、导出、Stats Board。
- Reviewer 队列与 AI verdict 展示。

CI 跑 Go test / vet、web test / lint / build,并单独跑 integration job。

<div style="page-break-after: always;"></div>

## 8. 产品体验与最后收敛

最后阶段重点不是继续加功能,而是清理评委会直接遇到的阻塞:

- README 账号密码必须和 seed 一致。
- 官方任务 seed 后必须有 active AI Prompt,否则 labeler 提交不会触发 AI review。
- 架构图必须反映真实 worker 行为,不能保留已删除的 auto approve。
- 交付包 manifest 必须指向真实存在的文件,不能把待补素材伪装成交付物。
- 1280 和 1920 视口下 Designer、Owner、Reviewer 关键路径不能出现遮挡或布局坍塌。

AI Coding 的最终价值体现在这些收敛环节:用 review checklist 反复对照实现、文档、demo 脚本、seed 数据,把“代码能跑”和“评委能独立验收”之间的差距补齐。


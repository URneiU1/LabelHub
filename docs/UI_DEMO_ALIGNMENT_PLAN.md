# 真前端视觉对齐 demo —— 实施方案

> 分支：`feat/ui-reskin-s7-and-followups`。参考稿：`~/labelhub-ui-demo`(已打磨的 mock demo,4 个页面)。
> 线上对照：http://43.155.210.70 (腾讯云)。

**目标**：把真前端 4 个页面的视觉调性对齐 demo —— 任务管理、模板搭建(Designer)、AI 预审、人工审核。**只改视觉/排版,不改功能、不动 API、不挪路由**(人工审核保留在 reviewer,不回迁 owner)。其余页面(数据集/数据看板/数据导出)本轮不动。

**核心事实**:真前端早已把 demo 的 `lh-` 系统拷进 `src/styles/lh/`,4 个页面都用同一套 class。差异来自真前端自己的二次改造(卡片包裹、彩色 stat 描边、大写英文 stat 标签、空心圆导航图标),以及 token 微调(更柔的蓝 `#2f6bff`、更大圆角、蓝灰阴影)。

## Token 策略(关键决定)

**保留真前端现有 token(`#2f6bff` 柔蓝、圆角 6/10/14、蓝灰阴影),不回退成 demo 的 Arco 原始蓝。** 理由:真前端这套 token 更精致、且 5 个页面统一;"demo 观感"主要来自**排版构图 + 图标 + 卡片处理**,不是那抹蓝。若你坚持要 demo 的精确配色,改 `lh/tokens.css` 3 个变量即可,一行回退。

## 约束 / 验证

- 不破坏现有测试:每阶段后 `pnpm test`(reviewer/owner/template 都有 .test.tsx)+ `pnpm build` + `pnpm lint` 全绿。
- 视觉验证:改完本地 `pnpm dev` 起前端(proxy 到 :8080 或线上后端),逐页截图对照 demo。**需要你这边能跑起后端/或我截本地前端**——执行前确认验证方式。
- `git add <具体文件>`,不用 `-A`;commit 走仓库现有 conventional 风格;不加 AI 署名。
- 大组件(Designer/Dashboard/Queue)动手前**先完整读该组件**再改,避免盲改。

---

## Phase 0 — 共享:导航图标(最高性价比,全局统一)

**File:** `src/shared/layout/AppLayout.tsx`(改 nav 渲染)+ 新建 `src/shared/components/Icon.tsx`(移植 demo 的内联 SVG 图标集)+ `src/styles/lh/globals.css`(`.lh-side-item__icon` 去掉占位方块样式)。

- [ ] 移植 demo 的 `Icon.tsx`(SVG 图标集:list/layout/database/sparkle/userCheck/barChart/download + 动作图标 up/down/copy/close/box)到 `src/shared/components/Icon.tsx`。
- [ ] AppLayout 各 nav item 加 `icon` 字段,把 `<span className="lh-side-item__icon" />` 换成 `<Icon name={item.icon} .../>`;映射:任务管理→list、模板搭建→layout、数据集→database、AI 预审→sparkle、审核结果→userCheck、数据看板→barChart、数据导出→download。
- [ ] `lh/globals.css` 的 `.lh-side-item__icon` 去掉 border/background 占位,改为只控尺寸 + opacity(参考 demo 改法)。
- [ ] `pnpm build && pnpm test && pnpm lint` 全绿;本地起前端,确认 5 个 owner 页 + reviewer 导航图标正常、选中态颜色跟随。
- [ ] commit:`feat(web): replace placeholder nav bullets with real icons`

## Phase 1 — 任务管理(`src/modules/owner/TaskManagePanel.tsx` 236 行 + `lh/tasks.css`)

对照 demo Tasks 图,差异 = ①stat 卡彩色描边 → 平卡+柔阴影+选中蓝高亮 ②整页大白卡包裹 → 标题置于灰底、stat/表格分离 ③补搜索+筛选行(纯前端过滤,mock 行为即可,不接新 API)。

- [ ] 完整读 TaskManagePanel.tsx,定位 stat 卡 / 容器 / 表格的 JSX。
- [ ] stat 卡:移除彩色描边,采用 demo 的 `.lh-stat`(平卡 + `--lh-shadow-sm` + 选中 `.lh-stat--active` 蓝高亮 + Sora 数字 tabular-nums)。
- [ ] 容器:去掉最外层包裹大卡,页头用 demo 的 `.lh-page-head`(标题直接在灰底)。
- [ ] 加 `.tasks-filters` 行(搜索框 + 状态/策略下拉),做纯前端过滤。
- [ ] `pnpm test`(TaskManagePanel.test.tsx)+ build/lint 绿;截图对照 demo 图。
- [ ] commit:`feat(web): align task-manage page visuals to demo`

## Phase 2 — Designer(`src/modules/template/Designer.tsx` 2313 行,轻触)

真 Designer 是完整功能版,**不重写**。只补 demo 的小视觉点:

- [ ] 完整读 Designer.tsx 相关 JSX(canvas 字段动作区、palette、header)。
- [ ] canvas 字段动作按钮:文字「复制/删除/↑/↓」→ `<Icon>`(copy/close/up/down),与 demo 一致。
- [ ] ShowItem 字段加 demo 的「ShowItem · 不参与提交」角标(`.canvas-field__corner`)若缺。
- [ ] 卡片层次:确认 `.canvas-field` 有柔阴影 + hover 抬升(参考 demo designer.css 改法)。
- [ ] `pnpm test`(Designer.integration.test.tsx)+ build/lint 绿;截图对照。
- [ ] commit:`feat(web): polish designer canvas visuals to demo`

## Phase 3 — AI 预审(`src/modules/owner/Dashboard.tsx` ai section,1758 行)

真 AI 预审是配置页(非 demo 的队列页),对齐 = 采用 demo 的卡片/排版调性:

- [ ] 完整读 Dashboard.tsx 的 ai section。
- [ ] stat 卡(TASK/AI REVIEW/PROMPTS/EVAL/HISTORY)采用 demo `.ai-stat-card` / `.lh-stat` 调性(平卡+阴影+Sora 数字);保留其信息架构。
- [ ] verdict / prompt / 日志区若有,套 demo 的 `.ai-section` / `.ai-verdict` / `.ai-prompt` 卡片样式。
- [ ] build/test/lint 绿;截图对照 demo AiReview。
- [ ] commit:`feat(web): align AI-review page visuals to demo`

## Phase 4 — 人工审核(`src/modules/reviewer/Queue.tsx` 1818 行,**留在 reviewer**)

已用 demo 同款 `hr-`/`ai-` class,对齐 = 应用 demo 的细化(图标、卡片阴影、决策卡 hover、半选态 checkbox):

- [ ] 完整读 Queue.tsx。
- [ ] hr-item / hr-compare__col / ai-section 套 demo 的柔阴影 + hover;hr-decision 决策卡 hover 抬升。
- [ ] 批量「全选」checkbox 若无半选态,补 demo 的 indeterminate 逻辑。
- [ ] 头部/图标用 Phase 0 的 Icon。
- [ ] `pnpm test`(Queue.test.tsx / ArbitrationQueue.test.tsx)+ build/lint 绿;reviewer 角色登录截图对照 demo HumanReview。
- [ ] commit:`feat(web): align human-review page visuals to demo`

---

## 执行顺序与回退

P0 → P1 → P4 →(P2、P3 视效果)。P0 是低风险全局收益,先做。每 Phase 独立 commit,出问题单 commit 回退。token 策略若要改成 demo 精确配色,单独一个 commit 改 `lh/tokens.css`。

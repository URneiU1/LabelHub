# HANDOFF — Designer 重做 + S8 任务条例

> 交接给下一个开发者。本文 = ①已完成的 UI 换肤 + 数据看板丰富 ②已完成的 Designer 对齐 ③已完成的 S8 第一批任务条例 ④后续可选的 S8 第二批。
> 部署/环境/历史背景先读 `docs/HANDOFF.md`；本文聚焦接下来要做的两件事。

最后部署：上线在 **http://43.155.210.70**（`api`+`worker`+`web` 均最新）。部署方式 = rsync 工作树到 `ubuntu@43.155.210.70:/home/ubuntu/labelhub/`（`ssh -i ~/Downloads/labelhub.pem`），再 `docker compose --env-file deploy/.env -f deploy/docker-compose.prod.yml up -d --build <web|api worker>`。详见 `docs/HANDOFF.md`。

---

## 0. 本会话已完成（已 commit + 部署 + 浏览器冒烟验证）

UI 从 Arco/Editorial 双皮统一为 **PetaV2 风格**，并把 Owner 导航改成路由式页面，最后丰富了数据看板。提交链（feature 分支 `feat/ui-reskin-s7-and-followups`）：

| commit | 内容 |
|---|---|
| `fd3e986` | 全站换肤 PetaV2（柔和蓝 + Poppins/Montserrat + 圆角 + 柔和阴影 + 冷色盘），统一两套冲突 token |
| `73f2066` | 主色调深到 `#2f6bff`，新增 `--lh-primary-rgb` 让辉光跟随主色 |
| `2d9106d` | **Owner 侧栏 7 个分节改为独立 URL 页**（`/owner/:section`，无下拉）；AI 预审分节加页内标签页（`/owner/ai/<sub>`，CSS `data-sub` 过滤，默认 `all` 全显以免测试回归） |
| `45b73d0` | 「数据导出」拆 `导出配置/导出历史` 子页；过滤机制泛化为 `.lh-sub-views`；清理手风琴遗留孤儿 id |
| `bd32ba2` | 「数据导出」去掉「全部」标签、默认落 `导出配置`（`SECTION_DEFAULT_SUB` + `/owner/export` 重定向） |
| `8cc86b2` `067464d` | **数据看板丰富**：混淆矩阵 / AI 分数分布 / 完成趋势 / 维度雷达 / KPI 环（详见 `docs/CHANGELOG.md` 的 `owner-dashboard-enrich`） |

关键设计（接手前必看）：
- **导航 URL↔store 同步在 `AppLayout.tsx`**，`Dashboard.tsx` 保持 router-free（只读 `ownerSection.ts` 的 store），这样 `Dashboard.test.tsx` 37 处 `render(<OwnerDashboard/>)` 零改动。改 Designer/Dashboard 时别破坏这个边界。
- 子页过滤靠 `.lh-sub-views[data-sub=x] #id { display:none }`（`globals.css`），**默认 `all` 不隐藏任何块**——既是「全部」概览页，也保证 jsdom 测试在默认态能断言到全部内容。
- 分节子页定义集中在 `apps/web/src/shared/state/ownerSection.ts` 的 `OWNER_SUB_NAV`（加一项即多一个子页）；无概览的分节登记在 `SECTION_DEFAULT_SUB`。

---

## 1. 已完成 · Designer 对齐设计稿 + 横向子导航

目标：让 **模板搭建器（Designer）** 长得像组织方设计稿（用户提供的截图），并在画布顶部加 **横向子导航（Tab 栏）**。

文件：
- `apps/web/src/modules/template/Designer.tsx`（1997 行，主组件 + `CanvasField` + `NestedCanvasPreview` + `PropertyPanel`）
- `apps/web/src/modules/template/Designer.css`（仅 33 行，三栏 grid 响应式断点）
- `apps/web/src/renderer/SchemaRenderer.tsx`（Tabs 已渲染真实 tablist/tabpanel，可参考其交互）

### 现状（已具备，别重造）
- **三栏布局** `template-designer-grid`：左 `组件物料` palette / 中 canvas / 右属性。
- **画布字段卡** `CanvasField`：已有上移/下移/复制/删除控件、选中高亮、拖拽排序、`Group`/`Tabs` 嵌套预览（`NestedCanvasPreview`）。
- **右侧属性面板 `PropertyPanel`**：**已经有 `基础 / 校验 / 联动` 三个横向标签页**（`PropertyTab = 'basic'|'validation'|'logic'`，约 Designer.tsx:674-725）——这正好对齐设计稿右栏，**保留，仅需按新视觉重新上色**。
- 顶部栏：返回链接 + "Template Designer" 标题 + ID/Version/LATEST 徽章 + `重置修改`/`保存并发布新版`（或 Fork）。

### 已完成（2026-06-01，本地待随 S8 一起发布）
1. **画布顶部横向 Tab 栏（用户要的「横向子导航」）**：设计稿画布顶部有 `基础信息 | 标注 | + 新 Tab`——把表单按**顶层 Tab** 分组。当前 `Tabs` 只是一种 widget，没有画布级 Tab 栏。可参考 `globals.css` 里已有的 `.lh-subtabs/.lh-subtab` 视觉（AppLayout 的分节标签条同款），保持全站一致。
2. **顶部栏重做**：面包屑（`任务负责人后台 / 模板搭建 / <任务名> · v3`）、右上 `预览` / `导出 Schema JSON` / `保存并发布版本 r13`（实心蓝）+ 用户头像；标题改 `模板搭建器（Designer）`+ 副标题；显示 `当前版本 r12` 与 `绑定任务 T-2041`。（`导出 Schema JSON`/`预览` 是新按钮。）
3. **左侧物料改图标卡 + 分组**：设计稿分 `物料`（单行输入 Aa / 多行文本 / 单选 / 多选 / 标签选择 / 富文本 / 文件图片 / JSON 编辑器 / LLM 触发组件 / 展示项 ShowItem）和 `布局`（分组容器 / 多 Tab 布局）两组，每项是带图标的卡片。当前是 `{widget}` 文本码按钮、无分组无图标。
4. **字段卡视觉精修**：对齐设计稿——字段名 + `字段名: x · 类型`、右上控件、选项 chips、LLM 触发组件用紫色虚线卡 + `生成参考`/`采纳→`、ShowItem 展示项样式。
5. 整体留白/圆角/阴影按 PetaV2（用 `--lh-*` / `--color-*` token，别硬编码颜色）。

注意：Designer 用的是 **Editorial 层 token（`--color-*` / `--radius-*` / `--font-*`）**，这套已被本会话换成 PetaV2 冷色盘，所以底色已对；重点是结构（Tab 栏、物料分组）与卡片细节。

---

## 2. 已完成 · S8 第一批任务条例

给「任务」在 状态 / 分发策略 / 配额 之外，加质量与管控条例。**用户已明确分批与字段，照做即可。**

### 现状（重要：模型里已有若干字段，UI 未用上）
`apps/api/internal/model/models.go` 的 `Task` 已有：`Tags`、`RewardConfig`、`BaselineDescription`、`RichDescription`、`Deadline`、`HumanReviewEnabled`、`Distribution`(默认 `first_come`)、`QuotaPerUser`、`PublishedAt`。所以「截止时间/标签/人工审核开关/富文本说明」**只缺前端表单**，不必新增列。

相关代码：
- 分发/领题逻辑：`apps/api/internal/service/submission/claim.go`（+ `claim_distribution_test.go`）——overlap / lease / sampling 都在这里挂钩。
- 任务 CRUD handler：`apps/api/internal/handler/task.go`、`task_manage.go`。
- 状态机：`apps/api/internal/statemachine/task.go`（终态 `ended`）——**发布冻结要在这里加约束**。
- 迁移：`apps/api/internal/migration/`（golang-migrate，烤进镜像，api 启动时自动跑）。本批新增 `009_task_policies` 与 `010_overlap_arbitration`。
- 前端任务创建/编辑：`apps/web/src/modules/owner/TaskManagePanel.tsx`（抽屉式表单）。

### S8 第一批（必须做）

> 进度（2026-06-01，本地）:第一批全部完成，尚未部署。`009_task_policies` 增加任务策略列；`010_overlap_arbitration` 放宽 submission 唯一键并增加仲裁状态。

1. **多人重复标注 + 共识仲裁**（核心展示能力，对标 Label Studio / Labelbox）
   - `overlap_count`：默认 `1`；高质量任务建议 `2` 或 `3`。
   - `overlap_coverage_pct`：只让**部分**题进入多人标注，控制成本。
   - 不一致时提交进入 `needs_arbitration`（新增提交/题目状态，路由给资深审核）。
2. **租约超时回收**（first_come 已有，必须补，否则领了题离开会长期占坑）
   - `lease_timeout_minutes`：默认建议 `30`。
   - 超时后题目重新入池；旧提交应**拒绝或保存为草稿**。
3. **审核抽检比例**
   - `review_sampling_pct`：默认 `100`（保持现有行为），可配 `10%`/`30%`。
   - 未命中抽检且无风险的题**直接通过**。（按概率抽检，不是精确配额。）
4. **每日提交上限**
   - `daily_submission_limit_per_labeler`：`0` 表示不限。先做每日上限即可，**不急做复杂限速**。
5. **发布后冻结**
   - 发布任务后锁定 `schema`、`distribution`、`overlap`、`review_sampling`。
   - **不能只是 UI 字段——后端状态机也必须拒绝修改**（在 `statemachine/task.go`）。需要改时复制任务建新版本。

### 第一批实现边界
- overlap 采用**顺序租约**，复用单一 `task_items.claimed_by`：上一位提交后题目释放回池，下一位再领；未另建并发占题表。
- 覆盖率按 `item_id % 100` 确定性分桶；同题答案 canonical JSON 一致才派发审核，不一致转 `needs_arbitration`。
- 仲裁队列用 `GET /reviewer/submissions?status=needs_arbitration`；资深审核一次终审，选中 submission 落终态，其余冲突 submission 收口为 rejected。
- 抽检用共享 `pkg/reviewsampling` 确定性分桶，重试不会漂移。无 AI 时未抽中直接批准；有 AI 时只有 `pass` 且未抽中直通，`reject/uncertain` 始终进入人工审核。
- Owner 表单已拆成「分发设置 / 质量设置 / 发布设置」，并接入人工审核开关。`humanReviewEnabled=false` 的持久化已修复。

### S8 第二批（待办）
- **准入门槛**：基于现有 golden samples，加 `min_accuracy_pct` + 资格测试。**前置依赖**：先把标注员的失败/重试/暂停流程做好。
- **优先级**：放在**数据批次 / 单题**层级（`priority`），不要只做任务级。
- **动态抽检**：低共识 / AI 低置信 / 新标注员 → 提高抽检率；稳定标注员 → 降低。

### 任务创建页建议收敛成三个区块（用户指定）
| 区块 | 字段 |
|---|---|
| 分发策略 | `first_come`、租约时长 `lease_timeout_minutes`、每日上限 `daily_submission_limit_per_labeler` |
| 质量控制 | 重复标注人数 `overlap_count`、覆盖比例 `overlap_coverage_pct`、审核抽检比例 `review_sampling_pct` |
| 发布策略 | 草稿 / 发布、发布后冻结提示 |

---

## 3. 工程约定（接手必读）
- **测试是验收门槛**：改完跑 `go test ./internal/...`（handler 用有序 sqlmock，新增查询要按顺序补 `mock.ExpectQuery`，正则注意 gorm 给字段加反引号 → 用 `.字段. ` 而非 `字段`）；前端 `pnpm -F web build && pnpm -F web lint && pnpm -F web test`。
- 纯逻辑（分桶/聚合/共识判定）抽成纯函数单独 TDD（参考 `stats.go` 的 `buildConfusion`/`bucketScores`）。
- 新列必须配成对的 `NNN_*.up.sql` + `.down.sql`；api 启动自动迁移，所以 `up -d --build api` 即生效。
- 提交：约定式前缀、无 AI 署名；每次改动在 `docs/CHANGELOG.md` 的「最近完成」追加一条。
- 分发策略/配额/状态在 UI 已有（`TaskManagePanel`），新增条例并入同一抽屉的三区块。

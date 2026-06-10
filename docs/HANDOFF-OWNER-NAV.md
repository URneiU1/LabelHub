# HANDOFF — Owner 左侧导航重排为「三组七栏」

> ✅ **已完成(2026-05-31,路线 A)**:用户拍板走路线 A(六栏 / AI 合一),已在 `feat/ui-reskin-s7-and-followups` 上实现。Dashboard 左栏现为三组 `lh-side-section`(数据生产 / 审核与质检 / 数据交付),6 个可切换分节(模板搭建·数据集 / AI 预审·审核结果 / 数据看板·数据导出)+ 左栏常驻任务列表(=任务管理)。AI 预审 = 配置+黄金评测合一;审核结果 = 只读聚合(人工审核动作仍归 Reviewer)。37 个 Dashboard 测试零改动全过,详见 `docs/CHANGELOG.md` 的 `owner-left-nav-three-groups` 条目。
>
> ✅ **后续(同日,采纳本文第 6 节决策 #4)**:用户要求把三组分节**提升到全局「工作区」侧栏**(`AppLayout`,= demo 的真实结构),已实现。新增共享 store `shared/state/ownerSection.ts` 桥接 shell↔页面;AppLayout owner 侧栏渲染 `OWNER_NAV_GROUPS`(7 项含任务管理),Dashboard 改单列、`'tasks'` 节挂 `TaskManagePanel`、非 tasks 节加「当前任务」chip 切换器保留选任务入口。158/158 测试 + lint + build 全过,详见 `owner-nav-promote-to-global-sidebar` 条目。下文为原始交接说明,保留供参考。

> 写给下一个 session。目标:把 Owner 后台左栏从现在的「任务列表 + 3 个视图(数据看板/AI 预审/数据导出)」**扩成对齐组织方 demo `~/labelhub-ui-demo` SideNav 的三组结构**,并守住职责分离(人工审核动作留在 Reviewer,不进 Owner)。先读本文件,再读 `docs/HANDOFF.md`(部署/项目总览)和 `docs/CHANGELOG.md`(最近改动)。

## 0. 一句话目标
Owner 左栏改成三组、共 7 个栏目;主区一次只显示选中栏目;**人工审核的动作不放进 Owner**(只读"审核结果")。

## 1. 目标结构(就按这个做)

```
① 数据生产
  - 任务管理      列表 / 新建 / 编辑 / 发布·暂停·下线(状态机)
  - 模板搭建      Designer(物料拖拽 + visibleWhen/customRule)
  - 数据集        当前任务题目:导入 JSON/JSONL/Excel · 批量编辑 · 预览
② 审核与质检
  - AI 预审规则   Prompt / 维度 / 阈值 / 模型 + Baseline + 启用开关
  - 黄金评测      golden 样本 + dry-run 回归 + 匹配率/历史
  - 审核结果(只读) 通过率 / AI vs 人工差异 / 三级审核进度  ← Owner 只看,不审
③ 数据交付
  - 数据看板      进度 / 通过率 / 状态分布 / 维度均分
  - 数据导出      多格式 + 下载历史
```

⛔ **不放进 Owner**:人工审核的**动作**(初审/复审/终审队列、通过/打回按钮)→ 归 Reviewer 角色(`/reviewer`)。Owner 的「审核结果」是**只读聚合视图**。这是和组织方 demo 的关键差异点(demo 是单人 walkthrough 把所有屏塞一个侧栏,不是权限模型;我们做了真正的 RBAC,是加分项,别破坏)。

左栏用组分隔标题(参考 `apps/web/src/styles/lh/globals.css` 里的 `.lh-side-section` / `.lh-side-section__title` / `.lh-side-item` / `.lh-side-item--active`,demo 的 `~/labelhub-ui-demo/src/components/SideNav.tsx` 就是这套)。

## 2. 当前实现状态(起点)

文件:`apps/web/src/modules/owner/Dashboard.tsx`(~2000 行,大量内联 style 常量;逻辑/handlers/effects 都在,**不要动逻辑,只动渲染结构**)。

本会话已做(`feat/ui-reskin-s7-and-followups` 分支,PR #2):
- 新增 `detailSection` 状态(`'stats' | 'ai' | 'export'`,默认 `'ai'`)—— 见组件顶部 `useState` 区(`selected` 状态附近)。
- 左列(任务列表所在 `<section>`)在任务列表下方加了一个 `<nav className="lh-side-section" aria-label="任务视图">`,3 个 `lh-side-item` 按钮(数据看板/AI 预审/数据导出),`aria-label="视图 X"` + `aria-pressed`,`onClick={() => setDetailSection(key)}`,仅 `selected` 时渲染。
- 右列把三块各自用 `{detailSection === 'x' && (...)}` 包起来:
  - `'stats'` = `<Suspense><StatsBoard/></Suspense>`
  - `'export'` = `<ExportPanel/>`
  - `'ai'` = Baseline 块(`<div>` 含 `aria-label="baseline_description"`)+ `<section id="ai-prompts">`(Prompt/维度/阈值/模型/启用/Dry-run/Golden/历史,一直到该 `</section>`)
- 任务头部(标题 + 「导出数据」+「模板 Designer」链接)与指标条 `MetricCell` 仍常驻在分节之上。
- Editorial→lh 令牌重映射已在 Dashboard/ExportPanel/StatsBoard 完成(用 `--lh-*`,别再引 `--color-*`/`--font-heading`)。
- 测试 `Dashboard.test.tsx` 加了一条「左栏视图分节切换」用例;**其余 36 个既有用例靠 `detailSection` 默认 `'ai'` 保持不变**(它们 render 后立刻断言 AI 控件可见,任务列表常驻可切任务)。

## 3. 要做的改动(把 3 栏扩成 7 栏 + 三组)

把 `detailSection` 的取值从 `'stats'|'ai'|'export'` 扩成 7 个,并按三组渲染左栏。建议取值:
`'tasks' | 'template' | 'dataset' | 'ai' | 'golden' | 'review' | 'stats' | 'export'`
(`'tasks'`=任务管理、`'template'`=模板搭建、`'dataset'`=数据集、`'ai'`=AI 预审规则、`'golden'`=黄金评测、`'review'`=审核结果、`'stats'`=数据看板、`'export'`=数据导出)。

左栏 nav 改成三个 `lh-side-section`,标题分别「数据生产 / 审核与质检 / 数据交付」,各组下放对应 `lh-side-item`(沿用现有按钮写法 + `aria-label="视图 X"`)。

主区按 `detailSection` 渲染对应内容,**内容来源见第 4 节**(大多是把现有 JSX 块拆细 + 移动,不是新写)。

## 4. 每栏的数据来源与现有组件映射

| 栏目 | 后端端点(均 owner/admin) | 现有前端来源 | 备注 |
|---|---|---|---|
| 任务管理 | `GET/POST /tasks`、`PUT /tasks/:id`、`POST /tasks/:id/{publish,pause,resume,end}` | `TaskManagePanel.tsx`(现在常驻页顶)| 把它移进 `'tasks'` 栏;**任务选择仍在这里**(见第 6 节关于"选任务"的取舍)|
| 模板搭建 | `GET/POST /tasks/:id/templates`、`/templates/:id` | Designer 路由 `/owner/tasks/:id/templates/:templateId` | 用链接跳转即可,别把 Designer 内嵌进 Dashboard |
| 数据集 | `GET /tasks/:id/items`(本会话新增,游标分页)、`/items/import`、`/items/import-file`、`/items/batch-update`、`/item-preview` | `ImportPanel.tsx`(现含「加载题目列表」+ 批量编辑)| 直接把 `ImportPanel` 放进 `'dataset'` 栏 |
| AI 预审规则 | `GET/POST /tasks/:id/ai-prompts`、`/ai-review-settings`、`/baseline` | Dashboard 里 Baseline 块 + `<section id="ai-prompts">` 的**前半**(启用开关 + Prompt 模板 + `DimensionEditor` + 阈值/模型 + 保存配置 + 单次 Dry-run 测试)| 见第 5 节测试影响 |
| 黄金评测 | `/tasks/:id/golden-samples`(增删/单跑/批量)、`/golden-samples/dry-runs`、`GET /tasks/:id/ai-dry-runs` | `<section id="ai-prompts">` 的**后半**:`goldenSampleSectionStyle` 块(Golden Samples)+ `historyPanelStyle` 块(Dry-run 历史/匹配率)| 从 AI 节里拆出来 |
| 审核结果(只读) | **暂无 owner 专用端点**;聚合数据来自 `GET /tasks/:id/stats`(已有 `passRate` / `aiVsHuman{compared,disagree,rate}`)| StatsBoard 已渲染这两项 | 见第 6 节"待定" —— 最省的是复用 stats 的通过率 + AI-vs-人工 + 加一句三级审核进度;若要"按条列出审核结果"需新端点(Reviewer 有 `GET /reviewer/results`,但那是 reviewer/admin 权限,owner 看不到;要么加 owner 版,要么只做聚合)|
| 数据看板 | `GET /tasks/:id/stats` | `StatsBoard.tsx` | 已有 |
| 数据导出 | `POST /tasks/:id/exports`、`GET /tasks/:id/exports`、`/exports/:id/download-url` | `ExportPanel.tsx` | 已有 |

完整路由清单可在仓库跑:`grep -rhE 'api\.(GET|POST|PUT|DELETE)\(' apps/api/internal/handler/*.go | grep -v _test`。

## 5. ⚠️ 测试影响与策略(最关键,先想清楚再动手)

`apps/web/src/modules/owner/Dashboard.test.tsx`(~1850 行,37 个用例,**全部**在一个 `describe('OwnerDashboard AI prompt flow')` 里)。它们的共同假设:`render(<OwnerDashboard/>)` → 首个任务自动选中 → **AI 控件(prompt_template / pass_threshold / dry-run / golden / history)在渲染后立即可见**,无需导航。

现在能不改测试,是因为 `detailSection` 默认 `'ai'` 且 AI 配置 + Dry-run + Golden + 历史**都在同一个 `'ai'` 节**里。
**一旦把 AI 预审规则 与 黄金评测 拆成两个栏目,golden/dry-run/history 那批用例(约 28 个)就不再默认可见 → 会全红。**

两条路线,按工作量选:

- **路线 A(推荐先确认):AI 预审规则 + 黄金评测合为一个左栏项「AI 预审」**(内部上下排),只把 数据集/模板/审核结果/看板/导出 拆出去。这样 7 栏变 6 栏,但 **37 个测试零改动**。视觉上仍是"审核与质检"组里一个「AI 预审」项。性价比最高。
- **路线 B(完全按七栏):AI 规则 与 黄金评测 真拆开。** 必须改 `Dashboard.test.tsx`:给每个用例在 `render` 后、断言前插入一次"导航到对应栏"的点击。建议加 helper:
  ```ts
  async function gotoSection(user, label /* 'AI 预审规则' | '黄金评测' | ... */) {
    await user.click(screen.getByRole('button', { name: `视图 ${label}` }))
  }
  ```
  - Prompt/Baseline/阈值/维度/启用 那批(约 9 个)→ `gotoSection(user, 'AI 预审规则')`
  - golden/dry-run/history 那批(约 28 个)→ `gotoSection(user, '黄金评测')`
  - 注意"切任务"类用例:任务列表必须仍常驻可点(见第 6 节),否则点 Task A/B 会找不到。
  - 默认 `detailSection` 设成哪个都行,但默认那个栏的用例可省 helper。
  - 改完务必 `pnpm -F web exec vitest run src/modules/owner/Dashboard.test.tsx` 全绿。

**先和用户确认走 A 还是 B**(用户原话是要七栏 = 倾向 B;但 B 要改 ~28 个测试,告诉他工作量,让他拍板)。

## 6. 待定决策(动手前定一下)

1. **"选任务"放哪**:现在任务列表常驻左列、点任务即切 `selected`,这保证"切任务"测试和跨栏共享选中任务都简单。若按 demo 把「任务管理」做成一个**栏目**(主区显示 TaskManagePanel+列表),那点任务后要不要自动跳到某个栏?跨栏时"当前任务"如何常驻可见(建议在左栏顶部加一个常驻的"当前任务"选择器/下拉,或保留一个迷你任务列表)。**推荐**:左栏顶部放常驻"当前任务"选择器 + 下面三组 nav;「任务管理」栏只放新建/编辑/状态机/导入等管理动作。
2. **审核结果(只读)的数据**:最省 = 复用 `/tasks/:id/stats` 的 `passRate` + `aiVsHuman` + 文案化三级审核进度(无需新端点)。若要"逐条审核结果列表",需要加一个 owner 版端点(`GET /tasks/:id/reviews` 之类,按任务归属鉴权),Reviewer 的 `/reviewer/results` 不能直接复用(权限不同)。**先做聚合版**,逐条列表列为后续。
3. **默认栏目**:建议默认 `'tasks'`(任务管理)做正常 UX;但若走路线 B 又想省测试,可临时默认到测试最多的那个栏(权衡见第 5 节)。
4. **左栏是页内还是全局**:现在视图 nav 在 Owner 页内左列(和任务列表同列)。要不要提升到 `AppLayout` 最外层侧栏(`apps/web/src/shared/layout/AppLayout.tsx`,跨角色共享、有独立测试 `AppLayout.test.tsx`)?**不推荐**动 AppLayout(会牵连其它角色 + 它的测试);保持页内左栏即可,视觉上加三组标题已足够像 demo。

## 7. 要改的文件
- `apps/web/src/modules/owner/Dashboard.tsx` —— 主改:`detailSection` 取值扩展、左栏三组 nav、主区分栏渲染(移动现有 JSX 块)。
- `apps/web/src/modules/owner/Dashboard.test.tsx` —— 路线 B 必改(加导航);路线 A 基本不动。
- (可能)`apps/web/src/modules/owner/Dashboard.css` 或内联 —— 三组 nav 间距等微调;优先复用 `lh-side-*` 既有类。
- 若做"逐条审核结果":`apps/api/internal/handler/*.go` 加 owner 端点 + 测试(否则只做聚合,后端不动)。

## 8. 部署 + 验证 SOP(照 docs/HANDOFF.md)
- 改完先本地:`pnpm -F web exec tsc -b` → `pnpm -F web exec vitest run src/modules/owner` → `pnpm -F web build`,**全绿再部署**。
- 部署(无 CI 到 prod,rsync + 重建):
  ```bash
  rsync -az -e "ssh -i ~/Downloads/labelhub.pem" --exclude=node_modules --exclude=dist \
    /Users/dadadineiyou/Desktop/LabelHub/apps/web/src/ ubuntu@43.155.210.70:/home/ubuntu/labelhub/apps/web/src/
  ssh -i ~/Downloads/labelhub.pem ubuntu@43.155.210.70 \
    'cd ~/labelhub && docker compose --env-file deploy/.env -f deploy/docker-compose.prod.yml up -d --build web'
  ```
  （后台跑,~2-4 分钟;`curl -s -o /dev/null -w "%{http_code}" http://43.155.210.70/` 应 200）
- 浏览器冒烟(chrome-devtools MCP):`localStorage.clear()` → 用 `/api/v1/auth/login` 拿 token 设 `labelhub_access_token`+`labelhub_current_user`(owner1 / 123456)→ 进 `/owner` → 选任务 → 逐栏点一遍确认主区切换、AI 控件/看板/导出各就各位、console 无 error。**用完关页面**。
- 集成测试(若动了后端)要 Colima env:`DOCKER_HOST=unix:///Users/dadadineiyou/.colima/default/docker.sock TESTCONTAINERS_RYUK_DISABLED=true go test -tags=integration ./internal/integration -count=1`。

## 9. Gotchas
- **别动 Dashboard 的 state/handlers/effects/序号守护/API 调用**,只重排 JSX + 加 `detailSection` 分支。
- 登录密码统一 `123456`(owner1/labeler1/reviewer1,admin1)。
- 不要 spawn 后台 dev server(吃内存);浏览器冒烟后关页面。
- 保持 lh 令牌(`--lh-*`),别回退 `--color-*`/`--font-heading`(Editorial)。
- `git add <具体文件>`,不要 `git add -A`;提交不带 AI 署名。
- 人工审核动作**始终**留在 Reviewer 角色,Owner 只读审核结果。

## 10. Git 状态(交接时)
- 分支 `feat/ui-reskin-s7-and-followups`,已推 origin,**PR #2 开着**(`main ← feat`,CI 绿:test + integration 都过,mergeStateStatus=CLEAN),**用户尚未合并**。
- 本次 Owner-nav 重排建议**继续在 `feat/ui-reskin-s7-and-followups` 上做**(main 还没同步这些工作),作为新 commit;或等用户合并 PR #2 后从 main 切新分支。先问用户。
- 生产 http://43.155.210.70 已是包含左栏 3 视图版本的最新部署。

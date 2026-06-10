# Demo Polish (主链路走查 5 坑) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement task-by-task. Steps use checkbox (`- [ ]`) syntax.

**Goal:** 修掉 2026-06-03 主链路端到端走查发现的 5 个会拉低 demo 体验 / 像 bug 的坑(详见 CHANGELOG 走查记录),全部在线上 43.155.210.70 复验。

**Architecture:** 4 个前端改动(`apps/web`)+ 1 个纯部署动作(Caddy 重载)。无后端代码改动,无新依赖(markdown 复用已有 `renderMarkdown` 并扩表格/加粗)。每个 fix 独立 commit,可独立回退。

**Tech Stack:** React 18 + TS + Semi-UI(前端);Caddy(反代 CSP);Docker Compose(部署)。本机 swap 会 OOM kill 本地 `vite build` → 用 `tsc -b` + `eslint` + 单文件 `vitest` 验证,生产 bundle 由服务器侧 `docker compose up -d --build web` 验证。

---

## Fix 1 — 字体 CSP(纯部署,无代码)

**根因**:`deploy/Caddyfile:12` 仓库里**已经**放行 `https://fonts.googleapis.com`(style-src)和 `https://fonts.gstatic.com`(font-src),但线上 `deploy-caddy-1` 容器 up 4 天、跑的是旧配置(实测 live header 缺这两源),所以 Montserrat 被 CSP 挡、全站回退系统字体。**仓库无需改,重建 caddy 容器即可。**

- [ ] **Step 1**:确认仓库 Caddyfile 已含字体源(只读核对,不改)
  Run: `grep -n "fonts.googleapis\|fonts.gstatic" deploy/Caddyfile`
  Expected: `style-src ... https://fonts.googleapis.com`、`font-src ... https://fonts.gstatic.com` 都在。
- [ ] **Step 2**:线上重建 caddy 容器(应用当前 Caddyfile)
  ```bash
  ssh -i ~/Downloads/labelhub.pem ubuntu@43.155.210.70 \
    'cd /home/ubuntu/labelhub && docker compose --env-file deploy/.env -f deploy/docker-compose.prod.yml up -d caddy'
  ```
- [ ] **Step 3**:验证 live CSP 已含字体源
  Run: `curl -sI http://43.155.210.70/ | grep -i content-security`
  Expected: 出现 `https://fonts.googleapis.com` 与 `https://fonts.gstatic.com`。
- [ ] **Step 4**:chrome 复验 `/owner/ai` console 不再报 Google Fonts CSP 错、标题字体变 Montserrat。

> 无 commit(无代码改动);记进 CHANGELOG。

---

## Fix 2 — INVALID_STATE 误导文案(前端)

**根因**:`apps/web/src/shared/api/client.ts:16-19` 的 `FRIENDLY_ERROR_BY_CODE` 把 `INVALID_STATE` **无条件**替换成"状态已被改动",吞掉后端真实 message(如 `task must have a bound template before publishing`)。

**改法(前端 only,不动 Go,无需 api 重部署)**:把 `INVALID_STATE` 从无条件映射改成**按后端 message 内容分流**的解析器。

**Files:** Modify `apps/web/src/shared/api/client.ts`;Test `apps/web/src/shared/api/client.test.ts`(若无则新建)

- [ ] **Step 1**:写失败测试(若无测试文件则建)
  ```ts
  import { describe, it, expect } from 'vitest'
  import { ApiError } from './client'
  describe('ApiError INVALID_STATE 文案', () => {
    it('没绑模板 → 提示先搭模板,而非"状态被改动"', () => {
      const e = new ApiError('INVALID_STATE', 'task must have a bound template before publishing', 'r1')
      expect(e.message).toContain('模板')
      expect(e.message).not.toContain('被其他人改动')
    })
    it('状态不对/冻结 → 提示刷新重试', () => {
      const e = new ApiError('INVALID_STATE', 'task cannot publish from status published', 'r1')
      expect(e.message).toContain('刷新')
    })
    it('LLM_PROVIDER_ERROR 文案不变', () => {
      const e = new ApiError('LLM_PROVIDER_ERROR', 'x', 'r1')
      expect(e.message).toContain('AI 服务')
    })
  })
  ```
  Run: `./node_modules/.bin/vitest run src/shared/api/client.test.ts` → Expected: FAIL。
- [ ] **Step 2**:实现 —— 把 `FRIENDLY_ERROR_BY_CODE[code]` 的直查改成 `resolveFriendlyMessage(code, backendMessage)`:
  ```ts
  function resolveFriendlyMessage(code: string, backendMessage: string): string | null {
    if (code === 'INVALID_STATE') {
      const msg = backendMessage.toLowerCase()
      if (msg.includes('template')) return '请先为该任务搭建并保存模板,再发布。'
      return '该任务状态已变化(可能已发布或已被改动),请刷新后重试。'
    }
    if (code === 'LLM_PROVIDER_ERROR') return 'AI 服务暂时不可用,可稍后重试,或直接转人工审核。'
    return null
  }
  ```
  构造函数改为 `super(resolveFriendlyMessage(code, backendMessage) ?? backendMessage ?? '请求失败')`。删除旧 `FRIENDLY_ERROR_BY_CODE` 常量(被新函数取代)。
- [ ] **Step 3**:`./node_modules/.bin/vitest run src/shared/api/client.test.ts` → PASS;`tsc -b`、`eslint src/shared/api/client.ts` 绿。
- [ ] **Step 4**:Commit `git add apps/web/src/shared/api/client.ts apps/web/src/shared/api/client.test.ts && git commit -m "fix(web): show actionable INVALID_STATE messages instead of generic stale-record text"`

---

## Fix 3 — Owner 默认空草稿 + 任务选择不持久化(前端)

**根因**:`apps/web/src/modules/owner/Dashboard.tsx:199` `initialTask = data.find(id===requestedTaskId) ?? data[0]`。`data[0]` 是按更新时间排首位的草稿 #4(空),且 `selected` 是组件本地 state、切 owner 子页重挂即重置 → 每次默认空草稿、选了就忘。

**改法**:① 默认优先选"有数据/已发布"的任务而非草稿;② 把最后选中的 taskId 持久化到 localStorage,跨子页/刷新沿用。

**Files:** Modify `apps/web/src/modules/owner/Dashboard.tsx`

- [ ] **Step 1**:加一个 localStorage 持久化 helper(文件内,常量 key)
  ```ts
  const LAST_TASK_KEY = 'labelhub_owner_last_task_id'
  function readLastTaskId(): number | null {
    const raw = localStorage.getItem(LAST_TASK_KEY)
    const n = raw ? Number(raw) : NaN
    return Number.isFinite(n) ? n : null
  }
  function writeLastTaskId(id: number | null) {
    if (id == null) return
    localStorage.setItem(LAST_TASK_KEY, String(id))
  }
  ```
- [ ] **Step 2**:改默认解析(line ~199),优先级 = URL requestedTaskId → localStorage 上次 → 首个非草稿 → data[0]:
  ```ts
  const initialTask =
    data.find((t) => t.id === requestedTaskId) ??
    data.find((t) => t.id === readLastTaskId()) ??
    data.find((t) => t.status !== 'draft') ??
    data[0] ?? null
  ```
- [ ] **Step 3**:在 `selectTask`(line ~389)里写持久化:设置 `selectedTaskIdRef.current = task.id` 后加 `writeLastTaskId(task.id)`;并在 line ~201 `setSelected(initialTask)` 后 `writeLastTaskId(initialTask?.id ?? null)`。
- [ ] **Step 4**:`tsc -b`、`eslint src/modules/owner/Dashboard.tsx`、`./node_modules/.bin/vitest run src/modules/owner/Dashboard.test.tsx` 全绿(若既有测试对默认选中有断言,按"非草稿优先"更新断言——seed 测试任务多为 published,通常不受影响;若 mock 数据全 draft 需补 localStorage mock)。
- [ ] **Step 5**:Commit `git add apps/web/src/modules/owner/Dashboard.tsx && git commit -m "fix(web): persist owner task selection and default to a task with data"`

> **遗留草稿 #4「商品标题清洗」**(test 脏数据)本计划不删 prod 数据;Fix 3 后它不再是默认,影响已消除。如要彻底从列表移除,需用户授权删 DB 行或后续加"删除草稿任务"功能(本轮 out of scope)。

---

## Fix 4 — 验收基线 markdown 不渲染(前端,无新依赖)

**根因**:`apps/web/src/modules/labeler/TaskPlaza.tsx:159` 把 `task.baselineDescription` 当纯文本渲染(`#`/`|表格|`/`>` 全是源码)。平台已有 `renderMarkdown`(`renderer/widgets/ShowItem.tsx:117`)但只处理标题/链接/图片/视频,无表格/加粗。

**改法**:把 `renderMarkdown` 提取到 `shared/markdown.tsx`,扩**连续 `|...|` 行 → `<table>`**、**`**粗体**`**,ShowItem 与 baseline 共用。无新依赖。

**Files:** Create `apps/web/src/shared/markdown.tsx`;Modify `apps/web/src/renderer/widgets/ShowItem.tsx`(import 共用)、`apps/web/src/modules/labeler/TaskPlaza.tsx`;Test `apps/web/src/shared/markdown.test.tsx`

- [ ] **Step 1**:失败测试
  ```tsx
  import { render, screen } from '@testing-library/react'
  import { Markdown } from './markdown'
  it('## 标题渲染为 heading', () => {
    render(<Markdown text={'## 任务背景'} />)
    expect(screen.getByRole('heading', { name: '任务背景' })).toBeInTheDocument()
  })
  it('管道行渲染为表格', () => {
    render(<Markdown text={'| 字段 | 含义 |\n|---|---|\n| id | 编号 |'} />)
    expect(screen.getByRole('table')).toBeInTheDocument()
    expect(screen.getByText('编号')).toBeInTheDocument()
  })
  ```
  Run: `./node_modules/.bin/vitest run src/shared/markdown.test.tsx` → FAIL。
- [ ] **Step 2**:建 `shared/markdown.tsx` —— 移植 ShowItem 的 `renderMarkdown`(含 video/image/link 处理),包成 `export function Markdown({ text }: { text: string })`,新增:① 预扫描把连续 `| a | b |` 行(跳过 `|---|` 分隔)聚成 `<table>`;② `**x**` → `<strong>`。安全:链接/媒体沿用 `isSafeURL`。
- [ ] **Step 3**:`ShowItem.tsx` 删掉本地 `renderMarkdown`,改 `import { Markdown }`,markdown 分支用 `<Markdown text={...} />`(保留既有 ShowItem 集成测试断言;媒体处理逻辑随移植保留)。
- [ ] **Step 4**:`TaskPlaza.tsx:159` `<div className="lz-detail__body">{task.baselineDescription}</div>` 改 `<div className="lz-detail__body"><Markdown text={task.baselineDescription} /></div>`。
- [ ] **Step 5**:`tsc -b`、`eslint`、`vitest run src/shared/markdown.test.tsx src/renderer/SchemaRenderer.test.tsx src/modules/labeler/Plaza.test.tsx` 全绿。
- [ ] **Step 6**:Commit `git add apps/web/src/shared/markdown.tsx apps/web/src/shared/markdown.test.tsx apps/web/src/renderer/widgets/ShowItem.tsx apps/web/src/modules/labeler/TaskPlaza.tsx && git commit -m "feat(web): render task baseline and ShowItem markdown with tables and bold"`

---

## Fix 5 — 模板复用(从现有任务复制,前端 only)

**根因**:新建任务只能从零搭模板,无复用入口。`TemplateList.tsx:42` 有"+ 新建模板"→ `/templates/new`;`Designer.tsx:130` `isNew` 进空白态;后端已有 `GET /tasks/:id`(带 template)+ `POST /tasks/:id/templates`(createTemplate)。

**改法(纯前端,复用现有 API)**:TemplateList 加"从现有任务复制"下拉(列出**其他有模板的任务**)→ 导航到 `/owner/tasks/:taskId/templates/new?copyFrom=<sourceTaskId>` → Designer 在 `isNew` 且有 `copyFrom` 时,拉源任务模板 schema 预填画布(而非空白),用户复核后保存即 `POST` 创建本任务首版。

**Files:** Modify `apps/web/src/modules/template/TemplateList.tsx`、`apps/web/src/modules/template/Designer.tsx`;client.ts 若缺 `getTaskTemplate`/`listTasks` 则补;Test `Designer.integration.test.tsx` 加 copyFrom 用例

- [ ] **Step 1**:client.ts 确认/补两个读 API:`listTasks()`(取候选任务,可能已存在)、`getTaskTemplateSchema(taskId)`(GET /tasks/:id 取 template.schema)。若已有等价函数则复用,不重复造。
- [ ] **Step 2**:`TemplateList.tsx` —— 在"+ 新建模板"旁加 `<select>`:`从现有任务复制…`,options = 拉 `listTasks()` 里 `id !== 当前 taskId` 且有模板的任务;选中即 `navigate('/owner/tasks/'+taskId+'/templates/new?copyFrom='+sourceId)`。(原生 select,对齐本仓表单约定。)
- [ ] **Step 3**:`Designer.tsx` —— `isNew` 分支读 `useSearchParams().get('copyFrom')`;若有,`useEffect` 拉 `getTaskTemplateSchema(copyFrom)` 把 schema 灌进画布 state(与加载已有模板同一条 setState 路径,但不绑定 templateId、保持新建态);加载中显示 loading,失败 Toast 并退回空白态。
- [ ] **Step 4**:失败测试 → 实现 → 测试绿:`Designer.integration.test.tsx` 加 `renderDesigner('/owner/tasks/2/templates/new?copyFrom=1')` → 断言画布出现源任务字段(如 `source_display`)、且为新建态(显示"新建模板"、保存走 POST /tasks/2/templates)。
- [ ] **Step 5**:`tsc -b`、`eslint`、`vitest run src/modules/template/Designer.integration.test.tsx` 全绿。
- [ ] **Step 6**:Commit `git add apps/web/src/modules/template/TemplateList.tsx apps/web/src/modules/template/Designer.tsx apps/web/src/shared/api/client.ts apps/web/src/modules/template/Designer.integration.test.tsx && git commit -m "feat(web): copy an existing task's template into a new task"`

---

## 收尾 — 部署 + 复验 + 文档

- [ ] **D1**:四个前端 commit 完成后,rsync 前端 src + 重建 web 容器:
  ```bash
  rsync -az -e "ssh -i ~/Downloads/labelhub.pem" apps/web/src/ ubuntu@43.155.210.70:/home/ubuntu/labelhub/apps/web/src/
  ssh -i ~/Downloads/labelhub.pem ubuntu@43.155.210.70 \
    'cd /home/ubuntu/labelhub && docker compose --env-file deploy/.env -f deploy/docker-compose.prod.yml up -d --build web'
  ```
  (Fix 1 的 caddy 重建已在 Fix1-Step2 单独做。)
- [ ] **D2**:chrome 复验:① console 无 Google Fonts CSP 错 + Montserrat 生效(#1);② 新建空草稿发布 → 提示"先搭模板"(#2);③ owner 切子页保持选中任务、默认非空草稿(#3);④ labeler 任务详情基线渲染成标题+表格(#4);⑤ TemplateList 复制现有任务模板进新建画布(#5)。
- [ ] **D3**:`docs/CHANGELOG.md` 追加一条 `demo-polish-5fixes`(本次走查 + 5 修复 + 验证);更新 `CLAUDE.md` handoff。
- [ ] **D4**:关闭 chrome-devtools 浏览器释放内存(`pkill -f chrome-devtools-mcp/chrome-profile`)。

## Out of scope(本轮不做,记录)
- 删除遗留草稿 #4 prod 数据(需用户授权 / 或后续加"删除草稿任务"功能)。
- 把 INVALID_STATE 拆成后端独立错误码(更彻底,但需 Go 改 + api 重部署,本轮用前端 message 分流够用)。
- markdown 换 react-markdown/remark-gfm(本轮复用自写渲染器扩表格/加粗,零新依赖)。

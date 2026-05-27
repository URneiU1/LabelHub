# S6 Implementation Plan — Editorial Polish + Bonus Materials

> 配套 `docs/PLAN.md` §Sprint 6。S6 不新增后端业务主链路,目标是把已完成的标注系统打磨成可演示、可高密度操作、状态清晰的后台产品,并补齐 Sprint 2 延后的 Tabs / Group 加分物料体验。

## 0. Current State

### 已有基础

- React/Semi/Vite 前端主线已经稳定,`strict:true`、RTL 测试和 build/lint 门禁均已通过。
- `SchemaRenderer` 已能递归渲染 `Group.fields` 和 `Tabs.tabs[].fields`,答案模型保持 flat object。
- Designer 已能添加 11 类 widget,`Group` / `Tabs` 的子字段可在属性面板里增删改、保存、parse、round-trip 到 Renderer。
- S5 已有 `ErrorBoundary`,OpenAPI/Postman,生产 compose,`make dev`。

### 主要差距

| Area | Gap | S6 action |
|---|---|---|
| Visual system | `tokens.css` 仍是 Schematic/Inter/默认蓝,与 PLAN.md Editorial 风格不一致 | 收敛为 Editorial console tokens,覆盖 Semi 常用控件和状态色 |
| Status clarity | 多处直接显示 raw `status`,颜色/文案不统一 | 新增共享 `StatusBadge`,覆盖 7 态 + export/AI dry-run 状态 |
| Loading/empty/error | 多处是裸文本 loading/error | 新增共享 `LoadingBlock`,`EmptyState`,顶部 progress bar |
| Information density | Owner/Labeler/Reviewer 局部仍偏 card/list,扫描成本高 | 高密度表格/行布局,保留必要详情面板 |
| Designer responsive | 当前固定三栏;1280 宽度下易挤压 | 1280 默认两栏 + 属性抽屉/折叠栏;1920 展开三栏 |
| Tabs material | Renderer 目前按 tab section 全量堆叠,不是真正 tab 交互 | Runtime Tabs 改为真实 tab switch,只显示 active pane |
| Group/Tabs Designer | 嵌套子字段主要在属性面板编辑,画布内缺少嵌套结构感 | Canvas 渲染嵌套 child preview + 子字段顺序/选择入口;拖拽若超时则保留 Up/Down fallback |
| A11y | focus ring/label 依赖浏览器默认,不统一 | 全局 focus-visible + 关键表单 label/aria 补齐 |

## 1. Non-Goals / Cuts

这些不是 S6 必做,时间紧时直接砍:

- Dark mode。
- Framer Motion 或额外动效库。
- 移动端全量体验;只保证主要页面在窄屏不重叠。
- TanStack Query / axios 迁移。
- Formily / dnd-kit 迁移。
- 新后端业务 API。

## 2. Day 1 — Theme Foundation And Shared UI

### Task 1.1: Editorial tokens

Files:

- `apps/web/src/styles/tokens.css`
- optional: `apps/web/src/shared/components/StatusBadge.tsx`
- optional: `apps/web/src/shared/components/EmptyState.tsx`
- optional: `apps/web/src/shared/components/TopProgress.tsx`

Actions:

- Replace Schematic wording/tokens with Editorial console tokens.
- Use system serif for headings (`Georgia`/`Times` fallback), system sans for body, mono for codes.
- Avoid one-note palette. Keep neutral paper surface + black text + limited blue/green/orange/red/purple status accents.
- Add global Semi overrides for Button/Table/Modal/Form/Tag/Nav where CSS selectors are stable enough.
- Add `:focus-visible` ring.

Verification:

```bash
pnpm -F web lint
pnpm -F web build
```

### Task 1.2: Shared state components

Implement:

- `StatusBadge`: maps exactly:
  - `draft` grey
  - `submitted` blue
  - `ai_reviewing` purple
  - `human_reviewing` orange
  - `approved` green
  - `rejected` red
  - `revising` yellow
  - queue/export statuses: `queued`,`running`,`succeeded`,`failed`,`dead`
- `EmptyState`: serif title + short body + optional action, with simple line SVG variants.
- `LoadingBlock`: skeleton-like block without adding a dependency.
- `TopProgress`: NProgress-style top bar driven by route/module loading props, no external package.

Tests:

- `StatusBadge.test.tsx`: all 7 state labels/classes.
- `EmptyState.test.tsx`: title/body/action render.

## 3. Day 2 — Dense Owner / Labeler / Reviewer Surfaces

### Task 2.1: Owner Dashboard density pass

Files:

- `apps/web/src/modules/owner/Dashboard.tsx`
- `apps/web/src/modules/owner/StatsBoard.tsx`
- `apps/web/src/modules/owner/ExportPanel.tsx`

Actions:

- Replace raw status spans with `StatusBadge`.
- Make task list and export history scan like dense operational tables/rows.
- Keep charts, prompt controls, and golden sample sections compact; no nested cards.
- Replace naked loading/error strings with `LoadingBlock` / `EmptyState`.

Tests:

- Existing `Dashboard.test.tsx`, `StatsBoard.test.tsx`, `ExportPanel.test.tsx` updated only where copy/roles change.

### Task 2.2: Labeler and Reviewer density pass

Files:

- `apps/web/src/modules/labeler/Plaza.tsx`
- `apps/web/src/modules/reviewer/Queue.tsx`

Actions:

- Labeler task/revision lists use compact rows with clear status badge and primary action.
- Reviewer queue remains high-density: left list/table, center answer, right AI/audit/rules details.
- Batch/retry/revise buttons keep loading states and disabled states.
- Friendly messages for no task / no review / AI failure.

Tests:

```bash
pnpm -F web test -- Dashboard Plaza Queue ExportPanel StatsBoard
```

## 4. Day 3 — Designer Responsive Layout + Real Tabs/Group Experience

### Task 3.1: Responsive Designer shell

Files:

- `apps/web/src/modules/template/Designer.tsx`
- optional: `apps/web/src/modules/template/designer.css`

Actions:

- Move only layout-critical styles to CSS classes with media queries.
- 1280×800:
  - palette + canvas visible;
  - property panel is collapsed/drawer-like or below canvas;
  - text/buttons do not overlap.
- 1920×1080:
  - full three-column layout visible.
- Keep existing drag/order behavior for top-level fields.

Verification:

- Browser screenshots at 1280×800 and 1920×1080.
- `pnpm -F web test -- Designer.integration`

### Task 3.2: Runtime Tabs as real tab interaction

Files:

- `apps/web/src/renderer/SchemaRenderer.tsx`
- `apps/web/src/renderer/SchemaRenderer.test.tsx`
- possible extraction: `apps/web/src/renderer/components/RenderField.tsx`

Actions:

- Render `Tabs` as tab buttons + one active panel instead of stacked sections.
- Preserve flat answer object and validation across hidden tabs.
- `Group` remains nested section but gets clearer visual grouping.
- Add RTL test:
  - initial first tab visible;
  - clicking second tab hides first tab fields and reveals second tab fields;
  - answer values persist when switching tabs.

### Task 3.3: Designer nested material usability

Files:

- `apps/web/src/modules/template/Designer.tsx`
- `apps/web/src/modules/template/Designer.integration.test.tsx`

Actions:

- Canvas field for `Group`/`Tabs` shows nested children as editable mini rows, not only a generic preview.
- Child rows can be selected from canvas and edited in the existing property controls.
- Child order supports Up/Down at minimum; drag inside nested containers is optional and can be cut if it risks destabilizing tests.
- Add test for adding Tabs/Group, reordering child fields, saving, then renderer shows real nested/tabbed output.

## 5. Day 4 — Friendly States, Error Copy, A11y

### Task 4.1: Error copy normalization

Files:

- `apps/web/src/shared/api/client.ts`
- `apps/web/src/shared/components/*`
- touched module pages only as needed

Actions:

- Preserve backend `{code,message}` but surface Chinese action copy.
- For state-machine illegal transition / AI provider failure, use specific messages:
  - illegal transition: explain current status and ask refresh/retry;
  - LLM failure: offer retry or move to human review where such actions already exist.
- Avoid swallowing `request_id`; include it in console/debug text only if useful.

### Task 4.2: A11y and keyboard quick wins

Actions:

- Ensure inputs/selects in Designer nested controls have labels.
- Add focus rings for buttons/links/inputs.
- Optional if Day 1-3 are stable:
  - Labeler `Ctrl/Cmd+Enter` submit shortcut.
  - Reviewer `A/R/B` approve/revise/reject shortcuts.

Verification:

```bash
pnpm -F web test
pnpm -F web lint
pnpm -F web build
```

## 6. Day 5 — Visual QA And Acceptance

### Task 5.1: Browser smoke

Use local dev server and Browser plugin:

- `/auth/login`
- `/owner`
- `/owner/tasks/:taskId/templates/:templateId`
- `/labeler`
- `/reviewer`
- `/style-guide`

Viewports:

- 1280×800
- 1920×1080
- 390×844 smoke only, no full mobile feature promise

Checks:

- No console error.
- No obvious text overlap.
- Designer usable at 1280×800 and 1920×1080.
- Status colors consistent.
- Tabs switch works in renderer and Designer preview.

### Task 5.2: S6 acceptance doc

Create `docs/S6_ACCEPTANCE.md`:

- Final checklist.
- Screenshots taken / browser targets checked.
- Test commands and known warnings.
- Explicitly list cut optional items, if any.

### Final verification

```bash
pnpm -F web test
pnpm -F web lint
pnpm -F web build
go test ./apps/api/... ./apps/ai-worker/... ./pkg/exporter ./pkg/llmreview -count=1
git diff --check
```

If Lighthouse CLI is already available locally, run against the built preview and record Best Practices. If not available, do not install a new global dependency just for S6; record browser smoke as the fallback.

## 7. Commit Plan

Use small commits:

1. `docs(s6): add editorial polish implementation plan`
2. `feat(web): add editorial state primitives`
3. `refactor(web): densify core work surfaces`
4. `feat(renderer): make tabs and group materials interactive`
5. `feat(web): polish designer responsive layout`
6. `docs(s6): add acceptance evidence`

Each code-writing commit updates `README.md`, `CODEX.md`, and `CLAUDE.md` with completed work, remaining cuts, next step, and verification.

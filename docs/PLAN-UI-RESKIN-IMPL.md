# UI Re-skin (labelhub-ui-demo look on apps/web engine) Implementation Plan

> **For agentic workers:** Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the deployed LabelHub frontend look like `~/labelhub-ui-demo` (Arco-style `.lh-` design system + its page layouts) while keeping `apps/web`'s working engine (API client, auth, role routing, SchemaRenderer, all 3-role flows wired to the live Go backend + doubao).

**Architecture:** Re-skin `apps/web` in place — port ui-demo's `styles/` design tokens + layout shell + per-page layouts INTO `apps/web`, rebuilding each module's presentation to call the existing hooks/API verbatim. The hard logic (SchemaRenderer 11 widgets, parser, validator, autosave stale-guard, review/export/stats flows) is reused unchanged. Role-based login is kept; ui-demo's open demo nav is NOT adopted. Delivery is role-by-role: Labeler → Reviewer → Owner, deploying after each phase.

**Tech Stack (LOCKED 2026-05-29, per organizer recommendation):** React 18 + TypeScript · **Semi Design (`@douyinfe/semi-ui`) — KEPT** (organizer's own lib, already wired; do NOT introduce Arco, do NOT remove Semi) · form = our existing self-built **Schema renderer** (`src/renderer/*`, satisfies the recommended "Schema 渲染" paradigm; not migrating to Formily) · state = context/hooks + Zustand (already a dep) where needed · drag = native HTML5 today, optionally `@dnd-kit/core` when rebuilding the Designer (Phase 3) · **ui-demo look = ported `.lh-` CSS painted on top of Semi components** · existing `src/shared/api/client.ts` reused.

> **Override note:** Earlier Task 0.1 "add Arco" and Phase 4 "remove Semi" are CANCELLED. Phase 0.1 was completed WITHOUT Arco (lh CSS only). Phase 4 keeps Semi; it only removes the unused Editorial `tokens.css`/StyleGuide if fully orphaned.

**Source of truth for the look:** `/Users/dadadineiyou/labelhub-ui-demo/src/` — copy CSS verbatim, replicate JSX layout per page, but swap hardcoded data for the real hooks.

**Deploy after each phase:** rsync apps/web → server, `docker compose ... up -d --build web`, verify externally at http://43.155.210.70.

---

## Decisions locked (from user)
- Keep **role-based login** (owner1/labeler1/reviewer1, pwd 123456). Each role enters its own area; no flat demo nav.
- **Role-by-role**, deploy each when done. Order: **Labeler → Reviewer → Owner/Designer → Owner admin pages**.
- Re-skin `apps/web` (reuse engine), do NOT rebuild data layer inside ui-demo.

## Known mapping (ui-demo page → apps/web module)
| ui-demo page | apps/web module to re-skin | Reused logic |
|---|---|---|
| `Workbench.tsx` | `modules/labeler/Plaza.tsx` | claim / draft autosave / submit / llm inline; **center form = existing SchemaRenderer** |
| `HumanReview.tsx` + `AiReview.tsx` | `modules/reviewer/Queue.tsx` | queue / detail / approve-reject-revise / batch / AI verdict / rules |
| `Tasks.tsx` + `OwnerSection.tsx` | `modules/owner/Dashboard.tsx` (+ ExportPanel/StatsBoard) | tasks / AI prompt / golden / stats / export |
| `Designer.tsx` | `modules/template/Designer.tsx` | draft schema / drag reorder / widget config / save |

## Risk notes
- apps/web has its own `src/styles/tokens.css` (Editorial). ui-demo tokens use a different `.lh-*` namespace + Arco blue. They can coexist: new layout/pages use `.lh-*`; Semi pages keep their tokens until replaced. Remove Semi + Editorial tokens only in the final cleanup phase.
- Mixing Arco + Semi transiently bloats the bundle. Acceptable mid-migration; Phase 5 removes Semi once unused.
- ui-demo `Datasets` page has no backend concept in apps/web. Map to a real list (task items count per task) or mark visual-only — decide in Phase 4, log the choice.
- ui-demo `AiReview` as a standalone "AI queue monitor" has no direct apps/web equivalent (asynqmon covers ops). Fold its visual treatment (score bars, verdict box, audit timeline) into reviewer detail rather than building a new live queue page.

---

## Phase 0.5: Import real demo data (no app code; do first)

Source: `~/Downloads/datasets.zip` → `qa_quality` (30 items) + `preference_compare` (12 items). Target: live Seoul prod DB.

### Task 0.5.1: Determine import mechanism
- [ ] **Step 1:** Inspect the backend item-import path: `apps/api/internal/handler/task.go` `ImportItems` (route `POST /tasks/:id/items` import) — confirm expected JSON body shape (`external_id` + `payload` per item, or array). Cross-check with how `cmd/seed` inserts items so the payload keys match the task template's ShowItem `$payload.*` paths.
- [ ] **Step 2:** Decide loader: (a) call the import API as owner1 from local with a transformed JSON, or (b) extend a one-off seed container. Prefer (a) if the API accepts the dataset shape with minimal transform.

### Task 0.5.2: Import the 42 items into prod
- [ ] **Step 1:** Transform each dataset record into the import shape: `external_id` = item `id` (Q0001.../P0001...), `payload` = the full record object. Keep payload keys intact so template ShowItem paths resolve.
- [ ] **Step 2:** Import qa_quality (30) and preference_compare (12) into their live task IDs (look up task IDs via `GET /labeler/tasks` as owner1). Verify counts via API.
- [ ] **Step 3:** Confirm a labeler can claim and the SchemaRenderer shows the real prompt/model_answer/responses.

### Task 0.5.3: Pre-generate real submissions across states
- [ ] **Step 1:** Drive the real flow (or a scripted sequence of real API calls) to produce a spread: a few drafts, several submitted→AI-reviewed (real doubao), some human-approved, some revised/rejected — so reviewer queue, stats, and export pages are non-empty with genuine data.
- [ ] **Step 2:** Verify reviewer queue shows real items with AI verdicts; `GET /tasks/:id/stats` returns non-zero progress/pass-rate; an export download contains real rows.
- [ ] **Step 3:** Log what was generated (counts per state) — no silent fabrication.

---

## Phase 0: Design foundation (shared, once)

### Task 0.1: Bring ui-demo design tokens + Arco into apps/web

**Files:**
- Create: `apps/web/src/styles/lh/tokens.css` (copy of ui-demo `src/styles/tokens.css`)
- Create: `apps/web/src/styles/lh/globals.css` (copy of ui-demo `src/styles/globals.css`)
- Modify: `apps/web/src/main.tsx` (import Arco CSS + lh tokens/globals)
- Modify: `apps/web/package.json` (add `@arco-design/web-react`)

- [ ] **Step 1:** `pnpm -F web add @arco-design/web-react` (match ui-demo version `^2.66.15`).
- [ ] **Step 2:** Copy ui-demo `src/styles/tokens.css` and `globals.css` into `apps/web/src/styles/lh/` verbatim. Confirm all class names are `.lh-`/page-prefixed (no global element resets that would clobber Semi pages — if `globals.css` has a `body{}`/`*{}` reset, scope it under a `.lh-root` wrapper instead).
- [ ] **Step 3:** In `main.tsx`, add `import '@arco-design/web-react/dist/css/arco.css'` and `import './styles/lh/tokens.css'; import './styles/lh/globals.css'` AFTER the existing Editorial tokens import so `.lh-*` wins where both apply.
- [ ] **Step 4:** `pnpm -F web build` — expect success.
- [ ] **Step 5:** Commit: `feat(web): add labelhub-ui-demo design tokens + Arco for re-skin`.

### Task 0.2: New layout shell (role-aware TopNav + SideNav)

**Files:**
- Create: `apps/web/src/shared/layout/LhTopNav.tsx` (port ui-demo `components/TopNav.tsx`)
- Create: `apps/web/src/shared/layout/LhSideNav.tsx` (port ui-demo `components/SideNav.tsx`, but items filtered by role)
- Modify: `apps/web/src/shared/layout/AppLayout.tsx` (replace Semi Layout with `.lh-shell` + LhTopNav + LhSideNav, keep `<Outlet/>` + logout)

- [ ] **Step 1:** Port `TopNav.tsx`/`SideNav.tsx` markup + `.lh-topnav`/`.lh-shell` classes. SideNav items come from a role→links map: labeler → [工作台]; reviewer → [AI 预审视图(同审核台)、人工审核]; owner/admin → [任务管理、模板设计、数据集、数据看板、导出中心]. Use existing `hasAnyRole()` from `client.ts` to filter.
- [ ] **Step 2:** Rewrite `AppLayout.tsx` to render `<div className="lh-root"><LhTopNav user={getCurrentUser()} .../><div className="lh-shell"><LhSideNav role.../><main className="lh-shell__main"><Outlet/></main></div></div>`. Keep the "切换角色" logout calling `clearToken()` → navigate `/auth/login`. Drop ui-demo `DemoNav` entirely.
- [ ] **Step 3:** Update `AppLayout.test.tsx` to assert role-filtered nav links render (labeler sees 工作台, not 任务管理).
- [ ] **Step 4:** `pnpm -F web test -- AppLayout && pnpm -F web build` — expect pass.
- [ ] **Step 5:** Commit: `feat(web): role-aware lh layout shell (TopNav + SideNav), drop demo nav`.

---

## Phase 1: Labeler workbench (deploy target #1)

### Task 1.1: Re-skin SchemaRenderer widgets to ui-demo look

**Files:**
- Modify: `apps/web/src/renderer/widgets/styles.ts` (or per-widget files) — apply `.lh-` look: Radio/Tags as button-pills, LLMTrigger as purple dashed box, ShowItem read-only gray block, Input with char counter styling.
- Test: `apps/web/src/renderer/SchemaRenderer.test.tsx` stays green (behavior unchanged).

- [ ] **Step 1:** Map each widget to ui-demo's visual treatment (see ui-demo `Workbench.tsx` field cards + `designer.css`): Radio → `.lh-` segmented buttons; Tags → multi-select chips; LLMTrigger → purple box with 生成/采纳; ShowItem → `lh-bg-elev` mono block. Change classNames/inline styles ONLY — do not touch widget logic, props, or onChange contracts.
- [ ] **Step 2:** Run `pnpm -F web test -- SchemaRenderer renderer/` — expect all pass (logic unchanged). Fix only assertions that checked old class strings.
- [ ] **Step 3:** Commit: `feat(web): restyle schema widgets to lh look`.

### Task 1.2: Rebuild Plaza presentation as ui-demo Workbench

**Files:**
- Modify: `apps/web/src/modules/labeler/Plaza.tsx` (replace JSX/markup with ui-demo `Workbench.tsx` 3-column layout; keep ALL existing state/hooks/handlers)
- Create: `apps/web/src/modules/labeler/Plaza.css` (port ui-demo `workbench.css`)
- Test: `apps/web/src/modules/labeler/Plaza.test.tsx` (update markup assertions; keep behavior assertions: claim, autosave, submit)

- [ ] **Step 1:** Lay out 3 columns matching ui-demo Workbench: left = task/item nav (drive from existing task list + my submissions state), center = heading/meta + warning banner (reject reason from auditLogs) + `<SchemaRenderer>` (the real one) + footer (保存草稿/提交本题 buttons wired to existing `saveDraft`/`submit`), right = 我的贡献 stats + 本题历史 (from auditLogs) + 快捷键. Reuse the existing autosave indicator text.
- [ ] **Step 2:** Wire the left nav clicks to existing claim/open handlers; ensure the center renders `SchemaRenderer` with `schema`, `payload`, `value`, `onChange`, `errors`, `runtime` exactly as today.
- [ ] **Step 3:** `pnpm -F web test -- Plaza` — update markup-based assertions, keep flow assertions passing.
- [ ] **Step 4:** `pnpm -F web build && pnpm -F web lint` — expect pass.
- [ ] **Step 5:** Commit: `feat(web): rebuild labeler workbench with lh layout`.

### Task 1.3: Phase 1 browser smoke + deploy

- [ ] **Step 1:** Ask user to run `make web` locally (do NOT spawn background dev server) OR deploy first then smoke against the server. Verify Labeler: login labeler1 → claim → answer real schema → autosave → submit, at 1280 + 1920. No console errors.
- [ ] **Step 2:** Deploy: rsync apps/web → server; `ssh ... 'cd ~/labelhub && docker compose --env-file deploy/.env -f deploy/docker-compose.prod.yml up -d --build web'`; external check `curl -f http://43.155.210.70/` + manual labeler login.
- [ ] **Step 3:** Append CHANGELOG entry; commit.

---

## Phase 2: Reviewer (deploy target #2)

### Task 2.1: Rebuild Queue presentation as ui-demo HumanReview (+AiReview visuals)

**Files:**
- Modify: `apps/web/src/modules/reviewer/Queue.tsx` (replace markup; keep all hooks/API)
- Create: `apps/web/src/modules/reviewer/Queue.css` (port ui-demo `humanreview.css` + relevant `aireview.css` pieces: ScoreBar, verdict box, audit timeline)
- Test: `apps/web/src/modules/reviewer/Queue.test.tsx` (update markup assertions; keep approve/reject/revise/batch/detail behavior)

- [ ] **Step 1:** Left = item list with filter tabs (AI建议通过/打回/转人工 — derive from submission.aiVerdict/status) + batch checkbox bar wired to existing batch review. Center = detail: before/after comparison (use existing revisions; if only one revision, show single), AI re-run box (ScoreBar per dimension from `aiReview.scores`, verdict text), 审核意见 textarea + quick tags (append to reason), 3 decision cards wired to existing approve/reject/revise. Right = stats (today reviewed/pass-rate from existing data or hide if unavailable) + 审核时间线 from auditLogs.
- [ ] **Step 2:** Keep `SchemaRenderer readOnly` for showing the labeler answer where ui-demo shows hardcoded fields. Keep AI retry ("转人工"/失败重跑) wired to existing retry endpoint.
- [ ] **Step 3:** `pnpm -F web test -- Queue` — update assertions, keep flow tests green.
- [ ] **Step 4:** `pnpm -F web build && lint` — pass.
- [ ] **Step 5:** Commit: `feat(web): rebuild reviewer console with lh layout`.

### Task 2.2: Phase 2 browser smoke + deploy
- [ ] Verify reviewer1: queue → detail (AI verdict + scores + timeline) → approve/reject/revise + batch, at 1280/1920. Deploy web. CHANGELOG. Commit.

---

## Phase 3: Owner dashboard + Designer (deploy target #3)

### Task 3.1: Rebuild Owner Dashboard as ui-demo Tasks

**Files:**
- Modify: `apps/web/src/modules/owner/Dashboard.tsx`
- Modify: `apps/web/src/modules/owner/ExportPanel.tsx` (restyle to ui-demo exports look)
- Create: `apps/web/src/modules/owner/Dashboard.css` (port ui-demo `tasks.css`)
- Test: existing owner tests stay green

- [ ] **Step 1:** Top = 4 stat cards (publishing/draft/this-week/pending — derive from task list + stats) acting as filters; table of tasks (status pill, strategy, progress) from existing task list; right publish drawer reused for task/template/AI-prompt config (wire to existing template + ai-prompt + golden + dry-run handlers). Keep AI Prompt / Golden Sample / dry-run panels, restyled inside the drawer or a detail panel.
- [ ] **Step 2:** Keep StatsBoard (VChart, lazy) and ExportPanel logic; restyle their containers to `.lh-` cards.
- [ ] **Step 3:** `pnpm -F web test -- Dashboard ExportPanel` — keep green. `build && lint`.
- [ ] **Step 4:** Commit: `feat(web): rebuild owner dashboard with lh layout`.

### Task 3.2: Rebuild Template Designer as ui-demo Designer

**Files:**
- Modify: `apps/web/src/modules/template/Designer.tsx`
- Modify: `apps/web/src/modules/template/Designer.css` (merge ui-demo `designer.css` palette/canvas/inspector look)
- Test: `Designer.integration.test.tsx` + `Designer.test.tsx` stay green (round-trip + responsive)

- [ ] **Step 1:** 3-column palette (materials + layouts) / canvas (field cards with ↑↓⎘✕, tab bar) / inspector (基础/校验/联动 tabs) matching ui-demo, but bound to the EXISTING draft-schema state, drag reorder, widget config, and save handlers. Keep `NestedCanvasPreview` behavior.
- [ ] **Step 2:** `pnpm -F web test -- Designer` — keep round-trip + responsive assertions green; update class-name assertions.
- [ ] **Step 3:** `build && lint`. Commit: `feat(web): rebuild template designer with lh layout`.

### Task 3.3: Owner admin pages (datasets/stats/exports) + Phase 3 deploy
- [ ] **Step 1:** Stats route → reuse StatsBoard full page. Exports route → reuse ExportPanel full page. Datasets → DECIDE: render real per-task item counts as the "dataset" list, OR mark visual-only with a note (log decision in CHANGELOG). Style per ui-demo `OwnerSection`.
- [ ] **Step 2:** Browser smoke owner1: tasks → designer → AI prompt/golden/dry-run → stats → export download, at 1280/1920. Deploy web. CHANGELOG. Commit.

---

## Phase 4: Cleanup
- [ ] **Step 1:** Remove Semi Design dep + old Editorial `tokens.css`/StyleGuide if fully unused (`grep` for `@douyinfe/semi`); update `/style-guide` to the lh system or remove the route.
- [ ] **Step 2:** `pnpm -F web build` — check bundle size (landing JS budget); confirm no Semi in bundle.
- [ ] **Step 3:** Full browser smoke all 3 roles. Final deploy. Update `submission/DEMO_ENV.md` (note new UI), CHANGELOG, README screenshots flagged for re-capture.
- [ ] **Step 4:** Commit: `chore(web): drop Semi, finalize lh re-skin`.

---

## Acceptance criteria (definition of done)
- Deployed http://43.155.210.70 visually matches labelhub-ui-demo (Arco `.lh-` look, the per-page layouts) across all 3 roles at 1280 + 1920.
- All 3 role flows still work end-to-end against the live backend + real doubao: labeler claim→answer(real schema)→autosave→submit; reviewer queue→AI verdict→approve/reject/revise→batch; owner tasks→designer→AI prompt/golden/dry-run→stats→multi-format export download.
- `pnpm -F web build` + `lint` + existing test suites pass (assertions updated for new markup, behavior coverage preserved).
- No regression in auth/role gating; no console errors beyond known Router future-flag warnings.

## Self-review notes
- Spec coverage: every ui-demo page maps to a task (1.2 Workbench, 2.1 HumanReview+AiReview, 3.1 Tasks, 3.2 Designer, 3.3 OwnerSection). ✓
- The one underspecified item (Datasets backend concept) is explicitly deferred to a logged decision in Task 3.3, not left as a silent placeholder.
- Logic reuse is explicit per task ("keep ALL existing state/hooks/handlers") so no data-layer is re-implemented.

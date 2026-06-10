# UI Re-skin Design Spec (labelhub-ui-demo look on real backend)

> Brainstormed & approved 2026-05-29. Implementation plan: `PLAN-UI-RESKIN-IMPL.md`.

## Problem
The deployed demo (http://43.155.210.70) currently uses `apps/web` (Semi Design). The challenge organizer's reference demo `~/labelhub-ui-demo` defines the expected **layout + visual** look (Arco-style `.lh-` design system). We want the deployed app to match that look while keeping our real backend (Go API + worker + doubao) and real data.

## Decisions (locked)
0. **Tech stack (LOCKED, per organizer recommendation):** React 18 + TS + **Semi Design (kept — organizer's own lib, already wired; NO Arco, do NOT remove Semi)** + self-built **Schema renderer** for forms (satisfies the recommended "Schema 渲染" paradigm; not migrating to Formily) + context/Zustand for state + native HTML5 drag today (optionally `@dnd-kit/core` at Phase 3). ui-demo's look = ported `.lh-` CSS on top of Semi components. (ui-demo itself uses Arco; we deliberately keep Semi.)
1. **Re-skin `apps/web`, don't rebuild the data layer in ui-demo.** Reuse the working engine: API client, auth, role routing, SchemaRenderer (11 widgets + parser + validator), and all 3-role flows. Only the presentation (the `.lh-` CSS design system, layout shells, per-page markup/classNames on existing Semi components) is ported from ui-demo.
2. **Visual/layout ≈ ui-demo; content = real.** ui-demo's screenshot data is the organizer's fake demo data and is irrelevant. Our pages render real fields via SchemaRenderer driven by our actual task templates (`qa_quality`, `preference_compare`) and real API responses.
3. **Keep role-based login** (owner1/labeler1/reviewer1, pwd 123456). Drop ui-demo's open flat demo nav; use a role-filtered SideNav.
4. **Decorative data positions:** compute real metrics where a backend source exists (task progress, pass-rate, pending count, AI-vs-human, dimension averages). **Remove** positions with no real source (live throughput/s, SLA countdown). No fabricated numbers.
5. **Load real demo content:** import the official datasets (`~/Downloads/datasets.zip`) into the live tasks — 30 `qa_quality` items + 12 `preference_compare` items — and pre-generate a small set of real submissions across states (submitted / AI-reviewed / human-approved / revised) so reviewer queue, stats, and export pages are populated with genuine data.
6. **Target = live Seoul prod DB** (this is the judge-facing demo).
7. **Sequencing:** Phase 0.5 data import → Phase 0 design foundation → Phase 1 Labeler → Phase 2 Reviewer → Phase 3 Owner+Designer → Phase 4 cleanup. Deploy + user-verify after each role phase.

## Dataset shapes (for import + template mapping)
- **qa_quality** item payload keys: `id, category, difficulty, lang, media_type, media_url, content_markdown, prompt, model_answer, reference, tags[], source, expected_dimensions[]` (30 items).
- **preference_compare** item payload keys: `id, task_type, lang, prompt, response_a, model_a, response_b, model_b, preferred, margin, dimensions[], safety_flag, annotator_note` (12 items).
- `reference` / `preferred` / `annotator_note` are organizer ground-truth — usable as golden-sample expected answers, not shown to labelers.

## Acceptance criteria
- http://43.155.210.70 visually ≈ labelhub-ui-demo across all 3 roles at 1280 + 1920.
- All 3 role flows work end-to-end on the live backend + real doubao + the imported real data.
- `pnpm -F web build` + `lint` + existing tests pass (markup assertions updated, behavior coverage preserved).
- Auth/role gating intact; no console errors beyond known Router future-flag warnings.

## Out of scope
- Mobile layout, i18n, dark mode.
- ui-demo's standalone "AI Review live queue" page as a real feature (its visuals fold into reviewer detail; queue ops stay in asynqmon).
- A real "Datasets" CRUD (Phase 3 maps it to per-task item counts or marks visual-only — decision logged then).

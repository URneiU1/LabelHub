# S6 Acceptance

Date: 2026-05-28

## Scope

S6 covers Editorial polish and bonus materials only — no new backend business flow. Goal: turn the working labeling system into a demo-ready, high-density, status-clear console, and finish the Tabs/Group materials deferred from S2. See `docs/PLAN-S6-IMPL.md`.

## Checklist

- [x] **Day1 — Editorial tokens + shared primitives.** `tokens.css` converted from Schematic/Inter/default-blue to Editorial console tokens (no Google Font dependency), with stable 7-state status colors, Semi control overrides, global `:focus-visible` ring, and skeleton/top-progress keyframes. Shared `StatusBadge` / `EmptyState` / `LoadingBlock` / `TopProgress` added with `StatePrimitives` tests.
- [x] **Day2 — Dense status surfaces.** Owner Dashboard, ExportPanel, Labeler Plaza, and Reviewer Queue reuse `StatusBadge` / `EmptyState` / `LoadingBlock` for task lists, export history, AI-review toggle, golden samples, dry-run history, and empty/loading states instead of raw status/loading text.
- [x] **Day3 — Runtime Tabs interaction.** `SchemaRenderer` Tabs render a real `tablist` / `tabpanel`: only the active tab's fields render, the flat answer object is preserved across switches, and tests cover switch / hide / value-persist.
- [x] **Day3 — Designer responsive layout + nested canvas.** `Designer.css` grid: 1920 three columns (palette / canvas / property), ≤1599 two columns with the property panel wrapped full-width below the canvas, ≤768 single column. `NestedCanvasPreview` shows Group/Tabs child fields as mini rows; integration test asserts the nested preview renders Group children and Tabs per-tab children.
- [x] **Day4 — Friendly error copy.** `client.ts` throws a backwards-compatible `ApiError` carrying `code` / `requestId`; `INVALID_STATE` and `LLM_PROVIDER_ERROR` surface Chinese action copy through any existing `err.message` display, other codes keep the backend message. `client.test.ts` covers the mapping and the empty-message fallback.
- [x] **Day4 — A11y.** Designer nested controls already carry `aria-label` (verified across `NestedFieldsEditor` / `GroupControls` / `TabsControls`); global `:focus-visible` ring shipped in Day1.
- [x] **Day4 — Keyboard shortcut.** Labeler `Ctrl/Cmd+Enter` submits the active answer (with a button `title` hint and a Plaza test).
- [ ] **Day5 — Browser smoke (PENDING).** Requires a locally-run dev server (`make web`); not yet executed. See "Pending Browser Verification" below.

## Deliberately Cut / Skipped

- Reviewer single-key `A/R/B` approve/revise/reject shortcuts — plan-optional; skipped because single keypresses triggering irreversible review actions are too easy to fire accidentally.
- Dark mode, Framer Motion, full mobile experience, TanStack Query / axios migration, Formily / dnd-kit migration, new backend APIs — explicit S6 non-goals in `docs/PLAN-S6-IMPL.md`.
- Designer canvas-level nested drag inside Tabs/Group remains Up/Down reorder only (drag was the optional, test-destabilizing path).

## Pending Browser Verification (Day5 Task 5.1)

Run locally, then verify:

```bash
cd ~/Desktop/LabelHub && make web   # http://localhost:5173
```

Routes: `/auth/login`, `/owner`, `/owner/tasks/:taskId/templates/:templateId`, `/labeler`, `/reviewer`, `/style-guide`.
Viewports: 1280×800, 1920×1080, 390×844 (smoke only).
Checks: no console error; no text overlap; Designer usable at 1280 and 1920 (property panel wraps below at 1280, three columns at 1920); status colors consistent; renderer Tabs switch works.

Automated tests cannot verify the responsive breakpoints — jsdom does not lay out CSS media queries — so the 1280/1920 layout claims rest on this browser pass.

## Verification Commands (already run during S6)

```bash
pnpm -F web test    # 14 files, 114 tests pass (jsdom canvas warning is pre-existing)
pnpm -F web lint
pnpm -F web build   # pre-existing StatsBoard chunk >500KB warning only
git diff --check
```

Backend was untouched in S6; the S5 Go gates remain the latest backend verification (see `docs/S5_ACCEPTANCE.md`).

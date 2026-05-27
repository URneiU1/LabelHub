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
- [x] **Day5 — Browser smoke (done 2026-05-28).** Full local stack run (API on :8090 to avoid an :8080 conflict, web proxied to it); login/owner/designer/reviewer/style-guide checked via chrome-devtools at 1280 and 1920. Designer responsive confirmed (1280: palette+canvas with property panel full-width below; 1920: three columns side-by-side), StatusBadge/Editorial tokens consistent, no console errors (only pre-existing React Router v7 future-flag warnings). One inconsistency found and fixed: `/style-guide` still carried the old Schematic/Inter/#0f62fe identity and hand-rolled status tags — now Editorial-aligned (palette reads live CSS vars, real `StatusBadge`, updated title/footer).

## Deliberately Cut / Skipped

- Reviewer single-key `A/R/B` approve/revise/reject shortcuts — plan-optional; skipped because single keypresses triggering irreversible review actions are too easy to fire accidentally.
- Dark mode, Framer Motion, full mobile experience, TanStack Query / axios migration, Formily / dnd-kit migration, new backend APIs — explicit S6 non-goals in `docs/PLAN-S6-IMPL.md`.
- Designer canvas-level nested drag inside Tabs/Group remains Up/Down reorder only (drag was the optional, test-destabilizing path).

## Browser Verification (Day5 Task 5.1 — done 2026-05-28)

Stack brought up locally (`:8080` was occupied by an unrelated FastAPI service, so the LabelHub API ran on `:8090` with the web proxy pointed there):

```bash
# terminal A — API on a free port
set -a; . ./.env; set +a; cd apps/api && API_PORT=8090 go run ./cmd/server
# terminal B — web proxied to it
VITE_API_PROXY_TARGET=http://localhost:8090 make web   # http://localhost:5173
```

Checked via chrome-devtools (login as owner1): `/auth/login`, `/owner`, `/owner/tasks/1/templates/1`, `/reviewer`, `/style-guide`.

| Check | Result |
|---|---|
| Designer @ 1920 | Three columns side-by-side (palette / canvas / property) ✓ |
| Designer @ 1280 | Palette + canvas two columns, property panel full-width below ✓ |
| StatusBadge / Editorial tokens | Consistent on login, owner, reviewer (e.g. green `已发布`, queue tones) ✓ |
| Console errors | None — only pre-existing React Router v7 future-flag warnings ✓ |
| Text overlap | None observed ✓ |
| `/style-guide` consistency | Was stale (Schematic/Inter/#0f62fe + hand-rolled tags) → fixed to Editorial (live CSS-var palette, real `StatusBadge`, updated title/footer) ✓ |

jsdom cannot lay out CSS media queries, so the 1280/1920 Designer layout claims rested on this browser pass; now confirmed. 390×844 narrow smoke was not separately captured (single-column stack rule is CSS-only at ≤768).

## Verification Commands (already run during S6)

```bash
pnpm -F web test    # 14 files, 114 tests pass (jsdom canvas warning is pre-existing)
pnpm -F web lint
pnpm -F web build   # pre-existing StatsBoard chunk >500KB warning only
git diff --check
```

Backend was untouched in S6; the S5 Go gates remain the latest backend verification (see `docs/S5_ACCEPTANCE.md`).

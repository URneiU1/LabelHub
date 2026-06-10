# Technical Challenges Hardening Implementation Plan

> For agentic workers: execute task-by-task from P0 to P5. Keep every change small, tested, and independently revertible.

## Goal

Close the remaining gaps from the technical-challenge audit so LabelHub can credibly present:

- A robust dynamic form system: Designer/Renderer separation, frozen schema versions, conditional visibility, and backend-enforced runtime validation.
- A long, auditable state flow: task/item/submission/review transitions with transaction safety, idempotency, concurrency guards, and audit logs.
- A production-shaped AI Agent: configurable prompts, structured output, failover to human review, explainable results, and basic score-stability evidence.
- Engineering and UX hardening: type-safe API usage, large-form performance, online draft safety, offline draft preservation, and viewport proof for 1280/1920 plus mobile bonus.

## Current Snapshot

| Area | Current state | Gap to close |
|---|---|---|
| Designer / Renderer decoupling | Done. Designer builds JSON schema, Renderer consumes schema at runtime. | Keep as-is; add regression tests when touching schema logic. |
| Drag-and-drop stack | dnd-kit is in use. | No gap. |
| Formily | Not used. Current implementation is custom renderer/parser. | Do not migrate before demo unless rubric explicitly requires Formily. Write an ADR documenting the decision. |
| Schema versioning | Done. Submissions bind `template_version`. | Add tests around validation using frozen version when adding validator parity. |
| Field linkage | Frontend supports visibility and runtime validation. Backend handles required visibility basics. | Backend answer-level parity for min/max/regex/customRule is incomplete. |
| State machine | Mostly strong. AI no longer auto-approves. | Preserve invariant with tests; document sampling auto-approve as separate from AI auto-approve. |
| Transactions / concurrency / audit | Mostly strong. Locks, RowsAffected guards, outbox, and audit logs exist on key paths. | Add focused tests only where new work touches these paths. |
| AI Agent | Prompt config, structured output, traceable review rows, failover, and UI verdict display exist. | Score-stability evidence is still light. |
| Large-form performance | Basic React memoization and backend pagination exist. | No virtualized/capped rendering strategy for very large schemas or queues. |
| Drafts | Online autosave exists. | Offline draft preservation and recovery are missing. |
| Mobile | Responsive CSS exists. | Needs mobile smoke and targeted overflow fixes. |

## Non-Goals

- Do not replace the custom renderer with Formily inside the hardening sprint.
- Do not rewrite the state machine or review service.
- Do not add a broad rule-engine abstraction unless backend validation parity cannot be completed with the current schema surface.
- Do not add offline final-submit queueing in the first pass. Preserve drafts offline; require online for final submit.

## P0 - Backend Runtime Validation Parity

**Outcome:** A submission that passes frontend validation cannot bypass equivalent backend validation for visible answer fields.

### Backend Validation Parity Matrix (P0.1 decision)

Frontend source of truth: `apps/web/src/renderer/validator.ts`. Backend runtime: `apps/api/internal/service/submission/validate.go`. Template-save guard: `apps/api/internal/handler/template_validate.go`.

| Rule | Frontend (validator.ts) | Backend runtime (validate.go) | JSON field / shape |
|---|---|---|---|
| `required` / `requiredWhen` | empty visible field → error | enforced (`ErrIncompleteAnswer`) | `required: bool`, `requiredWhen: {field, equals?, notEmpty?}` |
| `visibleWhen` gating | hidden field skipped | hidden field **and hidden container** skipped | `visibleWhen: {field, equals?, notEmpty?}` |
| `minLength` | `value.trim().length < n` (strings) | same, counted in UTF-16 code units | `minLength: number` |
| `maxLength` | `value.length > n` (strings, no trim) | same, UTF-16 code units | `maxLength: number` |
| `regex` | `new RegExp(r).test(value)` (non-empty strings) | Go `regexp` (RE2) on non-empty strings | `regex: string` |
| `customRule` | `expr-eval` over `{value, answer, ...siblings, len}` | Go `customrule` evaluator over the supported subset | `customRule: {expr, message}` |

**customRule subset decision:** the Designer emits free-text `expr-eval` expressions, so the backend ships a parity evaluator (`apps/api/internal/customrule`) covering the realistically-authored subset — literals, scope vars (`value`, `answer`, sibling names), `answer.field` member access, `len(x)`, unary `- not !`, arithmetic `+ - * / %`, comparison `== != < <= > >=`, logical `and` / `or`, and ternary `? :`. Anything outside it (other functions, `^`, `in`, `&&`, `||`, deep member chains) is **rejected at template save** so a saved rule is always runtime-enforceable (no silent under-enforcement). RE2 vs JS-RegExp differences (lookahead/backreferences) are the only known non-parity edge and are not used by seed templates.

### Task P0.1 - Inventory Current Schema Rule Surface

Files:

- `apps/web/src/renderer/validator.ts`
- `apps/web/src/renderer/parser.ts`
- `apps/api/internal/handler/template_validate.go`
- `apps/api/internal/service/submission/validate.go`
- Seed schemas under `tools/seed` or `apps/api/cmd/seed`

Steps:

- [ ] List every runtime rule currently emitted or accepted by Designer: `required`, `requiredWhen`, `visibleWhen`, `minLength`, `maxLength`, `regex`, `customRule`, numeric bounds if present.
- [ ] Confirm exact JSON field names and value types from parser tests and seed templates.
- [ ] Decide the backend-supported `customRule` subset. Prefer only the subset Designer can generate today.
- [ ] Add a short table to this plan or a dedicated test comment explaining supported vs rejected rule syntax.

Verification:

- [ ] No production logic changes in this task.
- [ ] A reviewer can point from each frontend validation rule to a backend handling decision.

### Task P0.2 - Add Backend Answer Value Validator

Files:

- `apps/api/internal/service/submission/validate.go`
- `apps/api/internal/service/submission/validate_test.go`

Steps:

- [ ] Keep existing frozen-template lookup by `template_version`.
- [ ] Recurse through Group/Tabs fields exactly once and skip fields hidden by `visibleWhen`.
- [ ] Keep hidden answer pruning behavior separate from validation behavior.
- [ ] Enforce `required` and `requiredWhen` only for visible fields.
- [ ] Enforce string length rules for visible string fields.
- [ ] Enforce regex rules with safe compilation and clear validation errors.
- [ ] Enforce numeric bounds only if the current schema supports numeric widgets or numeric validation.
- [ ] Enforce the approved `customRule` subset. If a rule is unsupported, fail template save rather than accepting a rule the backend cannot enforce.
- [ ] Return user-facing validation errors that name the field label or field key.

Tests:

- [ ] Hidden Group with required child does not block submit.
- [ ] Hidden field value does not affect validation.
- [ ] Visible required child blocks submit.
- [ ] `minLength` and `maxLength` reject invalid values.
- [ ] `regex` rejects invalid values and accepts valid values.
- [ ] `customRule` rejects invalid values for the supported subset.
- [ ] Existing submission validates against its frozen template version, not the latest template.

Verification:

```bash
go test ./apps/api/internal/service/submission -count=1
go test ./apps/api/internal/handler -run 'RespondItem|Submission|Template' -count=1
```

### Task P0.3 - Align Template Save Validation With Runtime Validator

Files:

- `apps/api/internal/handler/template_validate.go`
- `apps/api/internal/handler/template_validate_test.go`

Steps:

- [ ] Reject any template rule that the backend runtime validator cannot enforce.
- [ ] Keep error copy specific: unsupported rule, bad regex, bad dependency key, or reserved field key.
- [ ] Add tests for unsupported `customRule` and malformed regex.

Verification:

```bash
go test ./apps/api/internal/handler -run TemplateValidate -count=1
```

## P1 - AI Score Stability And Explainability Evidence

**Outcome:** Owner can show that AI review is not a black box: prompts are versioned, outputs are structured, failures fail over, and dry-run gives basic stability metrics.

### Task P1.1 - Define Stability Metrics

Use the existing dry-run/golden-sample flow. Add metrics in `ai_dry_runs.result` first; avoid a migration unless the UI needs queryable columns.

Metrics:

- `verdict_agreement`: highest verdict count divided by total repeated runs.
- `score_stddev`: standard deviation of overall score across repeated runs.
- `expected_match_rate`: percentage matching the golden expected verdict.
- `error_rate`: failed attempts divided by total attempts.
- `dimension_stddev`: optional per-dimension score standard deviation.

Steps:

- [ ] Decide default repeat count: `1` for normal smoke, `3` for stability dry-run.
- [ ] Cap repeat count with an env or server-side constant to prevent accidental LLM cost spikes.
- [ ] Keep mock provider deterministic so tests remain stable.

### Task P1.2 - Implement Repeated Dry-Run Evaluation

Files:

- `apps/api/internal/handler/ai_dry_run.go`
- `apps/ai-worker/cmd/worker/ai_dry_run.go`
- `pkg/llmreview`
- Existing dry-run tests

Steps:

- [ ] Extend dry-run request payload with optional `repeat_count`.
- [ ] Run each golden sample `repeat_count` times in the worker.
- [ ] Aggregate the metrics listed above.
- [ ] Preserve existing single-run result shape for compatibility.
- [ ] Mark provider/model/temperature/prompt_config_id in the result payload for traceability.
- [ ] Fail the dry-run only when all attempts fail; otherwise expose partial errors in result details.

Tests:

- [ ] Mock provider repeated run returns `verdict_agreement=1`.
- [ ] A fake flaky provider records non-zero `error_rate` without losing successful attempts.
- [ ] Disallowed model still surfaces a clear provider failure and does not look like a scoring mismatch.

Verification:

```bash
go test ./apps/api/internal/handler ./apps/ai-worker/cmd/worker ./pkg/llmreview -run 'DryRun|Golden|AllowedModel' -count=1
```

### Task P1.3 - Surface Stability In Owner AI UI

Files:

- `apps/web/src/modules/owner` AI prompt/dry-run components
- `apps/web/src/shared/api/schema.d.ts` if OpenAPI changes
- `docs/openapi.yaml` if request/response changes

Steps:

- [x] Add a repeat-count control with conservative default.
- [x] Show agreement, expected match rate, score stddev, and error rate.
- [x] Keep the current per-sample verdict table.
- [x] Add copy that explains low agreement as "needs prompt adjustment or manual review", not as a system failure.

Verification:

```bash
pnpm -F web gen:api
pnpm -F web test -- owner
pnpm -F web build
```

## P2 - Large Form Performance

**Outcome:** Large templates stay usable, and performance claims are backed by a repeatable smoke.

### Task P2.1 - Add A Large-Schema Performance Fixture

Files:

- `apps/web/src/renderer/SchemaRenderer.test.tsx`
- `apps/web/src/modules/template/Designer.integration.test.tsx`
- Optional: `tools/large_schema_fixture.*`

Steps:

- [ ] Generate a deterministic schema with 250 to 500 fields, including Group, Tabs, and visibleWhen rules.
- [ ] Test that Renderer only submits visible fields and does not throw.
- [ ] Test that Designer can load and save the schema without corrupting field order or keys.
- [ ] Keep timing assertions out of jsdom unless they are stable locally.

Verification:

```bash
pnpm -F web test -- SchemaRenderer Designer
```

### Task P2.2 - Optimize Renderer And Designer Hot Paths

Files:

- `apps/web/src/renderer/SchemaRenderer.tsx`
- `apps/web/src/modules/template/Designer.tsx`
- Related CSS files

Steps:

- [ ] Extract render helpers only where it reduces rerender pressure.
- [ ] Memoize field rendering with stable props where practical.
- [ ] Skip hidden subtrees before rendering children.
- [ ] Keep Tabs rendering to the active tab panel while preserving flat answer state.
- [ ] Avoid inline object churn in tight field loops when it causes measurable rerenders.
- [ ] If 250+ row lists remain slow, add a small virtualization strategy for list-like surfaces only. Do not virtualize complex form containers until measured.

Verification:

- [ ] Browser smoke at 1280x800 and 1920x1080 with the large schema.
- [ ] No text overlap, broken drag state, or lost answers.
- [ ] `pnpm -F web build` passes.

## P3 - Offline Draft Preservation

**Outcome:** Labelers do not lose work when the network drops. Online submit remains the source of truth.

### Task P3.1 - Add Local Draft Store

Files:

- `apps/web/src/modules/labeler/Plaza.tsx`
- New focused helper, for example `apps/web/src/modules/labeler/offlineDraftStore.ts`
- `apps/web/src/modules/labeler/Plaza.test.tsx`

Steps:

- [ ] Store answer drafts locally by `taskId:itemId:submissionId:revisionNo`.
- [ ] Use IndexedDB if answer size can exceed localStorage comfort; otherwise keep a very small localStorage helper and document the limit.
- [ ] Mark each local draft with `updatedAt`, `templateVersion`, and `synced` status.
- [ ] On API autosave success, mark the local draft synced.
- [ ] On network failure, keep local draft and show a non-blocking "local draft saved" state.
- [ ] On page load, offer to restore newer local draft if it is newer than the server draft.
- [ ] Keep final submit online-only in this phase with a friendly message when offline.

Tests:

- [ ] Autosave success marks local draft synced.
- [ ] Autosave network failure preserves local draft.
- [ ] Reload restores newer local draft.
- [ ] Submit while offline is blocked with clear copy and does not delete local draft.

Verification:

```bash
pnpm -F web test -- Plaza
pnpm -F web build
```

### Task P3.2 - Add UX Recovery States

Steps:

- [ ] Add a compact connection/draft state indicator near the answer toolbar.
- [ ] Add a "discard local draft" action so recovery is reversible.
- [ ] Do not show modal noise during normal online autosave.

Verification:

- [ ] Manual browser smoke: type answer, simulate network failure, reload, recover draft, reconnect, autosave succeeds.

## P4 - Mobile And Viewport Proof

**Outcome:** The app remains coherent at required desktop sizes and has credible mobile bonus coverage.

### Task P4.1 - Desktop Regression Smoke

Viewports:

- 1280x800
- 1920x1080

Pages:

- Owner task list and export config
- Designer
- Labeler answer page
- Reviewer detail

Checks:

- [ ] No overlapping controls or clipped button text.
- [ ] Designer has usable palette/canvas/property controls.
- [ ] Labeler can answer and submit.
- [ ] Reviewer can inspect AI verdict and send revision.

### Task P4.2 - Mobile Bonus Smoke

Viewports:

- 390x844
- 768x1024

Pages:

- Labeler task plaza
- Labeler answer page
- Owner task list

Steps:

- [ ] Fix horizontal overflow.
- [ ] Stack dense panels into scan-friendly sections.
- [ ] Keep primary actions sticky or easy to reach where existing design allows.
- [ ] Document reviewer/Designer mobile limits if they remain desktop-first.

Verification:

- [ ] Save screenshots under `submission/assets/screenshots/` only after the UI is final.
- [ ] Update `submission/assets/README.md` if screenshot filenames change.

## P5 - Form Engine Decision Record

**Outcome:** The project can defend not using Formily, or has a separate migration decision if the rubric demands it.

Files:

- New `docs/ADR-FORM-ENGINE.md`
- Optional update to `docs/ARCHITECTURE.md`

Steps:

- [ ] Document current choice: custom schema renderer + dnd-kit + frozen template versions.
- [ ] Explain why Formily is not being migrated before demo: existing renderer already supports task payload rendering, AI widgets, schema freezing, export mapping, and conditional validation.
- [ ] State what would trigger a Formily migration: explicit judge requirement, many new widgets, or need for Formily ecosystem validators.
- [ ] If migration is required later, plan an adapter layer instead of rewriting Owner/Labeler flows at once.

Verification:

- [ ] Architecture docs and README do not claim Formily is used unless it actually is.

## Global Verification Gate

Run after P0/P1 code changes:

```bash
go test ./apps/api/... ./apps/ai-worker/... ./pkg/exporter ./pkg/llmreview ./pkg/reviewsampling -count=1
```

Run after frontend P2/P3/P4 changes:

```bash
pnpm -F web test
pnpm -F web build
```

Always run before final handoff:

```bash
git diff --check
git status --short
```

Smoke when local services are available:

```bash
python3 tools/smoke_ai_local.py
```

## Suggested Commit Order

1. `fix(api): enforce runtime validation rules on submitted answers`
2. `test(api): cover frozen-template validation parity`
3. `feat(ai): add dry-run stability metrics`
4. `feat(web): display AI dry-run stability metrics`
5. `test(web): add large-schema renderer and designer coverage`
6. `perf(web): reduce large dynamic form rerenders`
7. `feat(web): preserve labeler drafts during network loss`
8. `fix(web): harden responsive layouts for desktop and mobile smoke`
9. `docs: record form engine decision`

## Cut Line If Time Runs Short

Must finish:

- P0 backend runtime validation parity.
- P1 AI stability metrics at least in backend dry-run result.
- P4 desktop 1280/1920 smoke.

Good to finish:

- P3 offline draft preservation.
- P2 large-schema performance optimizations after measurement.
- P5 ADR.

Optional:

- Mobile polish beyond Labeler pages.
- Full Formily migration spike.

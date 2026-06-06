# ADR: Form Engine — Custom Schema Renderer vs Formily

- **Status:** Accepted
- **Date:** 2026-06-07
- **Deciders:** LabelHub maintainer (`@URneiU1`)
- **Related plan:** `docs/PLAN-TECH-CHALLENGES-IMPL.md` (P5 — Form Engine Decision Record)

## Context

LabelHub's dynamic form is one of the three core capabilities the challenge
evaluates ("Schema 渲染" / schema-driven forms). The platform needs a
Designer that produces a JSON schema and a Renderer that consumes that schema
at runtime, with a clean Designer/Renderer split, frozen schema versions,
conditional visibility, and validation that the backend can also enforce.

[Formily](https://formilyjs.org/) is the canonical reference for this paradigm,
and the original `docs/PLAN.md` listed Formily 2 as the intended form core
(`x-component`, `x-reactions`, `x-validator`). During implementation we instead
built a small custom schema engine. This ADR records that decision, why we are
**not** migrating to Formily before the demo, what would justify a migration,
and how a migration would be staged if it becomes required.

The custom engine is already wired end-to-end:

- **Renderer.** `apps/web/src/renderer/SchemaRenderer.tsx` walks the schema,
  renders the widget registry, evaluates `visibleWhen` to skip hidden subtrees,
  drives Tabs/Group nesting, and prunes hidden answer values
  (`pruneHiddenAnswerValues`) so submitted answers only carry visible fields.
- **Parser.** `apps/web/src/renderer/parser.ts` validates raw template JSON into
  a typed `TemplateSchema` (`apps/web/src/renderer/types.ts`): widget enum,
  unique/reserved field names, `requiredWhen` / `visibleWhen` / `customRule`
  shapes, `minLength` / `maxLength` / `regex` / `options` / `maxFiles`, LLM
  trigger `target_field` references, and `x-*` passthrough.
- **Frontend validator.** `apps/web/src/renderer/validator.ts` enforces
  `required`, `requiredWhen`, length, `regex`, and `customRule` (the
  `customRule.expr` is evaluated with `expr-eval`, not `eval`), and skips
  fields hidden by `visibleWhen`.
- **Designer.** `apps/web/src/modules/template/Designer.tsx` builds the schema
  via drag-and-drop using **dnd-kit** (`@dnd-kit/core` + `@dnd-kit/sortable`,
  with `PointerSensor` and `KeyboardSensor` for keyboard accessibility) and
  shares the same `expr-eval` parser to give a live, non-blocking hint for
  `customRule` expressions. The Designer's "form preview" mode renders the
  in-progress schema through the very same `SchemaRenderer` the labeler uses,
  so what the Owner designs is exactly what the Labeler answers.
- **Schema freezing.** Submissions bind a `template_version`. The backend
  validator (`apps/api/internal/service/submission/validate.go`) looks the
  frozen template up by `(task_id, version)` and validates the answer against
  that frozen schema — never against the latest template.
- **Dependencies.** `apps/web/package.json` carries `@dnd-kit/*` and
  `expr-eval`; **Formily is not a dependency.**

## Decision

**Keep the custom JSON schema renderer + Designer (dnd-kit) + frozen template
versions. Do not migrate to Formily before the demo.**

The custom engine already covers every form requirement the challenge exercises,
and it does so with a schema surface the backend can mirror. The concrete reasons:

1. **It already renders the full task surface.** The widget registry covers the
   locked core set (`apps/web/src/renderer/types.ts`): `ShowItem`, `Group`,
   `Tabs`, `Input`, `TextArea`, `Radio`, `Tags`, `RichText`, `JSONEditor`,
   `FileUpload`, and `LLMTrigger`. `ShowItem` renders the upstream task payload
   (text/image/video/markdown/json), and `LLMTrigger` is the AI-assist widget
   that targets another field. Both are first-class in
   `SchemaRenderer.tsx` and `parser.ts`.

2. **Conditional logic and validation are done.** `visibleWhen` (visibility)
   and `requiredWhen` (conditional required) are parsed
   (`parser.ts: parseVisibleWhen` / `parseRequiredWhen`), rendered
   (`SchemaRenderer.tsx: visibleWhenMatches`), and validated
   (`validator.ts: visibleWhenMatches` / `requiredWhenMatches`). `customRule`
   uses a sandboxed `expr-eval` expression with `value`, `len(value)`, and
   `answer.<field>` in scope — no string `eval`.

3. **Schema freezing + export mapping already work against this schema.**
   Submissions bind `template_version`; `schema.export_fields` selects export
   columns. The Designer can export the schema JSON directly. None of this
   depends on Formily's runtime.

4. **The backend can enforce the same schema, which Formily would not give us
   for free.** A frontend-only form library does not protect the server. With
   the custom schema, the backend reuses the *same* JSON shape:
   - `apps/api/internal/service/submission/validate.go` enforces runtime
     **required / requiredWhen / visibility** parity: it loads the frozen
     template by `template_version`, recurses Group/Tabs exactly once, skips
     fields hidden by `visibleWhen`, treats `ShowItem` as value-less, and
     rejects an answer whose visible required (or `requiredWhen`-triggered)
     fields are empty (`ErrIncompleteAnswer`). `answerIsEmpty` and `jsonEquals`
     mirror the frontend's emptiness and cross-type `equals` semantics. This is
     covered by `validate_test.go` (hidden containers do not block submit;
     visible required children do; numeric `equals`; frozen-version lookup).
   - `apps/api/internal/handler/template_validate.go` rejects, **at template
     save time**, any rule the backend cannot stand behind: bad regex
     (`regexp.Compile`), malformed `customRule.expr` (balanced
     parens/quotes, no dangling operator), `minLength`/`maxLength` bounds and
     `min > max`, reserved/duplicate field names, unknown widgets, dangling
     `requiredWhen`/`visibleWhen`/`target_field` references, and size caps.

   Extending answer-value runtime parity to `minLength` / `maxLength` /
   `regex` / the approved `customRule` subset is exactly the work tracked under
   **P0 (Backend Runtime Validation Parity)** in
   `docs/PLAN-TECH-CHALLENGES-IMPL.md`. Adopting Formily would not advance that
   server-side work — the backend would still need its own Go validator over the
   schema — so a migration would add risk without removing the P0 task.

5. **Migration cost is high and the deadline is close.** Formily would replace
   the renderer, the Designer's drag model, the parser's typed schema, and the
   `expr-eval` rule layer that the backend already shadows, with no functional
   gain for the demo. The hardening plan explicitly lists "replace the custom
   renderer with Formily" as a **Non-Goal** for the sprint.

## Consequences

**Positive**

- One JSON schema is the single contract shared by Designer, Renderer,
  frontend validator, and backend validator — frontend and backend validate the
  same shape, which is what makes server-side bypass protection feasible.
- No heavy form-framework dependency; the bundle stays small and the runtime is
  easy to reason about and test (`parser` / `validator` / `SchemaRenderer` have
  focused unit tests).
- Designer and Renderer are decoupled and version-frozen, so historical
  submissions always validate against the schema they were authored under.

**Negative / trade-offs**

- We own the engine: every new widget, validator, or linkage operator is
  hand-written in both the TS layer and (where runtime-enforced) the Go layer.
- We do not get Formily's ecosystem (rich validators, `x-reactions` graph,
  third-party field components) for free.
- The `customRule` expression language is intentionally a small `expr-eval`
  subset; complex cross-field rules must stay within what both `expr-eval` and
  the backend's `customRule` checks can express.
- Answer-value backend parity for `minLength` / `maxLength` / `regex` /
  `customRule` is not complete yet (tracked under P0); until then those three
  rules are enforced on the frontend and gated at template save, with `required`
  / visibility additionally enforced at submit time.

## When we would migrate to Formily

Migrate only if at least one of these becomes true:

1. **Explicit rubric / judge requirement.** A judge or the challenge rubric
   explicitly requires Formily (e.g. demonstrating Formily proficiency, or
   interop with a Formily-based schema). This is the primary trigger; the
   custom engine is otherwise sufficient.
2. **Widget-type explosion.** The product needs many new widget types (matrices,
   tables, repeatable sections, conditional sub-forms, complex layouts) where
   re-implementing each in the custom registry costs materially more than
   Formily's `x-component` model would.
3. **Need for Formily-ecosystem validators / reactions.** Requirements outgrow
   the small `expr-eval` rule layer and would benefit from Formily's
   `x-validator` / `x-reactions` graph and its third-party validator ecosystem.

Absent any of these, the custom engine remains the right choice.

## Migration approach if required (adapter layer)

If a migration is triggered, do **not** rewrite the Owner (Designer) and Labeler
(Renderer) flows at once. Stage it behind an adapter so the existing JSON schema
stays the source of truth:

1. **Schema adapter (TS).** Write a pure function that maps the existing
   `TemplateSchema` (`apps/web/src/renderer/types.ts`) to a Formily JSON schema:
   `widget` → `x-component`, `visibleWhen` → `x-reactions`, `required` /
   `requiredWhen` / `minLength` / `maxLength` / `regex` / `customRule` →
   `x-validator`, `options` → component props, and `ShowItem` / `LLMTrigger`
   → registered custom Formily components. Keep the existing schema as the
   persisted/stored format; the adapter runs at render time.
2. **Render behind a flag.** Introduce a Formily-backed renderer behind a
   feature flag and run it side-by-side with `SchemaRenderer` on the Labeler
   answer page. Verify parity (visibility, required, length, regex, custom rule,
   hidden-value pruning, frozen-version binding) against the existing renderer's
   test fixtures before switching the default.
3. **Migrate the Designer second.** Only after the runtime renderer is proven do
   we move the Designer to emit (or round-trip through) the Formily schema. The
   Designer can continue to author the existing schema and adapt-on-save, so
   stored templates and frozen `template_version` history remain valid.
4. **Keep the backend validator on the stored schema.** The Go validators
   (`submission/validate.go`, `handler/template_validate.go`) keep validating the
   stored JSON schema. If the stored format ever becomes Formily-native, port
   the validators to the new shape in the same step — but never let the
   frontend validate a shape the backend cannot.

This keeps the migration incremental and reversible: the JSON schema is the
contract, the adapter isolates Formily, and Owner/Labeler flows are cut over one
at a time rather than in a single rewrite.

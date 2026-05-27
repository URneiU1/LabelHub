# CLAUDE.md

## Project Workflow Notes

After every code change, update project documentation before the final handoff:

- Update `README.md` with what was completed, what still needs improvement, next steps, and verification.
- Update this file when Claude-specific workflow, handoff, or review notes change.
- Keep entries factual and tied to commits or completed work.

## Current Handoff

- Current branch: **`s5-engineering-quality`**. S5 engineering-quality sprint is complete from `docs/PLAN-S5-IMPL.md`: CI now includes `./pkg/exporter` and a separate `-tags=integration` job; state machine transition coverage is 100%; `review.Apply` and outbox publisher tests were expanded; AI worker verdict mapping and circuit-open failover are covered; testcontainers integration now covers submit→outbox/Redis→AI result→revise→resubmit→approve→export against real MySQL+Redis; web `strict:true` is enabled and Renderer/Designer round-trip tests cover ShowItem modes, LLMTrigger, and Designer payload→Renderer parsing; OpenAPI fallback lives in `docs/openapi.yaml`, `pnpm -F web gen:api` generates `schema.d.ts`, Postman main-flow collection exists, and `ErrorBoundary` wraps the app root; exporter row-load DB errors are retryable instead of terminal failed; stats dimension averages only count the current submission revision; StatsBoard retry uses request sequence guarding; production compose/Caddy/Dockerfile templates plus `make dev`, ARCHITECTURE, DEPLOY, and S5_ACCEPTANCE docs are in place. Targeted coverage gates passed for statemachine/review/outbox/handler/exporter/worker, integration passed locally with Colima env, deploy compose config passed with `deploy/.env.example`, `pnpm -F web build/lint/gen:api` passed, and targeted web tests passed.
- Latest completed work: **S4 (async multi-format export + Owner stats board) is done** on branch `s4-async-export`. New `pkg/exporter` module (shared by api+worker) encodes JSON/JSONL/CSV/XLSX; `POST /tasks/:id/exports` writes exports(queued)+outbox+audit in one tx, the export worker handler runs `exporter.Run` (queued→running→succeeded, atomic temp→rename), and HMAC-signed `/exports/download` streams the file (expired 410 / tampered 401 / path-traversal 403). Owner dashboard gains `ExportPanel` (format/field-map/history-poll/download) and `StatsBoard` (VChart: progress/pass-rate/status/AI-vs-human/dimension averages via `GET /tasks/:id/stats`). `first_come` has an explicit concurrency acceptance test. All 4 formats validated end-to-end against live MySQL+Redis. See `docs/S4_ACCEPTANCE.md`.
- Also closed earlier this session: S0–S3 review findings (commit `6ead3c1` — failing outbox test, autosave race, JWT server-side revocation+rotation, login rate-limit, BatchDryRun async, security hardening).
- Current focus: continue `docs/PLAN-S6-IMPL.md` Day3 after Day1 primitives, Day2 status/empty/loading replacement, and Runtime Tabs interaction landed. Owner Dashboard, ExportPanel, Labeler Plaza, and Reviewer Queue now reuse shared `StatusBadge`/`EmptyState`/`LoadingBlock`; `SchemaRenderer` Tabs now render a real tablist/tabpanel and preserve flat answers across tab switches. Next is Designer responsive layout plus nested Group/Tabs canvas usability. Note: `make worker` needs LLM config to boot (`LLM_PROVIDER=mock` for export-only smoke); Makefile go-run targets fixed to package paths.

## Known Follow-Ups

- AI queue/failover/provider, prompt enable controls, Owner baseline/prompt product controls, Owner golden sample management, task-scoped dry-run history listing, durable queued single dry-run polling, Owner history/trend/guard table, Owner batch run wiring, backend batch semantics, basic batch pacing, AI auto-approve, Labeler autosave/revision UX, Reviewer batch operations, Reviewer AI verdict/detail/audit/retry display, Reviewer rule selection/activation plus Owner edit deep-link, provider retry/backoff controls, env-gated dry-run quota/circuit breaker, and AI worker 5xx circuit breaker are closed for the first S3 slices.
- Remaining S3 product follow-ups are optional polish rather than blockers for the current demo: async batch dry-run, richer charted trends/alerts, and broader seeded browser smoke coverage.
- S2 Designer v1 still lacks canvas-level nested dragging inside Tabs/Group and fuller layout editing.
- Formily, TipTap, `packages/schema-spec`, and `@dnd-kit/core` are still migration-level choices; do not mix them into small hardening commits without an explicit plan.
- Add FileUpload download/preview authorization and orphan temp-file cleanup.
- Add Owner API/UI for assigning `task_reviewers`; current demo assignment is seeded for `reviewer1`.

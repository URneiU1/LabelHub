# CLAUDE.md

## Project Workflow Notes

After every code change, update project documentation before the final handoff:

- Update `README.md` with what was completed, what still needs improvement, next steps, and verification.
- Update this file when Claude-specific workflow, handoff, or review notes change.
- Keep entries factual and tied to commits or completed work.

## Current Handoff

- Latest completed work: first S0-S3 plan-gap polish is in place on top of the S3 demo closure: GitHub Actions CI, ops-profile asynqmon behind nginx basic auth, six-component `/style-guide`, Labeler 3s autosave, schema `regex`/`requiredWhen`, and AI worker provider-5xx circuit breaker.
- Current focus: stabilize and demo-test the S3 AI-assisted annotation loop end to end.
- Next priority: run a local seeded browser smoke for the Owner -> Labeler -> AI -> Reviewer -> revision path, then decide whether single golden dry-run needs durable Asynq execution before the contest demo.

## Known Follow-Ups

- AI queue/failover/provider, prompt enable controls, Owner baseline/prompt product controls, Owner golden sample management, task-scoped dry-run history listing, queued single dry-run polling, Owner history/trend table, Owner batch run wiring, backend batch semantics, basic batch pacing, AI auto-approve, Labeler autosave/revision UX, Reviewer batch operations, Reviewer AI verdict/detail/audit/retry display, Reviewer rule selection plus Owner edit deep-link, provider retry/backoff controls, env-gated dry-run quota/circuit breaker, and AI worker 5xx circuit breaker are closed for the first S3 slices, but S3 is not product-complete until durable dry-run execution, direct Reviewer-side rule switching, richer trend/history analysis, and operator-facing guard state are explicitly accepted or deferred.
- S2 Designer v1 still lacks canvas-level nested dragging inside Tabs/Group and fuller layout editing.
- Formily, TipTap, `packages/schema-spec`, and `@dnd-kit/core` are still migration-level choices; do not mix them into small hardening commits without an explicit plan.
- Add FileUpload download/preview authorization and orphan temp-file cleanup.
- Add Owner API/UI for assigning `task_reviewers`; current demo assignment is seeded for `reviewer1`.

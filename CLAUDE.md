# CLAUDE.md

## Project Workflow Notes

After every code change, update project documentation before the final handoff:

- Update `README.md` with what was completed, what still needs improvement, next steps, and verification.
- Update this file when Claude-specific workflow, handoff, or review notes change.
- Keep entries factual and tied to commits or completed work.

## Current Handoff

- Latest completed work: S3 demo hardening is now closed for the current slice: single golden dry-run uses durable `ai:dry-run` outbox/asynq worker execution, Reviewer can directly activate a task prompt/rule within the review access boundary, and Owner history shows guard state plus match-rate/average-score summary.
- Current focus: stabilize and demo-test the S3 AI-assisted annotation loop end to end.
- Next priority: run a local seeded browser smoke for the Owner -> Labeler -> AI -> Reviewer -> revision path, including Reviewer rule activation and worker-completed golden dry-run polling.

## Known Follow-Ups

- AI queue/failover/provider, prompt enable controls, Owner baseline/prompt product controls, Owner golden sample management, task-scoped dry-run history listing, durable queued single dry-run polling, Owner history/trend/guard table, Owner batch run wiring, backend batch semantics, basic batch pacing, AI auto-approve, Labeler autosave/revision UX, Reviewer batch operations, Reviewer AI verdict/detail/audit/retry display, Reviewer rule selection/activation plus Owner edit deep-link, provider retry/backoff controls, env-gated dry-run quota/circuit breaker, and AI worker 5xx circuit breaker are closed for the first S3 slices.
- Remaining S3 product follow-ups are optional polish rather than blockers for the current demo: async batch dry-run, richer charted trends/alerts, and broader seeded browser smoke coverage.
- S2 Designer v1 still lacks canvas-level nested dragging inside Tabs/Group and fuller layout editing.
- Formily, TipTap, `packages/schema-spec`, and `@dnd-kit/core` are still migration-level choices; do not mix them into small hardening commits without an explicit plan.
- Add FileUpload download/preview authorization and orphan temp-file cleanup.
- Add Owner API/UI for assigning `task_reviewers`; current demo assignment is seeded for `reviewer1`.

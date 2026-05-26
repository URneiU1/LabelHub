# CLAUDE.md

## Project Workflow Notes

After every code change, update project documentation before the final handoff:

- Update `README.md` with what was completed, what still needs improvement, next steps, and verification.
- Update this file when Claude-specific workflow, handoff, or review notes change.
- Keep entries factual and tied to commits or completed work.

## Current Handoff

- Latest completed work: S3 demo productization closure is in place on top of the prior Designer/Reviewer AI slices: Owner can edit baseline inline, AI Prompt dimensions/thresholds use form controls, single golden sample dry-run is queued/polled with background execution, AI worker can auto-approve pass results when human review is disabled, Labeler has a revising queue with previous reject reason, and Reviewer has batch approve/revise plus previous-opinion display.
- Current focus: stabilize and demo-test the S3 AI-assisted annotation loop end to end.
- Next priority: run a local seeded browser smoke for the Owner -> Labeler -> AI -> Reviewer -> revision path, then decide whether single golden dry-run needs durable Asynq execution before the contest demo.

## Known Follow-Ups

- AI queue/failover/provider, prompt enable controls, Owner baseline/prompt product controls, Owner golden sample management, task-scoped dry-run history listing, queued single dry-run polling, Owner history/trend table, Owner batch run wiring, backend batch semantics, basic batch pacing, AI auto-approve, Labeler revision UX, Reviewer batch operations, Reviewer AI verdict/detail/audit/retry display, Reviewer rule selection plus Owner edit deep-link, provider retry/backoff controls, and env-gated dry-run quota/circuit breaker are closed for the first S3 slices, but S3 is not product-complete until durable dry-run execution, direct Reviewer-side rule switching, richer trend/history analysis, and operator-facing guard state are explicitly accepted or deferred.
- S2 Designer v1 still lacks canvas-level nested dragging inside Tabs/Group and fuller layout editing.
- Add FileUpload download/preview authorization and orphan temp-file cleanup.
- Add Owner API/UI for assigning `task_reviewers`; current demo assignment is seeded for `reviewer1`.

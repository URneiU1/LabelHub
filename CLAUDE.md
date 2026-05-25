# CLAUDE.md

## Project Workflow Notes

After every code change, update project documentation before the final handoff:

- Update `README.md` with what was completed, what still needs improvement, next steps, and verification.
- Update this file when Claude-specific workflow, handoff, or review notes change.
- Keep entries factual and tied to commits or completed work.

## Current Handoff

- Latest completed work: OpenAI-compatible provider retry/backoff controls are implemented for network errors, HTTP 429, and 5xx, with bounded attempts/backoff env knobs and fail-fast sanitized handling for 400/401/schema/threshold failures.
- Current focus: continue S2 Designer buildout and the remaining S3 product surface.
- Next priority: Designer append/delete/simple property editing, plus server-side dry-run quota/circuit breaker and richer Owner dry-run trend/history analysis.

## Known Follow-Ups

- AI queue/failover/provider, prompt enable controls, Owner golden sample management, task-scoped dry-run history listing, Owner history/trend table, Owner batch run wiring, backend batch semantics, basic batch pacing, Reviewer AI verdict display, and provider retry/backoff controls are closed for the first S3 slices, but S3 is not product-complete until server-side quota/circuit breaker and richer trend/history analysis are added.
- Add FileUpload download/preview authorization and orphan temp-file cleanup.
- Add Owner API/UI for assigning `task_reviewers`; current demo assignment is seeded for `reviewer1`.

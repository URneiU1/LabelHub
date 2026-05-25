# CLAUDE.md

## Project Workflow Notes

After every code change, update project documentation before the final handoff:

- Update `README.md` with what was completed, what still needs improvement, next steps, and verification.
- Update this file when Claude-specific workflow, handoff, or review notes change.
- Keep entries factual and tied to commits or completed work.

## Current Handoff

- Latest completed work: S3 task-scoped AI dry-run history API is implemented. Owner/admin can list `ai_dry_runs` for a task, optionally filtered by `golden_sample_id`, with JSON-valued payload/answer snapshots and result data for future Owner history/trend UI.
- Current focus: continue S2 Designer buildout and the remaining S3 product surface.
- Next priority: Designer append/delete/simple property editing, plus S3 true batch dry-run contract / server-side dry-run throttle and provider rate-limit backoff, then Owner history/trend UI and Reviewer AI verdict/score display.

## Known Follow-Ups

- AI queue/failover/provider, prompt enable controls, Owner golden sample management, and task-scoped dry-run history listing are closed for the first S3 slices, but S3 is not product-complete until batch semantics, rate-limit/backoff controls, Owner result history/trend views, and Reviewer AI verdict display are added.
- Batch golden sample dry-run endpoint is deferred; current Owner result table intentionally runs visible samples serially through the single-sample endpoint until partial-failure semantics are designed.
- Add FileUpload download/preview authorization and orphan temp-file cleanup.
- Add Owner API/UI for assigning `task_reviewers`; current demo assignment is seeded for `reviewer1`.

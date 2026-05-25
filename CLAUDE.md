# CLAUDE.md

## Project Workflow Notes

After every code change, update project documentation before the final handoff:

- Update `README.md` with what was completed, what still needs improvement, next steps, and verification.
- Update this file when Claude-specific workflow, handoff, or review notes change.
- Keep entries factual and tied to commits or completed work.

## Current Handoff

- Latest completed work: S3 golden sample Owner UI follow-up is implemented. Golden sample create drafts reset on actual task switch without breaking same-task no-op clicks, and ad-hoc AI prompt dry-run now preserves raw JSON payload/answer numbers through handler/provider message construction.
- Current focus: continue S2 Designer buildout and the remaining S3 product surface.
- Next priority: Designer append/delete/simple property editing, plus S3 dry-run history list API / true batch dry-run contract / server-side dry-run throttle and provider rate-limit backoff, then Reviewer AI verdict/score display.

## Known Follow-Ups

- AI queue/failover/provider, prompt enable controls, and Owner golden sample management are closed for the first S3 slices, but S3 is not product-complete until dry-run history/batch semantics, rate-limit/backoff controls, Owner result history/trend views, and Reviewer AI verdict display are added.
- Batch golden sample dry-run endpoint is deferred; current Owner result table intentionally runs visible samples serially through the single-sample endpoint until partial-failure semantics are designed.
- Add FileUpload download/preview authorization and orphan temp-file cleanup.
- Add Owner API/UI for assigning `task_reviewers`; current demo assignment is seeded for `reviewer1`.

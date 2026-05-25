# CLAUDE.md

## Project Workflow Notes

After every code change, update project documentation before the final handoff:

- Update `README.md` with what was completed, what still needs improvement, next steps, and verification.
- Update this file when Claude-specific workflow, handoff, or review notes change.
- Keep entries factual and tied to commits or completed work.

## Current Handoff

- Latest completed work: S3 batch golden sample dry-run API is implemented. Owner/admin can post up to 20 task-owned sample IDs, the API runs them serially, records dry-runs where possible, and returns per-sample partial success/failure results.
- Current focus: continue S2 Designer buildout and the remaining S3 product surface.
- Next priority: Designer append/delete/simple property editing, plus wiring Owner Run all to the backend batch endpoint / configurable provider rate-limit backoff, then Owner history/trend UI and Reviewer AI verdict/score display.

## Known Follow-Ups

- AI queue/failover/provider, prompt enable controls, Owner golden sample management, task-scoped dry-run history listing, and backend batch dry-run semantics are closed for the first S3 slices, but S3 is not product-complete until Owner uses the batch endpoint, rate-limit/backoff controls, Owner result history/trend views, and Reviewer AI verdict display are added.
- Add FileUpload download/preview authorization and orphan temp-file cleanup.
- Add Owner API/UI for assigning `task_reviewers`; current demo assignment is seeded for `reviewer1`.

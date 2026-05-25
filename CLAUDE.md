# CLAUDE.md

## Project Workflow Notes

After every code change, update project documentation before the final handoff:

- Update `README.md` with what was completed, what still needs improvement, next steps, and verification.
- Update this file when Claude-specific workflow, handoff, or review notes change.
- Keep entries factual and tied to commits or completed work.

## Current Handoff

- Latest completed work: Owner Run all now uses the S3 batch golden sample dry-run API, and backend batch runs support optional `LLM_BATCH_DRY_RUN_DELAY_MS` pacing between samples.
- Current focus: continue S2 Designer buildout and the remaining S3 product surface.
- Next priority: Designer append/delete/simple property editing, plus Owner dry-run history/trend UI and Reviewer AI verdict/score display.

## Known Follow-Ups

- AI queue/failover/provider, prompt enable controls, Owner golden sample management, task-scoped dry-run history listing, Owner batch run wiring, backend batch semantics, and basic batch pacing are closed for the first S3 slices, but S3 is not product-complete until finer provider backoff controls, Owner result history/trend views, and Reviewer AI verdict display are added.
- Add FileUpload download/preview authorization and orphan temp-file cleanup.
- Add Owner API/UI for assigning `task_reviewers`; current demo assignment is seeded for `reviewer1`.

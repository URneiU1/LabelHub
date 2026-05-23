# CLAUDE.md

## Project Workflow Notes

After every code change, update project documentation before the final handoff:

- Update `README.md` with what was completed, what still needs improvement, next steps, and verification.
- Update this file when Claude-specific workflow, handoff, or review notes change.
- Keep entries factual and tied to commits or completed work.

## Current Handoff

- Latest completed work: S3 golden sample dry-run linking is implemented. `ai_dry_runs` now persists golden sample links, prompt version, input snapshots, expected/actual verdict, matched flags, and finished timestamps; owner/admin users can run a task-scoped sample through the existing provider path with sanitized failure recording and sample-pinned prompt support.
- Current focus: continue S2 Designer buildout and the remaining S3 product surface.
- Next priority: Designer append/delete/simple property editing, followed by Owner UI golden sample manager + batch run result table, then Reviewer AI verdict/score display.

## Known Follow-Ups

- AI queue/failover/provider and prompt enable controls are closed for the first S3 slices, but S3 is not product-complete until golden sample management/batch results and Reviewer AI verdict display are added.
- Batch golden sample dry-run endpoint is deferred until the UI/result-table requirements are clearer; single-sample persistence now provides the data shape without committing to partial-failure semantics.
- Add FileUpload download/preview authorization and orphan temp-file cleanup.
- Add Owner API/UI for assigning `task_reviewers`; current demo assignment is seeded for `reviewer1`.

# CLAUDE.md

## Project Workflow Notes

After every code change, update project documentation before the final handoff:

- Update `README.md` with what was completed, what still needs improvement, next steps, and verification.
- Update this file when Claude-specific workflow, handoff, or review notes change.
- Keep entries factual and tied to commits or completed work.

## Current Handoff

- Latest completed work: S3 golden sample persistence API is implemented. Owner/admin users can list, create, and delete task-scoped golden samples; create validates required JSON payload/expected answer, expected verdict enum, same-task optional prompt link, and duplicate payload hashes.
- Current focus: continue S2 Designer buildout and the remaining S3 product surface.
- Next priority: Designer append/delete/simple property editing, followed by golden sample dry-run linking, golden sample management, and Reviewer AI verdict/score display.

## Known Follow-Ups

- AI queue/failover/provider and prompt enable controls are closed for the first S3 slices, but S3 is not product-complete until golden sample dry-run linking/management and Reviewer AI verdict display are added.
- Add FileUpload download/preview authorization and orphan temp-file cleanup.
- Add Owner API/UI for assigning `task_reviewers`; current demo assignment is seeded for `reviewer1`.

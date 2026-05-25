# CLAUDE.md

## Project Workflow Notes

After every code change, update project documentation before the final handoff:

- Update `README.md` with what was completed, what still needs improvement, next steps, and verification.
- Update this file when Claude-specific workflow, handoff, or review notes change.
- Keep entries factual and tied to commits or completed work.

## Current Handoff

- Latest completed work: S3 golden sample Owner UI is implemented. Golden sample create/list now preserves raw JSON and returns JSON values; Owner Dashboard can load/create/delete samples, bind active or historical prompt versions, run one sample, and run all visible samples serially into a result table using the existing single-sample dry-run endpoint.
- Current focus: continue S2 Designer buildout and the remaining S3 product surface.
- Next priority: Designer append/delete/simple property editing, plus S3 dry-run history list API / true batch dry-run contract, then Reviewer AI verdict/score display.

## Known Follow-Ups

- AI queue/failover/provider, prompt enable controls, and Owner golden sample management are closed for the first S3 slices, but S3 is not product-complete until dry-run history/batch semantics and Reviewer AI verdict display are added.
- Batch golden sample dry-run endpoint is deferred; current Owner result table intentionally runs visible samples serially through the single-sample endpoint until partial-failure semantics are designed.
- Add FileUpload download/preview authorization and orphan temp-file cleanup.
- Add Owner API/UI for assigning `task_reviewers`; current demo assignment is seeded for `reviewer1`.

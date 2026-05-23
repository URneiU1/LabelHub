# CLAUDE.md

## Project Workflow Notes

After every code change, update project documentation before the final handoff:

- Update `README.md` with what was completed, what still needs improvement, next steps, and verification.
- Update this file when Claude-specific workflow, handoff, or review notes change.
- Keep entries factual and tied to commits or completed work.

## Current Handoff

- Latest completed work: S3 AI review follow-ups are implemented. Owner Dashboard ignores stale save/settings/dry-run responses after task switches, provider HTTP error bodies stay out of persisted/provider errors, LLM schema/threshold validation failures are marked non-retryable and fail over directly to human review, and HTTP 422 coverage exists for invalid active prompts and disallowed active models.
- Current focus: continue S2 Designer buildout and the remaining S3 product surface.
- Next priority: Designer append/delete/simple property editing, followed by golden sample persistence shape, golden sample management, and Reviewer AI verdict/score display.

## Known Follow-Ups

- AI queue/failover/provider and prompt enable controls are closed for the first S3 slices, but S3 is not product-complete until golden sample persistence/management and Reviewer AI verdict display are added.
- Add FileUpload download/preview authorization and orphan temp-file cleanup.
- Add Owner API/UI for assigning `task_reviewers`; current demo assignment is seeded for `reviewer1`.

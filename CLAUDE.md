# CLAUDE.md

## Project Workflow Notes

After every code change, update project documentation before the final handoff:

- Update `README.md` with what was completed, what still needs improvement, next steps, and verification.
- Update this file when Claude-specific workflow, handoff, or review notes change.
- Keep entries factual and tied to commits or completed work.

## Current Handoff

- Latest completed work: S3 AI product layer part1 is implemented and verified. The worker now supports OpenAI-compatible Function Calling with strict Go-side validation and token/latency/raw_response recording, while keeping deterministic/mock fallback. Owner can create versioned AI prompts and run scoped dry-runs without mutating formal submission state.
- Current focus: continue S2 Designer buildout and the remaining S3 product surface.
- Next priority: Designer append/delete/simple property editing, followed by task-level AI enable controls, golden sample management, and Reviewer AI verdict/score display.

## Known Follow-Ups

- AI queue/failover/provider is closed for the first S3 slice, but S3 is not product-complete until prompt enable controls, golden sample management, and Reviewer AI verdict display are added.
- Add FileUpload download/preview authorization and orphan temp-file cleanup.
- Add Owner API/UI for assigning `task_reviewers`; current demo assignment is seeded for `reviewer1`.

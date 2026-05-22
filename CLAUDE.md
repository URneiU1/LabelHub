# CLAUDE.md

## Project Workflow Notes

After every code change, update project documentation before the final handoff:

- Update `README.md` with what was completed, what still needs improvement, next steps, and verification.
- Update this file when Claude-specific workflow, handoff, or review notes change.
- Keep entries factual and tied to commits or completed work.

## Current Handoff

- Latest completed work: P1/P2 closeout is implemented and verified. FileUpload answers now attach uploads in the submit transaction; schema validation is aligned across Go/TS; Labeler/Reviewer page-level schema runtime tests exist; AI submit creates pending `ai_reviews` + `ai:review` outbox and worker success/failover moves submissions to `human_reviewing`.
- Current focus: continue S2 Designer buildout, then S3 AI product layer.
- Next priority: Designer append/delete/simple property editing, followed by Owner AI Prompt/golden dry-run UI and real LLM provider integration.

## Known Follow-Ups

- AI queue/failover is closed, but worker still uses deterministic evaluator; add real Doubao/OpenAI-compatible provider and Function Calling validation before claiming S3 complete.
- Add FileUpload download/preview authorization and orphan temp-file cleanup.
- Add Owner API/UI for assigning `task_reviewers`; current demo assignment is seeded for `reviewer1`.

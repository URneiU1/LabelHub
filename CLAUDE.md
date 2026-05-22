# CLAUDE.md

## Project Workflow Notes

After every code change, update project documentation before the final handoff:

- Update `README.md` with what was completed, what still needs improvement, next steps, and verification.
- Update this file when Claude-specific workflow, handoff, or review notes change.
- Keep entries factual and tied to commits or completed work.

## Current Handoff

- Latest completed work: P1/P2 edge hardening is implemented and verified. FileUpload can reuse an already attached file when a revising submission is resubmitted, but only if that file belongs to the same task/user/submission history. AI worker duplicate replay now no-ops after finalized reviews, failover cannot mark succeeded reviews dead, and the API process has an AI review sweeper for stuck pending/running reviews.
- Current focus: continue S2 Designer buildout, then S3 AI product layer.
- Next priority: Designer append/delete/simple property editing, followed by Owner AI Prompt/golden dry-run UI and real LLM provider integration.

## Known Follow-Ups

- AI queue/failover/sweeper is closed, but worker still uses deterministic evaluator; add real Doubao/OpenAI-compatible provider and Function Calling validation before claiming S3 complete.
- Add FileUpload download/preview authorization and orphan temp-file cleanup.
- Add Owner API/UI for assigning `task_reviewers`; current demo assignment is seeded for `reviewer1`.

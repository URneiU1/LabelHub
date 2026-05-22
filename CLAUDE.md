# CLAUDE.md

## Project Workflow Notes

After every code change, update project documentation before the final handoff:

- Update `README.md` with what was completed, what still needs improvement, next steps, and verification.
- Update this file when Claude-specific workflow, handoff, or review notes change.
- Keep entries factual and tied to commits or completed work.

## Current Handoff

- Latest completed work: Phase 1 P0 backend concurrency/RBAC hardening is implemented locally and ready to commit.
- Current focus: S2/S3 hardening after SchemaRenderer runtime and review/claim transaction fixes.
- Next priority: wire SchemaRenderer into the labeler/reviewer flows, then continue Designer editing capability and AI outbox closure.

## Known Follow-Ups

- Resolve or document the `lottie-web` eval build warning.
- Address web chunk size warning with route-level or vendor chunk splitting when the UI stabilizes.
- Add Owner API/UI for assigning `task_reviewers`; current demo assignment is seeded for `reviewer1`.

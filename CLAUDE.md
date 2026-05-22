# CLAUDE.md

## Project Workflow Notes

After every code change, update project documentation before the final handoff:

- Update `README.md` with what was completed, what still needs improvement, next steps, and verification.
- Update this file when Claude-specific workflow, handoff, or review notes change.
- Keep entries factual and tied to commits or completed work.

## Current Handoff

- Latest completed work: Phase 2 labeler/reviewer SchemaRenderer runtime wiring is implemented locally and ready to commit.
- Current focus: S2/S3 hardening after dynamic runtime and review/claim transaction fixes.
- Next priority: continue Designer append/delete/property editing, then close AI outbox + worker failover.

## Known Follow-Ups

- AI review failover remains incomplete: add transactional outbox publisher, pending `ai_reviews`, worker retry/dead-state handling, and human-review fallback.
- Move labeler item ownership verification into the submission service transaction before adding reclaim/reassign flows.
- Change DB default for `ai_review_enabled` to safe-off and migrate old unconfigured tasks.
- Add Owner API/UI for assigning `task_reviewers`; current demo assignment is seeded for `reviewer1`.

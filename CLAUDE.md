# CLAUDE.md

## Project Workflow Notes

After every code change, update project documentation before the final handoff:

- Update `README.md` with what was completed, what still needs improvement, next steps, and verification.
- Update this file when Claude-specific workflow, handoff, or review notes change.
- Keep entries factual and tied to commits or completed work.

## Current Handoff

- Latest completed work: Template Designer now has route-race fail-closed hardening, copy/order controls, drag ordering, Tabs/Group structure widgets, nested child-field property editing, per-field validation display, and real item payload preview; S3 dry-run endpoints now have env-gated task-scoped quota/circuit breaker; Reviewer Detail now returns and renders latest AI review details, prompt metadata, audit logs, and failed/dead AI retry; Reviewer rule configuration now lists real task prompt versions and deep-links to Owner prompt editing.
- Current focus: continue S2 Designer buildout and the remaining S3 product surface.
- Next priority: decide whether Reviewer should only inspect/jump to Owner editing or also request active rule switching, or continue canvas-level nested dragging/layout polish for Tabs/Group.

## Known Follow-Ups

- AI queue/failover/provider, prompt enable controls, Owner golden sample management, task-scoped dry-run history listing, Owner history/trend table, Owner batch run wiring, backend batch semantics, basic batch pacing, Reviewer AI verdict/detail/audit/retry display, Reviewer rule selection plus Owner edit deep-link, provider retry/backoff controls, and env-gated dry-run quota/circuit breaker are closed for the first S3 slices, but S3 is not product-complete until direct Reviewer-side rule switching is explicitly accepted or deferred, richer trend/history analysis, and operator-facing guard state are added.
- S2 Designer v1 still lacks canvas-level nested dragging inside Tabs/Group and fuller layout editing.
- Add FileUpload download/preview authorization and orphan temp-file cleanup.
- Add Owner API/UI for assigning `task_reviewers`; current demo assignment is seeded for `reviewer1`.

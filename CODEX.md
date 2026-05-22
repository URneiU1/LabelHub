# CODEX.md

## Coding Agent Rules

After every code-writing turn:

- Update `README.md` with:
  - what was completed;
  - what still needs improvement;
  - what should happen next;
  - what verification was run.
- Update `CODEX.md` when Codex-specific workflow notes, implementation constraints, or handoff details change.
- If Claude-facing workflow notes changed too, update `CLAUDE.md` in the same commit.
- Commit each logical change and keep the worktree clean before final handoff when possible.

## Current Project State

- S0 and S1 are complete enough to support the S2 flow.
- S2 Day1 backend template endpoints are in place.
- S2 Day2 SchemaRenderer runtime is in place.
- Security boundary fixes are in place as of `0b871d5`.
- Phase 1 P0 backend concurrency/RBAC hardening now adds `task_reviewers`, transactional claim draft creation, locked review apply, and locked revision numbering.
- Phase 2 runtime wiring now renders Labeler Plaza and Reviewer Queue from historical template schema, with real RichText/JSONEditor/FileUpload/LLMTrigger widgets and preserved `export_fields`/`x-*` schema metadata.

## Next Work

- Continue Designer implementation with append/delete/simple property editing.
- Close AI review outbox/worker failover before marking S3 complete.
- Move labeler item ownership checks into the submission service transaction before reclaim/reassign exists.
- Change the database default for `ai_review_enabled` to safe-off and migrate unconfigured tasks.
- Keep tests focused on behavior and role/resource boundaries.
- Add reviewer assignment management endpoints/UI before treating multi-reviewer operation as product-complete.

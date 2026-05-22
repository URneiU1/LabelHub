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
- P1 safety defaults now recheck labeler item ownership inside the submission service transaction and make AI review disabled by default at schema/model/migration level.
- P1/P2 closeout is in place: FileUpload submit attaches uploaded files to the revision; Go/TS schema validators are aligned; Labeler/Reviewer page tests cover schema runtime; AI submit writes pending `ai_reviews` and `ai:review` outbox; API publisher and worker consume/failover path move AI-reviewed submissions to `human_reviewing`.

## Next Work

- Continue Designer implementation with append/delete/simple property editing.
- Add real Doubao/OpenAI-compatible LLM provider, Function Calling schema validation, golden dry-run flow, and Reviewer AI verdict display before marking S3 complete.
- Add FileUpload download/preview authorization and orphan temp cleanup.
- Keep tests focused on behavior and role/resource boundaries.
- Add reviewer assignment management endpoints/UI before treating multi-reviewer operation as product-complete.

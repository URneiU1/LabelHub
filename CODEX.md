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
- P1/P2 edge hardening is in place: revising submissions may reuse attached FileUpload keys from the same submission history; AI worker finalized duplicate tasks no-op instead of replaying; API-side AI sweeper moves stale pending/running reviews out of `ai_reviewing`.
- S3 AI product layer part1 is in place: shared `llmreview` provides mock + OpenAI-compatible Function Calling, worker records structured verdict/score/dimensions/reason with token/latency/raw_response metadata, and Owner can version AI prompts plus run scoped dry-runs.
- S3 review fixes are in place: Owner prompt create now defaults model server-side, dimensions metadata survives UI re-save, and worker retry success clears stale `error_msg`.
- S3 task-level AI review controls are in place: owner/admin can toggle `tasks.ai_review_enabled` via `POST /tasks/:taskId/ai-review-settings`; enabling requires an active prompt for the task, disabling keeps prompt history, and Owner Dashboard reflects the status inline.
- Owner Dashboard now resets the AI Prompt form to defaults when switching to a task with no prompts or when prompt loading fails, preventing accidental cross-task prompt copies.
- S3 AI P1 hardening is in place: AI worker payload/idempotency anchors are verified and bound through running/completion/failover updates; retryable worker failures return reviews to `failed`; submission submit locks/reloads task before AI planning and rejects invalid active prompts; LLM verdict/score threshold mismatches are rejected; Owner Dashboard ignores stale prompt loads and disables prompt actions while loading failed or is in flight.

## Next Work

- Continue Designer implementation with append/delete/simple property editing.
- Add golden sample persistence shape before golden sample management UI and Reviewer AI verdict display.
- Add FileUpload download/preview authorization and orphan temp cleanup.
- Keep tests focused on behavior and role/resource boundaries.
- Add reviewer assignment management endpoints/UI before treating multi-reviewer operation as product-complete.

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
- S3 AI review follow-ups are in place: Owner Dashboard action responses are task/action guarded after task switches; provider HTTP error bodies are not exposed in persisted error messages; LLM schema/threshold validation errors are non-retryable and fail over directly to human review; HTTP 422 coverage exists for invalid active prompts and disallowed active models.
- Owner Dashboard same-task task-list clicks are no-ops, so they no longer invalidate the current in-flight save/settings/dry-run guard or leave loading stuck.
- Golden sample persistence API is in place for owner/admin users: list/create/delete samples under a task, validate expected verdicts and same-task prompt links, hash canonical payload JSON for duplicate protection, and return 409 for duplicate task payloads.
- Golden sample single-sample dry-run linking is in place: `ai_dry_runs` now stores sample links, prompt version, input snapshots, expected/actual verdicts, matched flags, and finished timestamps; `POST /tasks/:taskId/golden-samples/:sampleId/dry-run` runs through the existing provider path, prefers a sample-pinned prompt before task active prompt, and records sanitized failures.
- Golden sample JSON contract is hardened: create uses raw JSON/canonical hash so large numeric IDs are not coerced through JS/Go float64, and list/create responses return JSON values for `payload` and `expectedAnswer`.
- Owner Dashboard golden sample manager is in place: owners can load/create/delete samples, bind active or historical prompt versions, run one sample, or run all visible samples serially through the single-sample dry-run endpoint; result rows show expected/actual verdict, match state, score, provider/model, reason/error, and dryRunId.
- Owner Dashboard now resets golden sample create drafts only when switching to a different task, preventing Task A payload/answer/verdict/notes from being accidentally created under Task B while preserving same-task no-op clicks.
- Ad-hoc AI prompt dry-run now preserves raw JSON payload/answer end to end: Owner UI sends the textarea JSON through `apiPostRawJSON`, the handler accepts `json.RawMessage`, and OpenAI-compatible message construction uses `UseNumber`, avoiding float64 precision loss for large numeric IDs.
- AI prompt dry-run regression coverage now includes Owner raw request body precision, golden sample prompt choice reset, and backend missing/null/invalid raw JSON validation.
- Task-scoped AI dry-run history API is in place: owner/admin can call `GET /tasks/:taskId/ai-dry-runs?golden_sample_id=&limit=` and receive JSON-valued snapshots/results plus expected/actual verdict, matched flag, status/error, prompt version, and timestamps.
- Batch golden sample dry-run API is in place: owner/admin can call `POST /tasks/:taskId/golden-samples/dry-runs` with up to 20 task-owned sample IDs; the server runs them serially, returns per-sample partial results, and does not let provider/config/evaluator failure for one sample block the rest.
- Owner Run all now uses the backend batch endpoint and maps partial results into the existing result table; batch dry-runs can be paced with `LLM_BATCH_DRY_RUN_DELAY_MS` when a provider needs request spacing.
- Owner dry-run history/trend UI is in place: the Golden Samples area loads recent task dry-runs, supports per-sample filtering through the existing history API, shows compact total/matched/mismatch/failed summary, and lists expected/actual verdict, match state, status/error, prompt version, and finished time with stale task guards.
- Reviewer AI verdict display is in place: Reviewer Queue and detail use existing submission `aiVerdict/aiScore` fields to show AI precheck verdict/score, with an empty state when no AI result exists.

## Next Work

- Continue Designer implementation with append/delete/simple property editing.
- Add richer Owner dry-run trend/history analysis after the minimal summary table proves useful.
- Add finer provider-specific 429/backoff policy before enabling heavier golden sample runs.
- Add FileUpload download/preview authorization and orphan temp cleanup.
- Keep tests focused on behavior and role/resource boundaries.
- Add reviewer assignment management endpoints/UI before treating multi-reviewer operation as product-complete.

# Owner Data Acceptance Loop (强闭环数据验收)

An Owner-side quality-acceptance closed loop layered on top of the existing review
pipeline. Confirmed decisions:

- **Granularity**: a batch = per-task snapshot of the currently-`approved` submissions.
- **验收不通过 (reject)**: reopens spot-check-flagged approved items to re-review via the
  state machine (`approved → human_reviewing`).
- **Export is NOT gated** by acceptance — acceptance is a status marker only.

## Roles

- **Owner/Admin**: start a batch, spot-check approved items, accept / reject. Read + the
  acceptance actions only — the Owner never casts review verdicts (that stays with Reviewer).

## Data

- `acceptance_batches(id, task_id, status[pending|accepted|rejected], approved_count, note,
  decided_by, decided_at, created_by, created_at)`. At most one `pending` batch per task
  (enforced in the service layer).
- `acceptance_spot_checks(id, batch_id, submission_id, result[ok|flag], note, checked_by,
  created_at)`. Unique `(batch_id, submission_id)` — re-checking a submission upserts.

## Actions (`src/api/.../service/acceptance`)

- **Start**: reject if a pending batch already exists; snapshot `approved_count`; create a
  pending batch; audit `acceptance_batch/start`.
- **RecordSpotCheck**: batch must be pending; submission must belong to the task and be
  `approved`; upsert the spot-check.
- **Accept**: pending → accepted (`decided_by/at`, note); audit `accept`. Export stays open.
- **Reject**: pending → rejected; for each flagged-and-still-`approved` submission, reopen
  `approved → human_reviewing` via `statemachine.Apply(EventAcceptanceReopen)` with a
  RowsAffected guard + a per-submission audit row; audit `reject`. Reopened items re-enter
  human review (a reviewer re-confirms; prior approve records are not cleared — MVP).

## State machine

- New `EventAcceptanceReopen`; transition `{approved → human_reviewing}`. The `submissions`
  status enum already contains both states, so no submissions enum migration is needed.

## API (owner/admin, via `loadOwnedTask`)

- `GET  /tasks/:taskId/acceptance` — latest batch + its spot-checks + current approved count
- `POST /tasks/:taskId/acceptance` — start
- `POST /tasks/:taskId/acceptance/spot-checks` — `{submission_id, result, note}`
- `POST /tasks/:taskId/acceptance/accept` — `{note}`
- `POST /tasks/:taskId/acceptance/reject` — `{note}`

## Frontend

- Owner: new section (质检/验收) — start / spot-check / accept / reject + status + audit.
- Reviewer: surface the existing 审核结果列表 view as a dedicated sidebar nav entry.
- Copy: 三级 → 两级 (初审 → 终审); rename Owner sections; reviewer breadcrumb 审核与质检 → 审核中心.

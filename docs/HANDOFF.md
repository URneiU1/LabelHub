# LabelHub · Session Handoff (UI re-skin + feature completion)

> Last updated: 2026-05-30. Read this first when resuming. Related: `SPEC-UI-RESKIN.md`, `PLAN-UI-RESKIN-IMPL.md`, `CHANGELOG.md`.

## TL;DR
The deployed demo at **http://43.155.210.70** now has all four core capabilities working end-to-end on the real backend (Go API + asynq worker + **real 豆包/Doubao** AI review), re-skinned to the organizer's `labelhub-ui-demo` look, populated with **real demo data**. Login: `owner1` / `labeler1` / `reviewer1`, password **`123456`** (admin: `admin1`).

## Goal / approach (locked decisions)
- **Re-skin `apps/web` in place** with the `labelhub-ui-demo` (`~/labelhub-ui-demo`) **layout + visuals**, on the **real backend + real data**. ui-demo is the organizer's static reference (its screenshot data is fake; ours is real).
- **Component library = Semi Design, KEPT** (organizer recommends Semi; already wired). Do **NOT** introduce Arco, do **NOT** remove Semi. The ui-demo look is mostly its custom `.lh-*` CSS (copied to `apps/web/src/styles/lh/`) painted on top of Semi.
- Stack (organizer-recommended, already matched): React 18 + TS + Semi + self-built Schema renderer (`src/renderer/`, not Formily) + context/Zustand + native HTML5 drag (Designer).
- Role-based login kept (no open demo nav).

## Deployment (the live demo)
- Host: **Tencent Cloud Lightweight, Seoul (首尔), 免备案**, Ubuntu, 2C4G + 4G swap. Public IP **43.155.210.70**.
- SSH: `ssh -i ~/Downloads/labelhub.pem ubuntu@43.155.210.70` (the `.pem` is on the local Mac; `ubuntu` is in the docker group, no sudo needed for docker).
- Stack runs via `deploy/docker-compose.prod.yml` (mysql/redis/api/worker/web/asynqmon/caddy). Code lives at `~/labelhub` on the server.
- `deploy/.env` ON THE SERVER holds the real secrets (random JWT/DB/Redis, EXPORT_DOWNLOAD_SECRET, the **Doubao** LLM config, asynqmon basic-auth hash). It is gitignored / NOT in the repo. `CADDY_SITE_ADDRESS=:80` (HTTP, IP-direct; no domain yet). `API_CORS_ORIGINS=http://43.155.210.70`.
- LLM: `LLM_PROVIDER=doubao`, `LLM_BASE_URL=https://ark.cn-beijing.volces.com/api/v3`, `LLM_MODEL=ep-20260514105718-jthdm` (the API key is in the server `deploy/.env`, sourced from the local `~/Desktop/LabelHub/.env`). Verified reachable from Seoul (200/~2.4s).
- asynqmon queue UI: `http://43.155.210.70/asynqmon/` (basic auth user `admin`; password is in the server `deploy/.env` / deploy notes — not committed).

### Deploy workflow (how every change ships — NO CI to prod; rsync + rebuild)
The repo is **private** and local `main` is **ahead of origin** (work is NOT pushed). Deploy = rsync the working tree to the server, then rebuild only the changed service:
```bash
# full web/code sync (excludes node_modules/.git/.env/etc):
rsync -az --delete -e "ssh -i ~/Downloads/labelhub.pem" \
  --exclude='.git' --exclude='node_modules' --exclude='data' --exclude='.env' \
  --exclude='*.pem' --exclude='apps/web/dist' --exclude='.DS_Store' --exclude='.claude' \
  /Users/dadadineiyou/Desktop/LabelHub/ ubuntu@43.155.210.70:/home/ubuntu/labelhub/
# backend only: rsync apps/api/ then:
ssh -i ~/Downloads/labelhub.pem ubuntu@43.155.210.70 \
 'cd ~/labelhub && docker compose --env-file deploy/.env -f deploy/docker-compose.prod.yml up -d --build <web|api worker>'
```
- `up -d --build web` rebuilds frontend (~2-4 min); `up -d --build api worker` rebuilds Go (~2-3 min). Both run them as a **background** Bash task (slow) and verify after.
- Verify: `curl -s -o /dev/null -w "%{http_code}" http://43.155.210.70/` (200), and browser-smoke via chrome-devtools MCP (login by setting `localStorage` `labelhub_access_token`+`labelhub_current_user` from `/api/v1/auth/login`, then navigate `/labeler|/owner|/reviewer`; **close pages after**). NOTE: the chrome-devtools browser carries a stale session across smokes — always `localStorage.clear()` then re-login in the eval script.

## What's DONE (all deployed + verified)
1. **Designer + Renderer (first key capability)** — `src/renderer/` (11 widgets incl. ShowItem/Group/Tabs/Input/TextArea/Radio/Tags/RichText/JSONEditor/FileUpload/LLMTrigger), serializable JSON schema, same schema in Designer preview + Labeler runtime. Advanced: **`visibleWhen`** (conditional display), **`customRule`** (safe `expr-eval` expression, no eval/Function), requiredWhen, regex/length/required, Group + multi-Tab. Designer (`modules/template/Designer.tsx`) 3-pane palette/canvas/inspector with **drag-to-place from palette** + reorder, config UI tabs 基础/校验/联动. (Engine + UI tested; end-to-end browser e2e of owner-config→labeler-effect NOT yet run — see Remaining.)
2. **4.3 Labeler 工作台** — `modules/labeler/` (Plaza + ItemNav/TaskPlaza/MyData). Task plaza search/filter/cards; answer page with item-nav (progress + colored status dots) backed by new `GET /tasks/:taskId/labeler/items`; prev/next/skip; 3s autosave (seq-guarded); submit validation; field-level LLM; 我的数据 stats+list. Ctrl/Cmd+Enter submit. (报告题目 = Toast stub, no backend endpoint.)
3. **4.1 Owner 后台** — `modules/owner/` (Dashboard + TaskManagePanel/TaskForm/ImportPanel/AssigneePanel). All task fields (title/desc/richDescription/tags/rewardConfig/deadline/quotaPerUser/distribution); task state machine **draft→published→paused→ended** (`POST /tasks/:id/publish|pause|resume|end`); import **JSON/JSONL/Excel** (`/tasks/:id/items/import-file`, excelize) + JSON paste + batch-update + item-preview; distribution **first_come/assigned/quota** enforced in `submission.Claim` + assignee management. Stat cards from real data.
4. **4.2 Reviewer + AI Agent (⭐⭐⭐) + 4.5 state machine** — AI auto pre-review: owner configures prompt+dimensions (`/tasks/:id/ai-prompts`, `/ai-review-settings`), submit→outbox→asynq→worker→**real Doubao** structured scoring→verdict; retry+idempotent. **Multi-level 初审/复审/终审** (3 levels), derived from `human_reviews` approve-count for the current revision (NO migration); approve advances one stage, 3rd → approved; reject/revise need reason. `reviewStage/reviewLevel/requiredLevels` exposed in queue/detail. **审核结果列表** `GET /reviewer/results`. `modules/reviewer/` (Queue + AIVerdictPanel/ReviewResults/stage.ts), ui-demo HumanReview/AiReview look, batch ops, audit timeline.
5. **Real demo data (live DB)** — 2 official tasks seeded: `qa_quality` (30 items) + `preference_compare` (12). On `qa_quality`: AI review enabled (Doubao); generated via real flow → **7 approved · 2 rejected · 3 in human_reviewing** at stages **初审(item10) / 复审(item11) / 终审(item12)**. Stats: pass-rate 78%, AI-vs-human 9 compared / 2 disagree, real dimension averages. Reviewer queue + Owner StatsBoard + Labeler progress all populated.

## What REMAINS (polish / follow-ups — none are core)
1. ✅ **DONE (2026-05-30, deployed+verified)** — **Owner legacy panels re-skinned**: scoped Editorial→lh token remap in `Dashboard.tsx` (AI Prompt/golden/dry-run/baseline/list) + `ExportPanel.tsx` + `StatsBoard.tsx` only (accent rust→blue `#165dff`, Georgia serif→system sans, warm→cool grays). Global `tokens.css` untouched (Editorial tokens still used by 22 other files). Browser-confirmed on prod.
2. ✅ **DONE (2026-05-30, deployed+verified)** — **Owner batch-edit list endpoint**: `GET /tasks/:taskId/items` (owner, id-asc cursor pagination) + `listTaskItems` client + `ImportPanel` now "加载题目列表" loads the full list with 加载更多. Live: `?limit=3` returns 3 + `page{next_cursor,has_more}`.
3. ✅ **DONE (2026-05-30, browser-e2e'd on prod)** — **Designer visibleWhen/customRule end-to-end**: owner Designer 联动/校验 tabs correctly round-trip the config; labeler runtime hides `reason` until `decision==reject`, and `customRule len(value)>=5` blocks a 2-char reason ("打回理由至少需要 5 个字符") / passes an 8-char one. (Verified via a throwaway task #3, now paused.)
4. ✅ **DONE (2026-05-30)** — **Submission docs password**: `submission/README.md` + `submission/DEMO_SCRIPT.md` `pass`→`123456` (kept AI-verdict `pass/reject/uncertain`); README account table `admin`→`admin1`.
5. ✅ **DONE (2026-05-30, deployed+verified)** — **`tasks.status` enum couldn't store `'ended'`** (pre-existing, surfaced during #3 cleanup; NOT from the re-skin batch): `migration/001:45` had `ENUM('draft','published','paused','archived')` but `statemachine/task.go` terminal state is `'ended'` → Owner "下线/end" failed with MySQL `Error 1265 Data truncated` for **every** task. Fixed via new migration `008_task_status_add_ended` (`ALTER TABLE tasks MODIFY status ENUM(...,'ended')`, archived kept for back-compat). Migrations run on api startup (golang-migrate, `file://internal/migration` baked into the image) → applied by `docker compose up -d --build api`. Verified: `POST /tasks/:id/end` → `ended`.
6. **报告题目** (labeler) is a Toast stub — add a backend report endpoint if wanted.
7. Optional hardening (single-host demo, acceptable as-is): pin image digests, domain + auto-HTTPS (Seoul is 免备案, just point a domain at the IP and set `CADDY_SITE_ADDRESS`).
8. ✅ **DONE** — throwaway verification task #3 fully removed from the prod DB (transactional `DELETE submissions WHERE task_id=3` → `DELETE tasks WHERE id=3`, children cascaded). Owner task list is back to the 2 official tasks. Note: there is still **no delete-task endpoint** — task removal requires a manual DB op until one is added.

## Gotchas / notes
- **Auto-test rule**: every subagent ran `pnpm -F web exec vitest run <module>` + `tsc -b` + `build` (+ `go build/vet/test` for backend) before returning; keep that discipline.
- **claim semantics**: a labeler holds ONE active claimed item; `claim` returns it until it's finalized (approve frees it). To generate multiple submissions you must interleave reviewer finalization, or use multiple labelers (labeler1/2/3).
- **Reviewer optimistic lock**: two approves fired within the same instant → 409 `ErrConcurrentWrite` ("reload"). Real UI clicks won't trigger it; it's correct concurrency protection.
- **Multi-level review** is per current revision; a revise resets the approve-count (stage back to 初审 after resubmission).
- Web test env is slow (~2-4 min full suite) and has a documented flaky timeout when the WHOLE suite runs together (Dashboard race) — run per-module to verify.
- Don't spawn long-lived dev servers; deploy is via rsync+docker as above.
- All deploys this session were rsync (NOT git). If you want git history, commit the working tree to a feature branch (currently uncommitted on `main`).

## Suggested next step
Highest evaluation impact: **(4) fix the `pass`→`123456` password in submission docs** (judges log in by docs) + **(3) Designer e2e browser verify** (confirm the ⭐ first key capability end-to-end). Then (1) Owner visual unification.

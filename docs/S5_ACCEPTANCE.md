# S5 Acceptance

Date: 2026-05-28

## Scope

S5 covers engineering quality only: critical test coverage, real MySQL/Redis integration testing, TypeScript strict mode, OpenAPI/Postman artifacts, ErrorBoundary, production deploy templates, and developer/deployment docs.

## Checklist

- [x] Backend critical-path tests cover state machine transitions, review apply, outbox publisher behavior, exporter retry/terminal failures, stats current-revision aggregation, and AI worker verdict decisions.
- [x] CI Go job includes `./pkg/exporter`.
- [x] Build-tagged testcontainers integration covers submit -> outbox/Redis -> AI result -> revise -> resubmit -> approve -> JSONL export against real MySQL and Redis.
- [x] Web `strict:true` is enabled.
- [x] Renderer and Designer round-trip tests cover ShowItem modes, LLMTrigger target writes, and saved schema payload -> parser -> renderer consistency.
- [x] OpenAPI main-flow fallback is tracked at `docs/openapi.yaml`.
- [x] `pnpm -F web gen:api` generates `apps/web/src/shared/api/schema.d.ts`.
- [x] Postman main-flow collection is tracked at `docs/LabelHub.postman_collection.json`.
- [x] App root is wrapped in `ErrorBoundary` with fallback/reset coverage.
- [x] Production Docker Compose template includes api, worker, web, mysql, redis, asynqmon, and caddy.
- [x] API and worker production services share an absolute export volume.
- [x] `make dev` starts local infra, installs dependencies, seeds, and prints long-running process commands.
- [x] `docs/ARCHITECTURE.md` and `docs/DEPLOY.md` are present.

## Coverage Snapshot

Latest targeted S5 coverage runs:

| Package | Coverage |
|---|---:|
| `apps/api/internal/statemachine` | 100.0% |
| `apps/api/internal/service/review` | 70.1% |
| `apps/api/internal/service/outbox` | 45.5% |
| `apps/api/internal/handler` | 61.4% |
| `pkg/exporter` | 87.8% |
| `apps/ai-worker/cmd/worker` | 60.6% |

The "100%" goal is interpreted as the PLAN.md critical paths, not every thin package in the repository.

## Verification Commands

Already run during S5:

```bash
go test ./apps/api/... ./apps/ai-worker/... ./pkg/exporter ./pkg/llmreview -count=1
DOCKER_HOST=unix:///Users/dadadineiyou/.colima/default/docker.sock TESTCONTAINERS_RYUK_DISABLED=true go test -tags=integration ./apps/api/internal/integration -count=1 -v
pnpm -F web gen:api
jq empty docs/LabelHub.postman_collection.json
pnpm -F web test -- ErrorBoundary SchemaRenderer Designer.integration
pnpm -F web lint
pnpm -F web build
```

Final deploy/doc slice verification:

```bash
docker compose --env-file deploy/.env.example -f deploy/docker-compose.prod.yml config
make -n dev
go test ./apps/api/... ./apps/ai-worker/... ./pkg/exporter ./pkg/llmreview -count=1
pnpm -F web test
pnpm -F web lint
pnpm -F web build
pnpm -F web gen:api
jq empty docs/LabelHub.postman_collection.json
git diff --check
```

## Accepted Fallback

The original S5 plan preferred full swaggo annotations. That was deliberately timeboxed. The accepted fallback is a tracked, hand-maintained OpenAPI document for the contest main flow plus generated TypeScript types and a Postman collection. This keeps the API contract usable without spending the sprint on broad handler annotations.

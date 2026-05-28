# LabelHub Deploy Guide

## Local Development

Create `.env` from `.env.example` and set `EXPORT_DIR` to an absolute path:

```bash
cp .env.example .env
# edit EXPORT_DIR=/absolute/path/to/LabelHub/data/exports
make dev
```

`make dev` starts MySQL/Redis/Adminer, installs dependencies, runs the seed command, then prints the long-running commands to start separately:

```bash
make api
make worker
make web
```

Local URLs:

- Web: `http://localhost:5173`
- API health: `http://localhost:8080/health`
- Adminer: `http://localhost:18080`
- asynqmon dev profile: `docker compose --profile ops -f deploy/docker-compose.yml up -d`

## Production Compose

Copy the production env template and replace every placeholder secret:

```bash
cp deploy/.env.example deploy/.env
$EDITOR deploy/.env
docker compose --env-file deploy/.env -f deploy/docker-compose.prod.yml config
docker compose --env-file deploy/.env -f deploy/docker-compose.prod.yml up -d --build
```

The production stack contains:

- `mysql`
- `redis`
- `api`
- `worker`
- `web`
- `asynqmon`
- `caddy`

Caddy reads `CADDY_SITE_ADDRESS` from `deploy/.env`. Use `:80` for local compose smoke tests, or a real domain such as `labelhub.example.com` for automatic HTTPS.

## Required Environment Variables

| Variable | Required | Notes |
|---|---:|---|
| `MYSQL_ROOT_PASSWORD` | yes | MySQL root password |
| `MYSQL_DATABASE` | yes | Defaults to `labelhub` in compose |
| `MYSQL_USER` | yes | App database user |
| `MYSQL_PASSWORD` | yes | App database password |
| `JWT_SECRET` | yes | Must be at least 32 chars; use 64+ random chars |
| `EXPORT_DOWNLOAD_SECRET` | yes | HMAC secret for signed export downloads |
| `CADDY_SITE_ADDRESS` | yes | `:80` locally or production domain |
| `API_CORS_ORIGINS` | yes | Browser origins allowed by API |
| `ASYNQMON_USER` | yes | Basic-auth user for the `/asynqmon/*` admin UI behind Caddy |
| `ASYNQMON_PASSWORD_HASH` | yes | bcrypt hash from `caddy hash-password`; the stack refuses to boot without it so asynqmon is never exposed unauthenticated |
| `LLM_PROVIDER` | yes | `mock` for smoke; real provider for production AI |
| `LLM_BASE_URL` / `LLM_API_KEY` / `LLM_MODEL` | real LLM only | Required for OpenAI-compatible providers; with a real provider `LLM_API_KEY` must be set or AI calls go out unauthenticated |

The compose file sets container-only runtime values for `DB_HOST`, `DB_PORT`, `REDIS_HOST`, `REDIS_PORT`, and absolute `EXPORT_DIR`.

The asynqmon admin UI (queue inspection / job control) is reachable at `/asynqmon/` and is gated by Caddy basic auth. Generate the hash with `docker run --rm caddy:2-alpine caddy hash-password --plaintext 'your-password'` and put it in `deploy/.env`.

## Data And Backups

Named Docker volumes hold state:

- `mysql_data`: relational data and migrations state.
- `redis_data`: Redis append-only file.
- `exports_data`: generated export files shared by API and worker.
- `caddy_data` / `caddy_config`: Caddy certificates and runtime config.

Minimum backup before deploy or migration:

```bash
docker compose --env-file deploy/.env -f deploy/docker-compose.prod.yml exec mysql \
  sh -c 'mysqldump -uroot -p"$MYSQL_ROOT_PASSWORD" "$MYSQL_DATABASE"' > labelhub-backup.sql
```

Rollback is image-level for API/worker/web plus database restore if a migration changed data. The API runs migrations on startup, so review migration diffs before deploying a new image.

## Verification

```bash
docker compose --env-file deploy/.env -f deploy/docker-compose.prod.yml ps
curl -f http://localhost/health
curl -f http://localhost/api/v1/
```

For a domain-backed deployment, replace `localhost` with the configured `CADDY_SITE_ADDRESS` host and use HTTPS.

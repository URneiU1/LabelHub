.PHONY: up down install dev api worker web seed

# 自动加载 .env(若存在);CI / prod 通过显式 env vars 注入
ENV_LOAD := if [ -f .env ]; then set -a; . ./.env; set +a; fi

up:
	docker compose -f deploy/docker-compose.yml up -d

down:
	docker compose -f deploy/docker-compose.yml down

install:
	cd apps/web && pnpm install
	cd apps/api && go mod tidy
	cd apps/ai-worker && go mod tidy

dev: up install
	@printf "Waiting for MySQL and Redis health checks...\n"
	sleep 5
	$(MAKE) seed
	@printf "\nDevelopment stack is ready.\n"
	@printf "Run long-lived processes in separate terminals:\n"
	@printf "  make api\n"
	@printf "  make worker\n"
	@printf "  make web\n\n"

api:
	$(ENV_LOAD); cd apps/api && go run ./cmd/server

# 用包路径 ./cmd/worker:worker 包是多文件,go run cmd/worker/main.go 只编单文件会编译失败。
worker:
	$(ENV_LOAD); cd apps/ai-worker && go run ./cmd/worker

web:
	cd apps/web && pnpm dev

seed:
	$(ENV_LOAD); cd apps/api && go run ./cmd/seed

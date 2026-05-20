.PHONY: up down install api worker web seed

up:
	docker compose -f deploy/docker-compose.yml up -d

down:
	docker compose -f deploy/docker-compose.yml down

install:
	cd apps/web && pnpm install
	cd apps/api && go mod tidy
	cd apps/ai-worker && go mod tidy

api:
	cd apps/api && go run cmd/server/main.go

worker:
	cd apps/ai-worker && go run cmd/worker/main.go

web:
	cd apps/web && pnpm dev

seed:
	@echo "Seed 脚本将在 Sprint 1 实现,当前请手动导入 tools/seed/datasets/"

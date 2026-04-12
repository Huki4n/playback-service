.PHONY: build run test lint swagger \
       docker-build docker-up docker-up-infra docker-up-tracing docker-up-metrics docker-up-logs docker-up-all \
       docker-down docker-logs \
       migrate-up migrate-down migrate-create

APP_NAME := service
BUILD_DIR := ./bin
MIGRATIONS_DIR := ./migrations
DB_DSN ?= postgres://postgres:postgres@localhost:5432/service?sslmode=disable

# ---- Build & Run (local) ----

build:
	go build -o $(BUILD_DIR)/$(APP_NAME) ./cmd/service

run:
	go run ./cmd/service

test:
	go test -race -count=1 ./...

lint:
	golangci-lint run ./...

swagger:
	swag init -g cmd/service/main.go -o docs --parseDependency --parseInternal

# ---- Docker ----

docker-build:
	docker compose build

# Service only — no infra
docker-up:
	docker compose up -d

# Service + PostgreSQL + Redis + Kafka
docker-up-infra:
	docker compose --profile infra up -d

# Service + Jaeger (tracing)
docker-up-tracing:
	docker compose --profile tracing up -d

# Service + Prometheus + Grafana (metrics)
docker-up-metrics:
	docker compose --profile metrics up -d

# Service + Loki + Promtail + Grafana (logs)
docker-up-logs:
	docker compose --profile logs up -d

# Full observability stack + infra
docker-up-all:
	docker compose --profile infra --profile tracing --profile metrics --profile logs up -d

docker-down:
	docker compose --profile infra --profile tracing --profile metrics --profile logs down

docker-logs:
	docker compose logs -f service

# ---- Migrations ----

migrate-up:
	migrate -path $(MIGRATIONS_DIR) -database "$(DB_DSN)" up

migrate-down:
	migrate -path $(MIGRATIONS_DIR) -database "$(DB_DSN)" down 1

migrate-create:
	@read -p "Migration name: " name; \
	migrate create -ext sql -dir $(MIGRATIONS_DIR) -seq $$name

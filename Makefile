DATABASE_URL ?= postgres://termtext:termtext_dev@localhost:5432/termtext?sslmode=disable

build-server:
	@echo "Building server..."
	@go build -o ./bin/main ./cmd/server

build-client:
	@echo "Building client..."
	@go build -o ./bin/termtext ./cmd/termtext

run: build-server
	@./bin/main

test:
	go test -race ./...

vet:
	go vet ./...

docker-build:
	docker build -t termtext:latest .

db-up:
	docker compose up -d postgres

db-down:
	docker compose down

db-psql:
	docker compose exec postgres psql -U termtext -d termtext

migrate-up:
	goose -dir ./internal/db/schema postgres "$(DATABASE_URL)" up

migrate-down:
	goose -dir ./internal/db/schema postgres "$(DATABASE_URL)" down

migrate-status:
	goose -dir ./internal/db/schema postgres "$(DATABASE_URL)" status

sqlc-gen:
	sqlc generate

.PHONY: build-server build-client run test vet docker-build db-up db-down db-psql migrate-up migrate-down migrate-status sqlc-gen

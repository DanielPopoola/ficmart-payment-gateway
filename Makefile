.PHONY: up down shell test lint migrate-create

up:
	@cd docker && docker compose up -d --build

down:
	@cd docker && docker compose down

logs:
	@cd docker && docker compose logs gateway

restart:
	@cd docker && docker compose restart gateway

shell:
	@cd docker && docker compose exec gateway sh

test:
	@cd docker && docker compose exec gateway sh -c "go test -v \$$(go list ./... | grep -v /tests) && go test -v -count=1 -p 1 ./tests/..."

test-cover:
	@cd docker && docker compose exec gateway go test -v -cover ./...

lint:
	@cd docker && docker compose exec gateway golangci-lint run

fmt:
	@cd docker && docker compose exec gateway gofmt -w .

build:
	@cd docker && docker compose exec gateway go build -o tmp/main ./cmd/gateway

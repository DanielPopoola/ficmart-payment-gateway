.PHONY: up down shell test lint migrate-create

up:
	docker compose -f docker/docker-compose.yml up --build

down:
	docker compose -f docker/docker-compose.yml down

# Run commands inside the running gateway container
shell:
	docker compose -f docker/docker-compose.yml exec gateway sh

test:
	docker compose -f docker/docker-compose.yml exec gateway go test ./...

lint:
	docker compose -f docker/docker-compose.yml exec gateway golangci-lint run ./...

build:
	docker compose -f docker/docker-compose.yml exec gateway go build -o tmp/main ./cmd/gateway
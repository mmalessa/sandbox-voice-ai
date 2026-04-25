.PHONY: build up down tidy

build:
	docker compose build

up:
	docker compose up

down:
	docker compose down

tidy:
	docker run --rm -v "$(PWD)":/app -w /app golang:1.23-alpine go mod tidy

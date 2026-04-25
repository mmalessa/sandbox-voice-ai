DC = docker compose

.DEFAULT_GOAL = help

help: ## Outputs this help screen
	@grep -E '(^[a-zA-Z0-9_-]+:.*?##.*$$)|(^##)' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}{printf "\033[32m%-30s\033[0m %s\n", $$1, $$2}' | sed -e 's/\[32m##/[33m/'


.PHONY: build
build:
	docker compose build

.PHONY: init
init:
	docker compose run --rm --no-deps --entrypoint python3 server \
		-c "import os; from faster_whisper import WhisperModel; \
		    WhisperModel(os.environ.get('WHISPER_MODEL', 'tiny'), device='cpu', compute_type='int8'); \
		    print('model ready')"

.PHONY: up
up:
	$(DC) up -d --remove-orphans --force-recreate
	$(DC) logs -f server

.PHONY: down
down:
	docker compose down

.PHONY: logs
logs:
	docker compose logs -f server

.PHONY: tidy
tidy:
	docker run --rm -v "$(PWD)":/app -w /app golang:1.23-alpine go mod tidy

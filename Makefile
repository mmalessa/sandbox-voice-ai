DC = docker compose

.DEFAULT_GOAL = help

help: ## Outputs this help screen
	@grep -E '(^[a-zA-Z0-9_-]+:.*?##.*$$)|(^##)' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}{printf "\033[32m%-30s\033[0m %s\n", $$1, $$2}' | sed -e 's/\[32m##/[33m/'

.PHONY: build
build:
	$(DC) build

.PHONY: init
init:
	$(DC) up -d ollama
	$(DC) run --rm --no-deps -e HF_HUB_OFFLINE=0 --entrypoint python3 server \
		-c "import os; from faster_whisper import WhisperModel; \
		    WhisperModel(os.environ.get('WHISPER_MODEL', 'tiny'), device='cpu', compute_type='int8'); \
		    print('whisper model ready')"
	$(DC) run --rm --no-deps --entrypoint python3 server \
		-c "import urllib.request, os; \
		    os.makedirs('/app/voices', exist_ok=True); \
		    base='https://huggingface.co/rhasspy/piper-voices/resolve/v1.0.0/pl/pl_PL/darkman/medium/'; \
		    [urllib.request.urlretrieve(base+f, '/app/voices/'+f) or print(f+' ready') \
		     for f in ['pl_PL-darkman-medium.onnx', 'pl_PL-darkman-medium.onnx.json'] \
		     if not os.path.exists('/app/voices/'+f)]; \
		    print('piper voice ready')"
	$(DC) exec ollama bash /scripts/fetch-models.sh

.PHONY: up
up:
	$(DC) up -d --remove-orphans --force-recreate

.PHONY: down
down:
	$(DC) down

.PHONY: logs
logs:
	$(DC) logs -f server

.PHONY: tidy
tidy:
	docker run --rm -v "$(PWD)":/app -w /app golang:1.23-alpine go mod tidy

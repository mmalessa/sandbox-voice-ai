# Local Voice AI MVP

Minimal local voice agent:

1. Browser records one audio chunk.
2. Browser sends the chunk over WebSocket.
3. Go orchestrator runs `STT -> LLM -> TTS` sequentially.
4. Go sends synthesized WAV audio back over WebSocket.

## Project layout

```text
cmd/server/main.go          # process entrypoint
internal/app/               # config and HTTP/WebSocket server
internal/orchestrator/      # sequential pipeline and interfaces
internal/stt/               # STT adapter (CLI + mock)
internal/llm/               # Ollama streaming client
internal/tts/               # Piper adapter (CLI + mock)
web/index.html              # manual browser test page
```

## Interface contract

The Go backend keeps only three interfaces:

- `STT.Transcribe(ctx, audio) -> text`
- `LLM.Generate(ctx, prompt) -> text`
- `TTS.Synthesize(ctx, text) -> wav bytes`

## CLI expectations

### STT

`STT_COMMAND` must point to a **long-running process** that:

- stays alive for the lifetime of the server
- reads requests in a loop: `[4-byte big-endian uint32: audio length][audio bytes]`
- writes one response per request: `[transcript text]\n`
- writes logs/errors to `stderr`

The model is loaded once at startup; each request pays only inference cost.
`stt/wrapper.py` (faster-whisper) is the reference implementation.

### TTS

`PIPER_COMMAND` must point to a program that:

- reads plain text from `stdin`
- writes raw 16-bit PCM audio to `stdout`
- writes logs/errors to `stderr`

The Go backend wraps the raw PCM bytes into a WAV container before returning them to the browser.

For Piper, a typical command is:

```bash
piper --model /path/to/model.onnx --output-raw
```

## Environment

All configuration lives in `compose.yaml`. Key variables:

| Variable | Default in compose | Description |
|---|---|---|
| `LISTEN_ADDR` | `:8080` | HTTP listen address |
| `WHISPER_MODEL` | `tiny` | faster-whisper model size |
| `WHISPER_LANGUAGE` | `pl` | Force language, skip detection |
| `STT_COMMAND` | `python3 /app/stt/wrapper.py` | Long-running STT process |
| `OLLAMA_URL` | `http://ollama:11434` | Ollama endpoint (service within compose) |
| `OLLAMA_MODEL` | `llama3.1:8b` | Model name |
| `PIPER_COMMAND` | `python3 -m piper --model ... --output-raw` | TTS command (raw PCM to stdout) |
| `PIPER_SAMPLE_RATE` | `22050` | Must match the voice model |
| `MOCK_STT` | _(unset)_ | Set `true` to bypass real STT |
| `MOCK_TTS` | _(unset)_ | Set `true` to bypass real TTS |

## Run

```bash
# 1. Build the Docker image
make build

# 2. Download all models into Docker volumes (once, requires internet)
#    Downloads: Whisper STT, Piper TTS voice, Ollama LLM
make init

# 3. Start and follow logs
make up
```

Open `http://localhost:8080`, click Connect, then Record.

```bash
make down          # stop
make logs          # re-attach to logs without restarting
make fetch-models  # re-pull Ollama models (ollama must be running)
```

## Notes

- This is intentionally non-streaming end to end.
- One WebSocket binary message maps to one full `STT -> LLM -> TTS` turn.
- Browser audio format is currently whatever `MediaRecorder` emits, typically `audio/webm`.
- If your STT tool requires WAV instead of WebM, add a thin wrapper script that converts stdin before calling faster-whisper.

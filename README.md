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

`STT_COMMAND` must point to a program that:

- reads the full audio chunk from `stdin`
- writes only the final transcript to `stdout`
- writes logs/errors to `stderr`

Example wrapper contract:

```bash
cat audio.webm | ./bin/stt-wrapper
```

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

```bash
export LISTEN_ADDR=:8080
export OLLAMA_URL=http://127.0.0.1:11434
export OLLAMA_MODEL=llama3.1:8b

# Use mocks for bring-up
export MOCK_STT=true
export MOCK_TTS=true

# Or use real local CLIs
export STT_COMMAND="/absolute/path/to/stt-wrapper"
export PIPER_COMMAND="piper --model /absolute/path/to/model.onnx --output-raw"
export PIPER_SAMPLE_RATE=22050
export PIPER_CHANNELS=1
export PIPER_BITS_PER_SAMPLE=16
```

## Run

```bash
go mod tidy
go run ./cmd/server
```

Open `http://127.0.0.1:8080`, connect, then record.

## Notes

- This is intentionally non-streaming end to end.
- One WebSocket binary message maps to one full `STT -> LLM -> TTS` turn.
- Browser audio format is currently whatever `MediaRecorder` emits, typically `audio/webm`.
- If your STT tool requires WAV instead of WebM, add a thin wrapper script that converts stdin before calling faster-whisper.

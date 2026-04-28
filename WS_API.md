# WebSocket API Contract

## Connection

```
ws://<host>:8080/ws
```

The server handles **one session at a time** — a second concurrent connection receives HTTP 503. The session maintains conversation history for its entire lifetime (up to the last 10 turns).

---

## Client → Server Messages

### 1. Audio chunk (Binary frame)

```
[raw WAV bytes — cumulative per turn]
```

- Frame type: **binary**
- Content: a complete WAV blob that is **cumulative** — each successive chunk must contain the **full** recording from the start of the current turn, not just the delta
- The server runs STT on every chunk; more data yields better transcription
- Limit: 10 MB by default (`WS_READ_LIMIT_BYTES`)

### 2. End of speech (Text frame)

```json
{ "type": "vad_end" }
```

- Signals that the user has finished speaking
- The server flushes the remaining transcript, finalises STT, then drives the LLM + TTS pipeline
- Do not send further audio for this turn after `vad_end` — wait for `done`

### 3. Interrupt (Text frame)

```json
{ "type": "interrupt" }
```

- The user started speaking while the AI was still responding
- The server immediately cancels the active LLM + TTS pipeline and begins a new turn
- Audio for the new turn may be sent immediately after `interrupt`

---

## Server → Client Messages

### 1. Partial transcript (Text frame)

```json
{ "type": "transcript", "text": "What is the capital..." }
```

- Sent after each audio chunk — live STT feedback
- Optional to handle; may be ignored on resource-constrained clients

### 2. TTS audio chunk (Binary frame)

```
[WAV file bytes]
```

- Frame type: **binary**
- One complete WAV file per sentence (not streaming PCM) — the server synthesises sentence by sentence
- Default format: **PCM 16-bit, little-endian, mono, 22050 Hz** (configurable via env vars)
- Chunks arrive **in order** — play them sequentially
- Chunks may arrive **before** the server sends `done` (pipeline is parallel)

### 3. End of response (Text frame)

```json
{ "type": "done" }
```

- The server has finished generating the full response
- The client may now start a new turn — begin recording and send audio

### 4. Error (Close control frame)

```
WebSocket Close frame, code 1011 (Internal Error), reason: <error message>
```

- The server closed the connection due to a pipeline error
- The client should reconnect

---

## Single-turn Flow

```
Client                          Server
  |                               |
  |-- binary: WAV[0..N] -------> |  (STT partial)
  |<- text: {transcript} --------|
  |-- binary: WAV[0..M] -------> |  (STT partial, M > N — cumulative)
  |<- text: {transcript} --------|
  |-- text: {vad_end} ---------> |
  |                               |  (flush STT, LLM stream, TTS per sentence)
  |<- binary: WAV chunk 1 -------|
  |<- binary: WAV chunk 2 -------|
  |<- text: {done} --------------|
  |                               |
  |-- (new turn: send audio) ----|
```

## Interrupt Flow

```
Client                          Server
  |                               |
  |<- binary: WAV chunk 1 -------|  (AI speaking)
  |-- text: {interrupt} -------> |  (user starts speaking)
  |                               |  (server cancels pipeline)
  |-- binary: WAV[0..N] -------> |  (new turn begins immediately)
  ...
```

---

## REST Endpoints

| Endpoint  | Method | Description |
|-----------|--------|-------------|
| `/healthz` | GET | `{"status":"ok"}` |
| `/config`  | GET | `{"vad_silence_ms": 2000}` — client-side VAD parameters |

`/config` is useful for clients implementing VAD: it returns `vad_silence_ms`, the silence duration (in milliseconds) after which the client should send `vad_end`.

---

## Notes for Embedded Clients (e.g. ESP32)

1. **Cumulative audio** — the most important detail. Every binary frame sent to the server must contain the **entire** recording since the start of the current turn (a growing buffer), not just new samples.
2. **Client-side VAD** — the client decides when to send `vad_end`. Use the threshold from `GET /config`.
3. **Sequential playback** — maintain a queue of incoming WAV chunks; each chunk is a self-contained WAV file, play them in order.
4. **Session scope** — conversation history is tied to the WebSocket connection. If the client disconnects and reconnects, history is lost.

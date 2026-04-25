# Local Voice AI Agent (STT → LLM → TTS)

## Goal

Build a local, low-latency voice AI agent running in the browser that:

- captures speech from the user (audio input)
- transcribes it to text (STT)
- generates a response (LLM)
- converts the response to speech (TTS)
- plays the audio back to the user

---

# System Architecture

## Components

### 1. Frontend (Browser)
- captures audio from the microphone
- sends audio to the backend (WebSocket)
- receives audio (TTS) and plays it back

### 2. Backend (Go Orchestrator)
- central component of the system
- manages data flow
- integrates STT, LLM and TTS
- controls conversation logic and state

### 3. STT (Speech-to-Text)
- faster-whisper (CTranslate2)
- runs locally (CLI / subprocess)
- converts audio → text

### 4. LLM
- Ollama
- model: llama3.1:8b
- communication via HTTP API (streaming optional initially)

### 5. TTS (Text-to-Speech)
- Piper TTS
- runs locally (CLI / subprocess)
- converts text → audio

---

# Communication Between Components

## Browser ↔ Backend
- protocol: WebSocket
- bidirectional (full duplex)
- data:
    - input: audio (binary chunks)
    - output: audio (binary chunks)

## Backend ↔ STT / TTS
- subprocess (stdin / stdout)
- no network (low overhead)

## Backend ↔ LLM (Ollama)
- HTTP API
- streaming (target), request/response acceptable for MVP

---

# Stage 1 — MVP (simple, synchronous)

## Goal
Get a working pipeline without streaming or parallelism.

## Flow

1. Browser records audio (2–3 seconds)
2. Browser sends audio to backend (WebSocket)
3. Backend:
    - passes audio to STT
    - receives full transcript
4. Backend sends text to LLM (Ollama)
5. Backend receives full response
6. Backend passes text to TTS (Piper)
7. Backend receives audio
8. Backend sends audio to browser
9. Browser plays audio

## Characteristics

- no streaming
- no parallelism
- full step-by-step processing
- simplicity over performance

---

# Stage 2 — Pipeline Optimisation

## Goals

- reduce latency
- improve UX

## Changes

### 1. STT
- shorter audio segments (e.g. 1 second)
- introduce VAD (voice activity detection)

### 2. LLM
- enable streaming response
- process tokens as they arrive

### 3. TTS
- generate audio per complete sentence (still batch)
- optimise generation time

---

# Stage 3 — Pseudo Real-Time (partial streaming)

## Flow

1. STT generates partial results
2. on sentence-end detection → LLM
3. LLM streams tokens
4. TTS generates audio after response completes
5. audio sent to browser

## Characteristics

- partial streaming
- still no full overlap

---

# Stage 4 — Real-Time Streaming Pipeline

## Target flow

1. audio stream from browser
2. STT (streaming / sliding window)
3. LLM (token streaming)
4. TTS (audio streaming)
5. immediate playback

## Pipeline

audio → text → token → audio → playback

## Characteristics

- components run in parallel
- minimal latency
- no waiting for full responses

---

# Stage 5 — Agent Behaviour

## Extensions

- decision layer (LLM → actions)
- command handling (e.g. JSON output)
- integrations (API, local system)
- conversation context (memory)

---

# Backend (Go Orchestrator)

## Responsibilities

- WebSocket management
- audio buffering
- invoking STT / LLM / TTS
- flow control
- conversation state management

## Interfaces (concept)

- STT: audio → text
- LLM: text → text
- TTS: text → audio

---

# Design Principles

- MVP first, then optimise
- no microservices initially
- no message queues (Kafka, etc.)
- minimal dependencies
- full control in the backend (Go)

---

# Build Strategy

1. Implement MVP (full flow, synchronous)
2. Stabilise communication (WebSocket)
3. Optimise STT and LLM
4. Add LLM streaming
5. Add TTS streaming
6. Build full real-time pipeline
7. Extend with agent features

---

# End Goal

A system that:

- runs locally (offline-first)
- has low latency
- holds real-time conversations
- can execute actions
- runs in the browser

package app

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	ListenAddr string

	ReadLimitBytes int64
	RequestTimeout time.Duration

	EnableMockSTT bool
	EnableMockTTS bool

	STTCommand []string

	OllamaURL   string
	OllamaModel string
	OllamaPrompt string

	PiperCommand []string
	PiperSampleRate    int
	PiperChannels      int
	PiperBitsPerSample int
	VADSilenceMS       int
}

func LoadConfig() Config {
	return Config{
		ListenAddr:         envString("LISTEN_ADDR", ":8080"),
		ReadLimitBytes:     envInt64("WS_READ_LIMIT_BYTES", 10*1024*1024),
		RequestTimeout:     envDuration("REQUEST_TIMEOUT", 90*time.Second),
		EnableMockSTT:      envBool("MOCK_STT", false),
		EnableMockTTS:      envBool("MOCK_TTS", false),
		STTCommand:         splitCommand(os.Getenv("STT_COMMAND")),
		OllamaURL:          envString("OLLAMA_URL", "http://127.0.0.1:11434"),
		OllamaModel:        envString("OLLAMA_MODEL", "llama3.1:8b"),
		OllamaPrompt:       envString("OLLAMA_SYSTEM_PROMPT", "You are a concise voice assistant. Answer clearly and briefly."),
		PiperCommand:       splitCommand(os.Getenv("PIPER_COMMAND")),
		PiperSampleRate:    envInt("PIPER_SAMPLE_RATE", 22050),
		PiperChannels:      envInt("PIPER_CHANNELS", 1),
		PiperBitsPerSample: envInt("PIPER_BITS_PER_SAMPLE", 16),
		VADSilenceMS:       envInt("VAD_SILENCE_MS", 2000),
	}
}

func envString(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}

	parsed, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return parsed
}

func envInt(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}

	parsed, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return parsed
}

func envInt64(key string, fallback int64) int64 {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}

	parsed, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func envDuration(key string, fallback time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}

	parsed, err := time.ParseDuration(v)
	if err != nil {
		return fallback
	}
	return parsed
}

func splitCommand(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	return strings.Fields(raw)
}

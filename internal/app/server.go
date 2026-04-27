package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"sandbox-voice-ai/internal/llm"
	"sandbox-voice-ai/internal/orchestrator"
	"sandbox-voice-ai/internal/stt"
	"sandbox-voice-ai/internal/tts"
)

type Server struct {
	httpServer *http.Server
	upgrader   websocket.Upgrader
	orch       *orchestrator.Service
	cfg        Config
	sessionMu  sync.Mutex
}

func NewServer(cfg Config) (*Server, error) {
	sttClient, err := buildSTT(cfg)
	if err != nil {
		return nil, fmt.Errorf("build stt: %w", err)
	}
	ttsClient := buildTTS(cfg)
	llmClient := llm.NewOllamaClient(cfg.OllamaURL, cfg.OllamaModel, cfg.OllamaPrompt)

	orch := orchestrator.NewService(sttClient, llmClient, ttsClient)
	s := &Server{
		orch: orch,
		cfg:  cfg,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/ws", s.handleWS)
	mux.HandleFunc("/config", s.handleConfig)
	mux.HandleFunc("/healthz", s.handleHealth)
	mux.HandleFunc("/", s.handleIndex)

	s.httpServer = &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	return s, nil
}

func (s *Server) ListenAndServe() error {
	return s.httpServer.ListenAndServe()
}

func (s *Server) handleConfig(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"vad_silence_ms": s.cfg.VADSilenceMS,
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, "web/index.html")
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	if !s.sessionMu.TryLock() {
		log.Printf("ws rejected: session already active (%s)", r.RemoteAddr)
		http.Error(w, "session busy", http.StatusServiceUnavailable)
		return
	}
	defer s.sessionMu.Unlock()

	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("websocket upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	log.Printf("ws connected: %s", r.RemoteAddr)
	defer log.Printf("ws disconnected: %s", r.RemoteAddr)

	conn.SetReadLimit(s.cfg.ReadLimitBytes)

	// gorilla/websocket requires one concurrent writer.
	var writeMu sync.Mutex
	write := func(msgType int, data []byte) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return conn.WriteMessage(msgType, data)
	}

	sess := s.orch.NewSession()

	for { // outer loop: one iteration per conversation turn
		sess.Reset()

		sentenceCh := make(chan string, 16)
		pipeErrCh := make(chan error, 1)

		// LLM+TTS goroutine: serialises synthesis so audio chunks arrive in order.
		// It starts as soon as a sentence is detected — before vad_end arrives.
		go func() {
			var firstErr error
			for sentence := range sentenceCh {
				if firstErr != nil {
					continue // drain after first error
				}
				synthCtx, cancel := context.WithTimeout(r.Context(), s.cfg.RequestTimeout)
				firstErr = s.orch.Synthesize(synthCtx, sentence, func(chunk []byte) error {
					return write(websocket.BinaryMessage, chunk)
				})
				cancel()
				if firstErr != nil {
					log.Printf("synthesize: %v", firstErr)
				}
			}
			pipeErrCh <- firstErr
		}()

		var readErr error

	readLoop:
		for {
			msgType, payload, err := conn.ReadMessage()
			if err != nil {
				readErr = err
				break readLoop
			}

			switch msgType {
			case websocket.BinaryMessage:
				// Cumulative audio blob — run STT, detect new complete sentences.
				sttCtx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
				addErr := sess.AddAudio(sttCtx, payload, func(sentence string) {
					sentenceCh <- sentence
				})
				cancel()
				if addErr != nil {
					log.Printf("stt: %v", addErr)
				}
				// Forward partial transcript to browser for display.
				if partial := sess.Transcript(); partial != "" {
					if msg, err := json.Marshal(map[string]string{"type": "transcript", "text": partial}); err == nil {
						_ = write(websocket.TextMessage, msg)
					}
				}

			case websocket.TextMessage:
				var ctrl struct {
					Type string `json:"type"`
				}
				if json.Unmarshal(payload, &ctrl) == nil && ctrl.Type == "vad_end" {
					// User stopped speaking — flush remaining transcript fragment.
					sess.Flush(func(sentence string) { sentenceCh <- sentence })
					break readLoop
				}
			}
		}

		close(sentenceCh)
		pipeErr := <-pipeErrCh

		if readErr != nil {
			if !websocket.IsCloseError(readErr, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				log.Printf("websocket read: %v", readErr)
			}
			return
		}

		if pipeErr != nil {
			log.Printf("pipeline: %v", pipeErr)
			closeMsg := websocket.FormatCloseMessage(websocket.CloseInternalServerErr, pipeErr.Error())
			_ = conn.WriteControl(websocket.CloseMessage, closeMsg, time.Now().Add(2*time.Second))
			return
		}

		_ = write(websocket.TextMessage, []byte(`{"type":"done"}`))
	}
}

func buildSTT(cfg Config) (orchestrator.STT, error) {
	if cfg.EnableMockSTT || len(cfg.STTCommand) == 0 {
		return stt.Mock{Transcript: "mock transcript"}, nil
	}
	return stt.NewProcess(cfg.STTCommand)
}

func buildTTS(cfg Config) orchestrator.TTS {
	if cfg.EnableMockTTS || len(cfg.PiperCommand) == 0 {
		return tts.Mock{
			SampleRate:    cfg.PiperSampleRate,
			Channels:      cfg.PiperChannels,
			BitsPerSample: cfg.PiperBitsPerSample,
		}
	}

	return tts.Piper{
		Command:       cfg.PiperCommand,
		SampleRate:    cfg.PiperSampleRate,
		Channels:      cfg.PiperChannels,
		BitsPerSample: cfg.PiperBitsPerSample,
	}
}

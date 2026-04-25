package app

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
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
}

func NewServer(cfg Config) (*Server, error) {
	sttClient := buildSTT(cfg)
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
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("websocket upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	conn.SetReadLimit(s.cfg.ReadLimitBytes)

	for {
		msgType, payload, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				return
			}
			log.Printf("websocket read failed: %v", err)
			return
		}

		if msgType != websocket.BinaryMessage {
			_ = conn.WriteMessage(websocket.TextMessage, []byte("binary audio message required"))
			continue
		}

		ctx, cancel := context.WithTimeout(r.Context(), s.cfg.RequestTimeout)
		result, procErr := s.orch.Process(ctx, payload)
		cancel()

		if procErr != nil {
			log.Printf("pipeline failed: %v", procErr)
			closeMsg := websocket.FormatCloseMessage(websocket.CloseInternalServerErr, procErr.Error())
			_ = conn.WriteControl(websocket.CloseMessage, closeMsg, time.Now().Add(2*time.Second))
			return
		}

		if err := conn.WriteMessage(websocket.BinaryMessage, result.Audio); err != nil {
			log.Printf("websocket write failed: %v", err)
			return
		}
	}
}

func buildSTT(cfg Config) orchestrator.STT {
	if cfg.EnableMockSTT || len(cfg.STTCommand) == 0 {
		return stt.Mock{Transcript: "mock transcript"}
	}
	return stt.CLI{Command: cfg.STTCommand}
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
		Command:        cfg.PiperCommand,
		SampleRate:     cfg.PiperSampleRate,
		Channels:       cfg.PiperChannels,
		BitsPerSample:  cfg.PiperBitsPerSample,
	}
}

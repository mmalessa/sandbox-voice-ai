package orchestrator

import (
	"context"
	"fmt"
	"strings"
)

type STT interface {
	Transcribe(ctx context.Context, audio []byte) (string, error)
}

type LLM interface {
	Generate(ctx context.Context, prompt string) (string, error)
}

type TTS interface {
	Synthesize(ctx context.Context, text string) ([]byte, error)
}

type Result struct {
	Transcript string
	Response   string
	Audio      []byte
}

type Service struct {
	stt STT
	llm LLM
	tts TTS
}

func NewService(stt STT, llm LLM, tts TTS) *Service {
	return &Service{
		stt: stt,
		llm: llm,
		tts: tts,
	}
}

func (s *Service) Process(ctx context.Context, audio []byte) (*Result, error) {
	transcript, err := s.stt.Transcribe(ctx, audio)
	if err != nil {
		return nil, fmt.Errorf("stt: %w", err)
	}

	transcript = strings.TrimSpace(transcript)
	if transcript == "" {
		return nil, fmt.Errorf("stt: empty transcript")
	}

	reply, err := s.llm.Generate(ctx, transcript)
	if err != nil {
		return nil, fmt.Errorf("llm: %w", err)
	}

	reply = strings.TrimSpace(reply)
	if reply == "" {
		return nil, fmt.Errorf("llm: empty response")
	}

	speech, err := s.tts.Synthesize(ctx, reply)
	if err != nil {
		return nil, fmt.Errorf("tts: %w", err)
	}

	if len(speech) == 0 {
		return nil, fmt.Errorf("tts: empty audio")
	}

	return &Result{
		Transcript: transcript,
		Response:   reply,
		Audio:      speech,
	}, nil
}

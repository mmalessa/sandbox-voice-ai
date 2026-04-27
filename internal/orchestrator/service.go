package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
)

var ErrEmptyTranscript = errors.New("empty transcript")

type STT interface {
	Transcribe(ctx context.Context, audio []byte) (string, error)
}

type LLM interface {
	GenerateStream(ctx context.Context, prompt string, onToken func(string) error) error
}

type TTS interface {
	Synthesize(ctx context.Context, text string) ([]byte, error)
}

type Result struct {
	Transcript string
	Response   string
}

type Service struct {
	stt STT
	llm LLM
	tts TTS
}

func NewService(stt STT, llm LLM, tts TTS) *Service {
	return &Service{stt: stt, llm: llm, tts: tts}
}

// Process transcribes audio, streams an LLM response split into sentences, synthesizes each
// sentence with TTS and delivers the WAV chunk via onAudio. This allows playback to begin
// before the LLM has finished generating.
func (s *Service) Process(ctx context.Context, audio []byte, onAudio func([]byte) error) (*Result, error) {
	transcript, err := s.stt.Transcribe(ctx, audio)
	if err != nil {
		return nil, fmt.Errorf("stt: %w", err)
	}

	transcript = strings.TrimSpace(transcript)
	if transcript == "" {
		return nil, ErrEmptyTranscript
	}

	var fullReply strings.Builder
	var buf strings.Builder

	synth := func(text string) error {
		text = strings.TrimSpace(text)
		if text == "" {
			return nil
		}
		log.Printf("tts sentence: %q", text)
		wav, err := s.tts.Synthesize(ctx, text)
		if err != nil {
			return fmt.Errorf("tts: %w", err)
		}
		return onAudio(wav)
	}

	err = s.llm.GenerateStream(ctx, transcript, func(token string) error {
		fullReply.WriteString(token)
		buf.WriteString(token)

		sentences, remainder := splitSentences(buf.String())
		buf.Reset()
		buf.WriteString(remainder)

		for _, sentence := range sentences {
			if err := synth(sentence); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("llm: %w", err)
	}

	if err := synth(buf.String()); err != nil {
		return nil, err
	}

	reply := strings.TrimSpace(fullReply.String())
	if reply == "" {
		return nil, fmt.Errorf("llm: empty response")
	}

	log.Printf("llm reply: %q", reply)
	return &Result{Transcript: transcript, Response: reply}, nil
}

// splitSentences splits text on sentence boundaries (.!?\n) and returns complete sentences
// and the remaining incomplete fragment. Ellipsis (...) is not treated as a boundary.
func splitSentences(text string) (sentences []string, remainder string) {
	runes := []rune(text)
	n := len(runes)
	start := 0

	for i := 0; i < n; i++ {
		c := runes[i]

		isBoundary := false
		switch c {
		case '\n':
			isBoundary = true
		case '.', '!', '?':
			if c == '.' && ((i > 0 && runes[i-1] == '.') || (i+1 < n && runes[i+1] == '.')) {
				continue
			}
			if i+1 >= n || runes[i+1] == ' ' || runes[i+1] == '\t' || runes[i+1] == '\n' || runes[i+1] == '\r' {
				isBoundary = true
			}
		}

		if !isBoundary {
			continue
		}

		sentence := strings.TrimSpace(string(runes[start : i+1]))
		if sentence != "" {
			sentences = append(sentences, sentence)
		}

		j := i + 1
		for j < n && (runes[j] == ' ' || runes[j] == '\n' || runes[j] == '\t' || runes[j] == '\r') {
			j++
		}
		start = j
		i = j - 1
	}

	remainder = strings.TrimSpace(string(runes[start:]))
	return
}

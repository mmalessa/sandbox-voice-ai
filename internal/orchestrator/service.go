package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
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

// NewSession creates a streaming STT session backed by this service's STT client.
func (s *Service) NewSession() *Session {
	return &Session{stt: s.stt}
}

// Synthesize streams an LLM response for the given user text, splits the response
// into sentences, synthesizes each with TTS and delivers WAV chunks via onAudio.
func (s *Service) Synthesize(ctx context.Context, text string, onAudio func([]byte) error) error {
	log.Printf("llm prompt: %q", text)
	var buf strings.Builder

	synth := func(sentence string) error {
		sentence = strings.TrimSpace(sentence)
		if sentence == "" {
			return nil
		}
		log.Printf("tts sentence: %q", sentence)
		wav, err := s.tts.Synthesize(ctx, sentence)
		if err != nil {
			return fmt.Errorf("tts: %w", err)
		}
		return onAudio(wav)
	}

	err := s.llm.GenerateStream(ctx, text, func(token string) error {
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
		return fmt.Errorf("llm: %w", err)
	}
	return synth(buf.String())
}

// Process is the one-shot pipeline: STT → Synthesize (LLM streaming + TTS per sentence).
func (s *Service) Process(ctx context.Context, audio []byte, onAudio func([]byte) error) (*Result, error) {
	transcript, err := s.stt.Transcribe(ctx, audio)
	if err != nil {
		return nil, fmt.Errorf("stt: %w", err)
	}
	transcript = strings.TrimSpace(transcript)
	if transcript == "" {
		return nil, ErrEmptyTranscript
	}
	if err := s.Synthesize(ctx, transcript, onAudio); err != nil {
		return nil, err
	}
	return &Result{Transcript: transcript}, nil
}

// Session accumulates STT results across growing audio chunks and surfaces
// complete sentences as soon as they are detected — before the user stops speaking.
type Session struct {
	stt STT

	mu             sync.Mutex
	prevTranscript string
	sentenceCount  int // sentences from prevTranscript already delivered
}

// Reset clears accumulated state for the start of a new turn.
func (s *Session) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prevTranscript = ""
	s.sentenceCount = 0
}

// Transcript returns the latest STT result accumulated so far.
func (s *Session) Transcript() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.prevTranscript
}

// AddAudio runs STT on the given cumulative audio blob, diffs the resulting
// transcript with the previous one, and calls onSentence for each newly
// detected complete sentence.
func (s *Session) AddAudio(ctx context.Context, audio []byte, onSentence func(string)) error {
	transcript, err := s.stt.Transcribe(ctx, audio)
	if err != nil {
		return err
	}
	transcript = strings.TrimSpace(transcript)
	log.Printf("stt partial: %q", transcript)

	s.mu.Lock()
	defer s.mu.Unlock()

	s.prevTranscript = transcript
	sentences, _ := splitSentences(transcript)

	for i := s.sentenceCount; i < len(sentences); i++ {
		s.sentenceCount++
		onSentence(sentences[i])
	}
	return nil
}

// Flush delivers any remaining transcript text not yet emitted as a sentence.
// Call this when the user has finished speaking (vad_end).
func (s *Session) Flush(onSentence func(string)) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sentences, remainder := splitSentences(s.prevTranscript)

	for i := s.sentenceCount; i < len(sentences); i++ {
		onSentence(sentences[i])
	}
	s.sentenceCount = len(sentences)

	if remainder = strings.TrimSpace(remainder); remainder != "" {
		onSentence(remainder)
	}
}

// splitSentences splits text on sentence boundaries (.!?\n) and returns complete
// sentences and the remaining incomplete fragment. Ellipsis (...) is not a boundary.
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

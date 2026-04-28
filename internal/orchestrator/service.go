package orchestrator

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"

	"sandbox-voice-ai/internal/llm"
)

var ErrEmptyTranscript = fmt.Errorf("empty transcript")

type STT interface {
	Transcribe(ctx context.Context, audio []byte) (string, error)
}

type LLM interface {
	GenerateStream(ctx context.Context, messages []llm.Message, onToken func(string) error) error
}

type TTS interface {
	Synthesize(ctx context.Context, text string) ([]byte, error)
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

// Synthesize streams an LLM response for the given messages, splits the response
// into sentences, synthesizes each with TTS and delivers WAV chunks via onAudio.
// Returns the full assistant response text.
func (s *Service) Synthesize(ctx context.Context, messages []llm.Message, onAudio func([]byte) error) (string, error) {
	var buf, fullResponse strings.Builder

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

	err := s.llm.GenerateStream(ctx, messages, func(token string) error {
		buf.WriteString(token)
		fullResponse.WriteString(token)
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
		return "", fmt.Errorf("llm: %w", err)
	}
	if err := synth(buf.String()); err != nil {
		return "", err
	}
	return fullResponse.String(), nil
}

const maxHistoryTurns = 10

// Session accumulates STT results across growing audio chunks and surfaces
// complete sentences as soon as they are detected — before the user stops speaking.
// It also maintains conversation history across turns.
type Session struct {
	stt STT

	// per-turn STT state (reset each turn)
	mu             sync.Mutex
	prevTranscript string
	delivered      []string

	// conversation history (persists across turns)
	history []llm.Message
}

// Messages builds the full message slice for the current LLM call: history + new user message.
func (s *Session) Messages(userText string) []llm.Message {
	msgs := make([]llm.Message, len(s.history)+1)
	copy(msgs, s.history)
	msgs[len(s.history)] = llm.Message{Role: "user", Content: userText}
	return msgs
}

// AppendTurn records a completed turn in conversation history.
func (s *Session) AppendTurn(userText, assistantText string) {
	userText      = strings.TrimSpace(userText)
	assistantText = strings.TrimSpace(assistantText)
	if userText == "" || assistantText == "" {
		return
	}
	s.history = append(s.history,
		llm.Message{Role: "user", Content: userText},
		llm.Message{Role: "assistant", Content: assistantText},
	)
	if len(s.history) > maxHistoryTurns*2 {
		s.history = s.history[len(s.history)-maxHistoryTurns*2:]
	}
	log.Printf("history: %d turn(s)", len(s.history)/2)
}

// Reset clears per-turn STT state. Conversation history is preserved.
func (s *Session) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prevTranscript = ""
	s.delivered = s.delivered[:0]
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
//
// Only sentences that are confirmed by the appearance of a subsequent sentence
// are dispatched here — the trailing sentence is always held back because the
// model may still be refining it (e.g. "Ja." → "Jaka jest stolica Polski?").
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

	for i := len(s.delivered); i < len(sentences)-1; i++ {
		s.delivered = append(s.delivered, sentences[i])
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

	for i := len(s.delivered); i < len(sentences); i++ {
		onSentence(sentences[i])
	}

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

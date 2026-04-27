package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
)

type OllamaClient struct {
	baseURL      string
	model        string
	systemPrompt string
	httpClient   *http.Client
}

type generateRequest struct {
	Model  string `json:"model"`
	System string `json:"system,omitempty"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

type generateChunk struct {
	Response string `json:"response"`
	Done     bool   `json:"done"`
	Error    string `json:"error"`
}

func NewOllamaClient(baseURL, model, systemPrompt string) *OllamaClient {
	return &OllamaClient{
		baseURL:      strings.TrimRight(baseURL, "/"),
		model:        model,
		systemPrompt: systemPrompt,
		httpClient:   &http.Client{},
	}
}

// GenerateStream streams response tokens from Ollama, calling onToken for each non-empty token.
// Returns when the stream is complete or onToken returns an error.
func (c *OllamaClient) GenerateStream(ctx context.Context, prompt string, onToken func(string) error) error {
	log.Printf("llm prompt: %q", prompt)
	reqBody := generateRequest{
		Model:  c.model,
		System: c.systemPrompt,
		Prompt: prompt,
		Stream: true,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("post request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("unexpected status %s: %s", resp.Status, strings.TrimSpace(string(raw)))
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var chunk generateChunk
		if err := json.Unmarshal([]byte(line), &chunk); err != nil {
			return fmt.Errorf("decode stream chunk: %w", err)
		}

		if chunk.Error != "" {
			return fmt.Errorf("ollama error: %s", chunk.Error)
		}

		if chunk.Response != "" {
			if err := onToken(chunk.Response); err != nil {
				return err
			}
		}

		if chunk.Done {
			break
		}
	}

	return scanner.Err()
}

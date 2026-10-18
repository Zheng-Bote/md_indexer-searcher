// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/md-indexer/internal/ai/model"
)

// OpenAICompatProvider calls OpenAI-compatible /chat/completions APIs (OpenRouter, NVIDIA NIM,
// Gemini's compatibility endpoint) with a bearer API key.
type OpenAICompatProvider struct {
	name       string
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewOpenAICompatProvider creates a provider; baseURL may include the /chat/completions path.
func NewOpenAICompatProvider(name, baseURL, apiKey string, client *http.Client) *OpenAICompatProvider {
	if client == nil {
		client = http.DefaultClient
	}
	baseURL = strings.TrimSuffix(strings.TrimRight(baseURL, "/"), "/chat/completions")
	return &OpenAICompatProvider{name: name, baseURL: baseURL, apiKey: apiKey, httpClient: client}
}

// Name returns the provider ID, e.g. "openrouter".
func (p *OpenAICompatProvider) Name() string { return p.name }

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model         string         `json:"model"`
	Messages      []chatMessage  `json:"messages"`
	MaxTokens     int            `json:"max_tokens,omitempty"`
	Temperature   *float64       `json:"temperature,omitempty"`
	Stream        bool           `json:"stream"`
	StreamOptions map[string]any `json:"stream_options,omitempty"`
}

type chatUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

type chatChunk struct {
	Model   string `json:"model"`
	Choices []struct {
		Delta        chatMessage `json:"delta"`
		Message      chatMessage `json:"message"`
		FinishReason string      `json:"finish_reason"`
	} `json:"choices"`
	Usage *chatUsage `json:"usage"`
}

// Generate requests a complete answer.
func (p *OpenAICompatProvider) Generate(ctx context.Context, req *model.GenerateRequest) (*model.GenerateResponse, error) {
	return p.GenerateStream(ctx, req, nil)
}

// GenerateStream streams the answer via server-sent events and invokes onToken per content delta.
func (p *OpenAICompatProvider) GenerateStream(ctx context.Context, req *model.GenerateRequest, onToken func(token string)) (*model.GenerateResponse, error) {
	messages := []chatMessage{{Role: "user", Content: req.Prompt}}
	if req.System != "" {
		messages = append([]chatMessage{{Role: "system", Content: req.System}}, messages...)
	}
	body := chatRequest{
		Model:         req.Model,
		Messages:      messages,
		MaxTokens:     req.MaxTokens,
		Stream:        true,
		StreamOptions: map[string]any{"include_usage": true},
	}
	if req.Temperature > 0 {
		body.Temperature = &req.Temperature
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal %s request: %w", p.name, err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, transportError(p.name, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, statusError(p.name, resp.StatusCode, string(respBody))
	}

	out := &model.GenerateResponse{Model: req.Model}
	var full strings.Builder
	finish := ""
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		data, ok := strings.CutPrefix(line, "data:")
		if !ok {
			continue
		}
		data = strings.TrimSpace(data)
		if data == "[DONE]" {
			break
		}
		var chunk chatChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return nil, fmt.Errorf("decode %s stream chunk: %w", p.name, err)
		}
		if chunk.Model != "" {
			out.Model = chunk.Model
		}
		if chunk.Usage != nil {
			out.PromptTokens, out.OutputTokens = chunk.Usage.PromptTokens, chunk.Usage.CompletionTokens
		}
		for _, c := range chunk.Choices {
			text := c.Delta.Content + c.Message.Content
			if text != "" {
				full.WriteString(text)
				if onToken != nil {
					onToken(text)
				}
			}
			if c.FinishReason != "" {
				finish = c.FinishReason
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, transportError(p.name, err)
	}
	out.Response = full.String()
	if finish == "length" {
		return out, &Error{Provider: p.name, Kind: ErrKindTruncated, Detail: "answer reached max_tokens"}
	}
	return out, nil
}

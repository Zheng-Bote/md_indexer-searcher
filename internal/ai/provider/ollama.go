// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/md-indexer/internal/ai/model"
)

// OllamaProvider communicates with a local Ollama daemon via its HTTP API.
type OllamaProvider struct {
	baseURL    string
	httpClient *http.Client
}

// NewOllamaProvider creates a new OllamaProvider instance.
func NewOllamaProvider(baseURL string, client *http.Client) *OllamaProvider {
	if client == nil {
		client = &http.Client{
			Timeout: 120 * time.Second,
		}
	}
	return &OllamaProvider{
		baseURL:    baseURL,
		httpClient: client,
	}
}

// Name returns "ollama".
func (p *OllamaProvider) Name() string {
	return "ollama"
}

type ollamaGenerateRequest struct {
	Model   string                 `json:"model"`
	Prompt  string                 `json:"prompt"`
	System  string                 `json:"system,omitempty"`
	Stream  bool                   `json:"stream"`
	Options map[string]interface{} `json:"options,omitempty"`
}

type ollamaGenerateResponse struct {
	Response           string `json:"response"`
	Model              string `json:"model"`
	PromptEvalCount    int    `json:"prompt_eval_count"`
	EvalCount          int    `json:"eval_count"`
	TotalDurationNanos int64  `json:"total_duration"`
	Done               bool   `json:"done"`
}

// Generate sends the prompt to the Ollama HTTP API endpoint and returns the generated text.
func (p *OllamaProvider) Generate(ctx context.Context, req *model.GenerateRequest) (*model.GenerateResponse, error) {
	endpoint := fmt.Sprintf("%s/api/generate", p.baseURL)

	options := make(map[string]interface{})
	if req.Temperature > 0 {
		options["temperature"] = req.Temperature
	}
	if req.MaxTokens > 0 {
		options["num_predict"] = req.MaxTokens
	}

	body := ollamaGenerateRequest{
		Model:   req.Model,
		Prompt:  req.Prompt,
		System:  req.System,
		Stream:  false,
		Options: options,
	}

	payloadBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal ollama request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewBuffer(payloadBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create http request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to call ollama at %s: %w", endpoint, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var ollamaResp ollamaGenerateResponse
	if err := json.NewDecoder(resp.Body).Decode(&ollamaResp); err != nil {
		return nil, fmt.Errorf("failed to decode ollama response: %w", err)
	}

	return &model.GenerateResponse{
		Response:     ollamaResp.Response,
		Model:        ollamaResp.Model,
		PromptTokens: ollamaResp.PromptEvalCount,
		OutputTokens: ollamaResp.EvalCount,
	}, nil
}

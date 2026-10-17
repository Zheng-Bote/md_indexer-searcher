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
	Think   bool                   `json:"think"`
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
	resp, err := p.doRequest(ctx, req, false)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

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

// GenerateStream sends the prompt with stream=true to Ollama and invokes onToken for each generated token chunk.
func (p *OllamaProvider) GenerateStream(ctx context.Context, req *model.GenerateRequest, onToken func(token string)) (*model.GenerateResponse, error) {
	resp, err := p.doRequest(ctx, req, true)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	dec := json.NewDecoder(resp.Body)
	var full bytes.Buffer
	var last ollamaGenerateResponse
	for {
		var chunk ollamaGenerateResponse
		if err := dec.Decode(&chunk); err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("failed to decode ollama stream chunk: %w", err)
		}
		if chunk.Response != "" {
			full.WriteString(chunk.Response)
			if onToken != nil {
				onToken(chunk.Response)
			}
		}
		last = chunk
		if chunk.Done {
			break
		}
	}

	modelName := last.Model
	if modelName == "" {
		modelName = req.Model
	}
	return &model.GenerateResponse{
		Response:     full.String(),
		Model:        modelName,
		PromptTokens: last.PromptEvalCount,
		OutputTokens: last.EvalCount,
	}, nil
}

type ollamaTagsResponse struct {
	Models []struct {
		Name    string `json:"name"`
		Size    int64  `json:"size"`
		Details struct {
			Family string `json:"family"`
		} `json:"details"`
	} `json:"models"`
}

// ListModels returns the models installed in the Ollama daemon via GET /api/tags.
func (p *OllamaProvider) ListModels(ctx context.Context) ([]model.ModelInfo, error) {
	endpoint := fmt.Sprintf("%s/api/tags", p.baseURL)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create http request: %w", err)
	}

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to call ollama at %s: %w", endpoint, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var tags ollamaTagsResponse
	if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil {
		return nil, fmt.Errorf("failed to decode ollama tags response: %w", err)
	}

	models := make([]model.ModelInfo, 0, len(tags.Models))
	for _, m := range tags.Models {
		models = append(models, model.ModelInfo{Name: m.Name, Size: m.Size, Family: m.Details.Family})
	}
	return models, nil
}

func (p *OllamaProvider) doRequest(ctx context.Context, req *model.GenerateRequest, stream bool) (*http.Response, error) {
	endpoint := fmt.Sprintf("%s/api/generate", p.baseURL)

	options := make(map[string]interface{})
	if req.Temperature > 0 {
		options["temperature"] = req.Temperature
	}
	if req.MaxTokens > 0 {
		options["num_predict"] = req.MaxTokens
	}
	if req.ContextWindow > 0 {
		// Ollama silently truncates prompts longer than num_ctx, dropping the knowledge context.
		options["num_ctx"] = req.ContextWindow
	}

	// Think is disabled for thinking models (e.g. qwen3); otherwise the hidden thinking phase
	// consumes the num_predict budget and can leave the answer empty. Non-thinking models accept it.
	body := ollamaGenerateRequest{
		Model:   req.Model,
		Prompt:  req.Prompt,
		System:  req.System,
		Stream:  stream,
		Think:   false,
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

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("ollama returned status %d: %s", resp.StatusCode, string(respBody))
	}
	return resp, nil
}

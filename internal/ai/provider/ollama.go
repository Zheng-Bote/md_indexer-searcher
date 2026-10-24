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
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/md-indexer/internal/ai/model"
	"github.com/md-indexer/internal/netaddr"
)

// OllamaProvider communicates with an Ollama daemon (local) or Ollama Cloud via its HTTP API.
type OllamaProvider struct {
	baseURL    string
	httpClient *http.Client
	name       string
	apiKey     string
	// useChat selects /api/chat instead of /api/generate; Ollama Cloud documents only /api/chat.
	useChat bool

	// caps caches the capabilities reported by /api/show per "name@digest".
	capsMu sync.Mutex
	caps   map[string][]string
}

const (
	// showConcurrency and showTimeout bound the /api/show calls of one model listing.
	showConcurrency = 4
	showTimeout     = 5 * time.Second
)

// OllamaOption configures an OllamaProvider.
type OllamaOption func(*OllamaProvider)

// WithOllamaCloud configures access to Ollama Cloud: bearer API key, /api/chat and the given provider name.
func WithOllamaCloud(name, apiKey string) OllamaOption {
	return func(p *OllamaProvider) {
		p.name = name
		p.apiKey = apiKey
		p.useChat = true
	}
}

// NewOllamaProvider creates a new OllamaProvider instance.
func NewOllamaProvider(baseURL string, client *http.Client, opts ...OllamaOption) *OllamaProvider {
	if client == nil {
		client = &http.Client{
			Timeout: 120 * time.Second,
		}
	}
	// Accept endpoints configured with an API path, e.g. https://ollama.com/api/chat.
	baseURL = strings.TrimRight(baseURL, "/")
	for _, suffix := range []string{"/api/chat", "/api/generate", "/api"} {
		baseURL = strings.TrimSuffix(baseURL, suffix)
	}
	p := &OllamaProvider{
		baseURL:    baseURL,
		httpClient: client,
		name:       "ollama",
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Name returns the provider ID ("ollama" for the local daemon).
func (p *OllamaProvider) Name() string {
	return p.name
}

type ollamaGenerateRequest struct {
	Model   string                 `json:"model"`
	Prompt  string                 `json:"prompt"`
	System  string                 `json:"system,omitempty"`
	Stream  bool                   `json:"stream"`
	Think   bool                   `json:"think"`
	Options map[string]interface{} `json:"options,omitempty"`
}

type ollamaChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ollamaChatRequest struct {
	Model    string                 `json:"model"`
	Messages []ollamaChatMessage    `json:"messages"`
	Stream   bool                   `json:"stream"`
	Think    bool                   `json:"think"`
	Options  map[string]interface{} `json:"options,omitempty"`
}

// ollamaGenerateResponse covers both /api/generate (response) and /api/chat (message.content) chunks.
type ollamaGenerateResponse struct {
	Message            *ollamaChatMessage `json:"message,omitempty"`
	Response           string             `json:"response"`
	Model              string             `json:"model"`
	PromptEvalCount    int                `json:"prompt_eval_count"`
	EvalCount          int                `json:"eval_count"`
	TotalDurationNanos int64              `json:"total_duration"`
	Done               bool               `json:"done"`
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
		Response:     ollamaResp.text(),
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
		if text := chunk.text(); text != "" {
			full.WriteString(text)
			if onToken != nil {
				onToken(text)
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
		Digest  string `json:"digest"`
		Details struct {
			Family        string   `json:"family"`
			Families      []string `json:"families"`
			ParameterSize string   `json:"parameter_size"`
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

	caps := p.listCapabilities(ctx, tags)
	// Loaded marks are informational; a failing /api/ps leaves them unset.
	loaded, _ := p.loadedModels(ctx)
	models := make([]model.ModelInfo, 0, len(tags.Models))
	for i, m := range tags.Models {
		families := append([]string{m.Details.Family}, m.Details.Families...)
		kind := kindOf(caps[i])
		if kind == "" {
			kind = ModelKind(m.Name, families)
		}
		models = append(models, model.ModelInfo{
			Name:          m.Name,
			Size:          m.Size,
			Family:        m.Details.Family,
			Provider:      p.name,
			ProviderName:  "Ollama",
			Kind:          kind,
			Locality:      p.locality(m.Name),
			ParameterSize: m.Details.ParameterSize,
		})
		info := &models[len(models)-1]
		info.Loaded = loaded[m.Name] && info.Locality == model.LocalityLocal
	}
	return models, nil
}

// listCapabilities returns the capabilities of each listed model (nil if unknown), querying
// /api/show for uncached models with bounded concurrency. ":cloud" models are not queried.
func (p *OllamaProvider) listCapabilities(ctx context.Context, tags ollamaTagsResponse) [][]string {
	caps := make([][]string, len(tags.Models))
	sem := make(chan struct{}, showConcurrency)
	var wg sync.WaitGroup
	for i, m := range tags.Models {
		if IsOllamaCloudModel(m.Name) {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			caps[i] = p.capabilities(ctx, m.Name, m.Digest)
		}()
	}
	wg.Wait()
	return caps
}

// capabilities returns the cached or freshly reported capabilities of a model; nil if /api/show
// fails or reports none (older Ollama). Only non-empty results are cached.
func (p *OllamaProvider) capabilities(ctx context.Context, name, digest string) []string {
	key := name + "@" + digest
	p.capsMu.Lock()
	cached, ok := p.caps[key]
	p.capsMu.Unlock()
	if ok {
		return cached
	}

	ctx, cancel := context.WithTimeout(ctx, showTimeout)
	defer cancel()
	body, _ := json.Marshal(map[string]string{"model": name})
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/api/show", bytes.NewReader(body))
	if err != nil {
		return nil
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	var show struct {
		Capabilities []string `json:"capabilities"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&show); err != nil || len(show.Capabilities) == 0 {
		return nil
	}

	p.capsMu.Lock()
	if p.caps == nil {
		p.caps = make(map[string][]string)
	}
	p.caps[key] = show.Capabilities
	p.capsMu.Unlock()
	return show.Capabilities
}

// kindOf maps Ollama capabilities to a model kind; "" if they say neither.
func kindOf(caps []string) string {
	var embed, generate bool
	for _, c := range caps {
		switch c {
		case "embedding":
			embed = true
		case "completion":
			generate = true
		}
	}
	switch {
	case embed && generate:
		return model.KindBoth
	case embed:
		return model.KindEmbedding
	case generate:
		return model.KindGeneration
	}
	return ""
}

// loadedModels returns the names of the models the daemon currently holds in memory (GET /api/ps).
func (p *OllamaProvider) loadedModels(ctx context.Context) (map[string]bool, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+"/api/ps", nil)
	if err != nil {
		return nil, err
	}
	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama returned status %d", resp.StatusCode)
	}
	var ps struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&ps); err != nil {
		return nil, err
	}
	loaded := make(map[string]bool, len(ps.Models))
	for _, m := range ps.Models {
		loaded[m.Name] = true
	}
	return loaded, nil
}

func (r ollamaGenerateResponse) text() string {
	if r.Message != nil {
		return r.Message.Content
	}
	return r.Response
}

func (p *OllamaProvider) doRequest(ctx context.Context, req *model.GenerateRequest, stream bool) (*http.Response, error) {
	endpoint := fmt.Sprintf("%s/api/generate", p.baseURL)
	if p.useChat {
		endpoint = fmt.Sprintf("%s/api/chat", p.baseURL)
	}

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
	var body any = ollamaGenerateRequest{
		Model:   req.Model,
		Prompt:  req.Prompt,
		System:  req.System,
		Stream:  stream,
		Think:   false,
		Options: options,
	}
	if p.useChat {
		messages := []ollamaChatMessage{{Role: "user", Content: req.Prompt}}
		if req.System != "" {
			messages = append([]ollamaChatMessage{{Role: "system", Content: req.System}}, messages...)
		}
		body = ollamaChatRequest{Model: req.Model, Messages: messages, Stream: stream, Think: false, Options: options}
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
	if p.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, transportError(p.name, fmt.Errorf("call %s: %w", endpoint, err))
	}

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, statusError(p.name, resp.StatusCode, string(respBody))
	}
	return resp, nil
}

// embeddingFamilies are Ollama model families that only produce embeddings.
var embeddingFamilies = map[string]bool{"bert": true, "nomic-bert": true}

// ModelKind classifies an Ollama model as embedding or generation model.
func ModelKind(name string, families []string) string {
	if strings.Contains(strings.ToLower(name), "embed") {
		return model.KindEmbedding
	}
	for _, f := range families {
		if embeddingFamilies[strings.ToLower(f)] {
			return model.KindEmbedding
		}
	}
	return model.KindGeneration
}

// IsOllamaCloudModel reports whether the local daemon proxies the model to Ollama's cloud
// (tag "cloud", e.g. "gemma4:cloud", or a "-cloud" tag suffix, e.g. "gpt-oss:120b-cloud").
func IsOllamaCloudModel(name string) bool {
	_, tag, ok := strings.Cut(name, ":")
	return ok && (tag == "cloud" || strings.HasSuffix(tag, "-cloud"))
}

// locality reports where a model of this daemon runs: local only on a loopback host and not proxied to the cloud.
func (p *OllamaProvider) locality(name string) string {
	if p.apiKey != "" || !isLoopbackURL(p.baseURL) || IsOllamaCloudModel(name) {
		return model.LocalityCloud
	}
	return model.LocalityLocal
}

func isLoopbackURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	return netaddr.IsLoopback(u.Hostname())
}

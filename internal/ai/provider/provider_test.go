// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/md-indexer/internal/ai/model"
)

func TestMockProvider(t *testing.T) {
	mock := NewMockProvider("test answer")
	if mock.Name() != "mock" {
		t.Errorf("expected name mock, got %s", mock.Name())
	}

	req := &model.GenerateRequest{
		Model:  "gemma3:1b",
		Prompt: "hello",
	}

	resp, err := mock.Generate(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Response != "test answer" {
		t.Errorf("expected 'test answer', got %s", resp.Response)
	}
	if len(mock.Calls) != 1 {
		t.Errorf("expected 1 call recorded, got %d", len(mock.Calls))
	}
}

func TestOllamaProvider_Generate(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/generate" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed to read request: %v", err)
		}
		var req ollamaGenerateRequest
		if err := json.Unmarshal(raw, &req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		if req.Model != "gemma3:1b" {
			t.Errorf("expected model gemma3:1b, got %s", req.Model)
		}
		var fields map[string]interface{}
		if err := json.Unmarshal(raw, &fields); err != nil {
			t.Fatalf("failed to decode request fields: %v", err)
		}
		if think, ok := fields["think"]; !ok || think != false {
			t.Errorf("expected explicit think=false in request, got %v (present: %v)", think, ok)
		}
		if numCtx := req.Options["num_ctx"]; numCtx != float64(8192) {
			t.Errorf("expected num_ctx 8192, got %v", numCtx)
		}

		resp := ollamaGenerateResponse{
			Response:        "grounded answer from ollama",
			Model:           req.Model,
			PromptEvalCount: 15,
			EvalCount:       25,
			Done:            true,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	provider := NewOllamaProvider(server.URL, server.Client())
	if provider.Name() != "ollama" {
		t.Errorf("expected name ollama, got %s", provider.Name())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	genReq := &model.GenerateRequest{
		Model:         "gemma3:1b",
		Prompt:        "Explain something",
		Temperature:   0.1,
		MaxTokens:     100,
		ContextWindow: 8192,
	}

	res, err := provider.Generate(ctx, genReq)
	if err != nil {
		t.Fatalf("failed to generate: %v", err)
	}

	if res.Response != "grounded answer from ollama" {
		t.Errorf("unexpected response: %s", res.Response)
	}
	if res.OutputTokens != 25 {
		t.Errorf("expected 25 output tokens, got %d", res.OutputTokens)
	}
}

func TestOllamaProvider_Error(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "model not found", http.StatusNotFound)
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	provider := NewOllamaProvider(server.URL, server.Client())
	_, err := provider.Generate(context.Background(), &model.GenerateRequest{Model: "invalid"})
	if err == nil {
		t.Fatal("expected error on 404 status, got nil")
	}
}

func TestOllamaProvider_GenerateStream(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req ollamaGenerateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		if !req.Stream {
			t.Errorf("expected stream=true in GenerateStream request")
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		chunks := []ollamaGenerateResponse{
			{Response: "Hello ", Model: req.Model, Done: false},
			{Response: "world!", Model: req.Model, Done: false},
			{Response: "", Model: req.Model, PromptEvalCount: 12, EvalCount: 4, Done: true},
		}
		enc := json.NewEncoder(w)
		for _, c := range chunks {
			_ = enc.Encode(c)
		}
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	prov := NewOllamaProvider(server.URL, server.Client())
	var streamed string
	res, err := prov.GenerateStream(context.Background(), &model.GenerateRequest{
		Model:  "qwen3:14b",
		Prompt: "Say hello",
	}, func(tok string) {
		streamed += tok
	})
	if err != nil {
		t.Fatalf("GenerateStream failed: %v", err)
	}
	if streamed != "Hello world!" || res.Response != "Hello world!" {
		t.Errorf("expected 'Hello world!', got streamed=%q response=%q", streamed, res.Response)
	}
	if res.PromptTokens != 12 || res.OutputTokens != 4 {
		t.Errorf("unexpected token counts: %+v", res)
	}
}

func TestOllamaProvider_ListModels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if r.URL.Path == "/api/ps" {
			w.Write([]byte(`{"models":[{"name":"qwen3:14b"},{"name":"gemma4:cloud"}]}`))
			return
		}
		w.Write([]byte(`{"models":[{"name":"qwen3:14b","size":9300000000,"details":{"family":"qwen3","parameter_size":"14.8B"}},{"name":"bge-m3:latest","size":1200000000,"details":{"family":"bert","families":["bert"],"parameter_size":"566.70M"}},{"name":"gemma4:cloud","size":0,"details":{"family":"gemma4"}},{"name":"gpt-oss:120b-cloud","size":0,"details":{"family":"gptoss"}}]}`))
	}))
	defer srv.Close()

	p := NewOllamaProvider(srv.URL, nil)
	models, err := p.ListModels(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	base := model.ModelInfo{Provider: "ollama", ProviderName: "Ollama"}
	with := func(m model.ModelInfo) model.ModelInfo {
		m.Provider, m.ProviderName = base.Provider, base.ProviderName
		return m
	}
	want := []model.ModelInfo{
		with(model.ModelInfo{Name: "qwen3:14b", Size: 9300000000, Family: "qwen3", Kind: model.KindGeneration, Locality: model.LocalityLocal, ParameterSize: "14.8B", Loaded: true}),
		with(model.ModelInfo{Name: "bge-m3:latest", Size: 1200000000, Family: "bert", Kind: model.KindEmbedding, Locality: model.LocalityLocal, ParameterSize: "566.70M"}),
		with(model.ModelInfo{Name: "gemma4:cloud", Family: "gemma4", Kind: model.KindGeneration, Locality: model.LocalityCloud}),
		with(model.ModelInfo{Name: "gpt-oss:120b-cloud", Family: "gptoss", Kind: model.KindGeneration, Locality: model.LocalityCloud}),
	}
	if len(models) != len(want) {
		t.Fatalf("expected %d models, got %d", len(want), len(models))
	}
	for i := range want {
		if models[i] != want[i] {
			t.Errorf("model %d: expected %+v, got %+v", i, want[i], models[i])
		}
	}
}

func TestOllamaProvider_ListModelsWithoutLoadedInfo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/ps" {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		w.Write([]byte(`{"models":[{"name":"qwen3:14b","details":{"family":"qwen3"}}]}`))
	}))
	defer srv.Close()

	models, err := NewOllamaProvider(srv.URL, nil).ListModels(context.Background())
	if err != nil || len(models) != 1 || models[0].Loaded {
		t.Fatalf("expected one unmarked model without error, got %+v, %v", models, err)
	}
}

func TestOllamaProvider_ListModelsUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close()

	p := NewOllamaProvider(url, &http.Client{Timeout: time.Second})
	if _, err := p.ListModels(context.Background()); err == nil {
		t.Fatal("expected error for unreachable ollama host")
	}
}

func TestOllamaProvider_CloudChatWithBearer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Errorf("expected /api/chat, got %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer cloud-key" {
			t.Errorf("unexpected Authorization header %q", got)
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		msgs, _ := body["messages"].([]any)
		if len(msgs) != 2 || msgs[0].(map[string]any)["role"] != "system" {
			t.Errorf("expected system + user messages, got %v", body["messages"])
		}
		w.Write([]byte(`{"message":{"role":"assistant","content":"Hallo "},"done":false}` + "\n" +
			`{"message":{"role":"assistant","content":"Welt"},"done":true,"model":"gemma4:31b","prompt_eval_count":12,"eval_count":3}` + "\n"))
	}))
	defer srv.Close()

	p := NewOllamaProvider(srv.URL+"/api/chat", nil, WithOllamaCloud("ollama-cloud", "cloud-key"))
	var tokens []string
	res, err := p.GenerateStream(context.Background(), &model.GenerateRequest{Model: "gemma4:31b", System: "sys", Prompt: "q"}, func(t string) { tokens = append(tokens, t) })
	if err != nil {
		t.Fatal(err)
	}
	if res.Response != "Hallo Welt" || len(tokens) != 2 || res.PromptTokens != 12 || p.Name() != "ollama-cloud" {
		t.Errorf("unexpected result %+v tokens %v", res, tokens)
	}
}

func TestOllamaProvider_ErrorKinds(t *testing.T) {
	for status, kind := range map[int]string{401: ErrKindAuth, 429: ErrKindRateLimit, 503: ErrKindUnavailable} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
		}))
		_, err := NewOllamaProvider(srv.URL, nil, WithOllamaCloud("ollama-cloud", "k")).Generate(context.Background(), &model.GenerateRequest{Model: "m"})
		srv.Close()
		var perr *Error
		if !errors.As(err, &perr) || perr.Kind != kind || perr.Provider != "ollama-cloud" {
			t.Errorf("status %d: expected kind %s, got %v", status, kind, err)
		}
	}
}

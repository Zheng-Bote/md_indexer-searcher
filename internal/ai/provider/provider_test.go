// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"context"
	"encoding/json"
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
		var req ollamaGenerateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		if req.Model != "gemma3:1b" {
			t.Errorf("expected model gemma3:1b, got %s", req.Model)
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
		Model:       "gemma3:1b",
		Prompt:      "Explain something",
		Temperature: 0.1,
		MaxTokens:   100,
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

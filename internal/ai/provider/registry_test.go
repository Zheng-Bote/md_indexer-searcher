// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/md-indexer/internal/ai/config"
	"github.com/md-indexer/internal/ai/model"
)

func TestRegistry_ListAndResolve(t *testing.T) {
	ollama := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"models":[{"name":"qwen3:14b","details":{"family":"qwen3"}},{"name":"gemma4:cloud","details":{"family":"gemma4"}}]}`))
	}))
	defer ollama.Close()

	reg := NewRegistry(NewOllamaProvider(ollama.URL, nil), []config.ProviderConfig{
		{ID: config.ProviderClaude, DisplayName: "Claude", Protocol: config.ProtocolClaude, APIKey: "k", Models: []string{"claude-opus-5-5"}},
		{ID: config.ProviderGemini, DisplayName: "Gemini", Protocol: config.ProtocolOpenAI, APIKey: "k", Endpoint: "https://example.invalid", Models: []string{"gemini-3.8-flash"}},
	}, time.Minute, nil)

	models, err := reg.ListModels(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 4 || models[2].Provider != "claude" || models[2].Locality != model.LocalityCloud || models[3].ProviderName != "Gemini" {
		t.Fatalf("unexpected models %+v", models)
	}

	tests := []struct {
		ref      model.ModelRef
		name     string
		locality string
	}{
		{model.ModelRef{Provider: "ollama", Name: "qwen3:14b"}, "ollama", model.LocalityLocal},
		{model.ModelRef{Provider: "ollama", Name: "gemma4:cloud"}, "ollama", model.LocalityCloud},
		{model.ModelRef{Provider: "claude", Name: "claude-opus-5-5"}, "claude", model.LocalityCloud},
		{model.ModelRef{Provider: "gemini", Name: "gemini-3.8-flash"}, "gemini", model.LocalityCloud},
	}
	for _, tt := range tests {
		p, locality, err := reg.Resolve(tt.ref)
		if err != nil || p.Name() != tt.name || locality != tt.locality {
			t.Errorf("Resolve(%+v) = %v, %s, %v", tt.ref, p, locality, err)
		}
	}

	var unknown *ErrUnknownModel
	for _, ref := range []model.ModelRef{{Provider: "claude", Name: "claude-not-configured"}, {Provider: "groq", Name: "llama"}} {
		if _, _, err := reg.Resolve(ref); !errors.As(err, &unknown) {
			t.Errorf("Resolve(%+v) must fail with ErrUnknownModel, got %v", ref, err)
		}
	}
}

func TestRegistry_ListModelsWithoutLocalDaemon(t *testing.T) {
	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	downURL := down.URL
	down.Close()

	reg := NewRegistry(NewOllamaProvider(downURL, &http.Client{Timeout: time.Second}), []config.ProviderConfig{
		{ID: config.ProviderClaude, DisplayName: "Claude", Protocol: config.ProtocolClaude, APIKey: "k", Models: []string{"claude-opus-5-5"}},
	}, time.Minute, nil)

	models, err := reg.ListModels(context.Background())
	var localErr *LocalModelsError
	if !errors.As(err, &localErr) || localErr.Error() == "" {
		t.Fatalf("expected *LocalModelsError, got %v", err)
	}
	if len(models) != 1 || models[0].Name != "claude-opus-5-5" || models[0].Locality != model.LocalityCloud {
		t.Fatalf("expected the cloud models, got %+v", models)
	}
}

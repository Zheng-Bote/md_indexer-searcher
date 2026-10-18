// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/md-indexer/internal/ai/config"
	"github.com/md-indexer/internal/ai/model"
)

type cloudEntry struct {
	cfg      config.ProviderConfig
	provider LLMProvider
	models   map[string]bool
}

// Registry knows the local Ollama daemon and the cloud providers enabled in the .env file.
// It lists all offered models and resolves a model reference to the provider that serves it.
type Registry struct {
	local *OllamaProvider
	cloud []cloudEntry
}

// NewRegistry builds one provider per enabled cloud provider config.
func NewRegistry(local *OllamaProvider, cloud []config.ProviderConfig, timeout time.Duration, effortFor func(string) string) *Registry {
	r := &Registry{local: local}
	client := &http.Client{Timeout: timeout}
	for _, pc := range cloud {
		var p LLMProvider
		switch pc.Protocol {
		case config.ProtocolOllama:
			p = NewOllamaProvider(pc.Endpoint, client, WithOllamaCloud(pc.ID, pc.APIKey))
		case config.ProtocolOpenAI:
			p = NewOpenAICompatProvider(pc.ID, pc.Endpoint, pc.APIKey, client)
		case config.ProtocolClaude:
			p = NewClaudeProvider(pc.ID, pc.APIKey, pc.Endpoint, timeout, effortFor, nil)
		default:
			continue
		}
		models := make(map[string]bool, len(pc.Models))
		for _, m := range pc.Models {
			models[m] = true
		}
		r.cloud = append(r.cloud, cloudEntry{cfg: pc, provider: p, models: models})
	}
	return r
}

// ListModels returns the local daemon's models followed by the configured cloud models.
func (r *Registry) ListModels(ctx context.Context) ([]model.ModelInfo, error) {
	models, err := r.local.ListModels(ctx)
	if err != nil {
		return nil, err
	}
	for _, e := range r.cloud {
		for _, name := range e.cfg.Models {
			models = append(models, model.ModelInfo{
				Name:         name,
				Provider:     e.cfg.ID,
				ProviderName: e.cfg.DisplayName,
				Locality:     model.LocalityCloud,
				Kind:         model.KindGeneration,
			})
		}
	}
	return models, nil
}

// Providers returns the IDs and model counts of the enabled cloud providers (for startup logs).
func (r *Registry) Providers() map[string]int {
	out := make(map[string]int, len(r.cloud))
	for _, e := range r.cloud {
		out[e.cfg.ID] = len(e.cfg.Models)
	}
	return out
}

// ErrUnknownModel is returned for references to providers or models that are not configured.
type ErrUnknownModel struct{ Ref model.ModelRef }

func (e *ErrUnknownModel) Error() string {
	return fmt.Sprintf("unknown model %q of provider %q", e.Ref.Name, e.Ref.Provider)
}

// Resolve returns the provider serving ref and the model's locality.
func (r *Registry) Resolve(ref model.ModelRef) (LLMProvider, string, error) {
	if ref.Provider == r.local.Name() {
		if ref.Name == "" {
			return nil, "", &ErrUnknownModel{Ref: ref}
		}
		return r.local, r.local.locality(ref.Name), nil
	}
	for _, e := range r.cloud {
		if e.cfg.ID == ref.Provider && e.models[ref.Name] {
			return e.provider, model.LocalityCloud, nil
		}
	}
	return nil, "", &ErrUnknownModel{Ref: ref}
}

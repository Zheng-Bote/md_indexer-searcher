// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"context"

	"github.com/md-indexer/internal/ai/model"
)

// LLMProvider defines the contract for model inference backends.
type LLMProvider interface {
	// Generate produces a completion for the provided request.
	Generate(ctx context.Context, req *model.GenerateRequest) (*model.GenerateResponse, error)
	// Name returns the provider identifier.
	Name() string
}

// StreamingLLMProvider is an optional interface for LLM backends that support incremental token streaming.
type StreamingLLMProvider interface {
	LLMProvider
	GenerateStream(ctx context.Context, req *model.GenerateRequest, onToken func(token string)) (*model.GenerateResponse, error)
}

// ModelLister is an optional interface for LLM backends that can enumerate their installed models.
type ModelLister interface {
	ListModels(ctx context.Context) ([]model.ModelInfo, error)
}

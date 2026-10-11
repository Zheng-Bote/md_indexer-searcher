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

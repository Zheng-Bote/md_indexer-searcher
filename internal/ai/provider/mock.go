// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/md-indexer/internal/ai/model"
)

// MockProvider provides deterministic responses for testing.
type MockProvider struct {
	ResponseFunc func(ctx context.Context, req *model.GenerateRequest) (*model.GenerateResponse, error)
	Calls        []*model.GenerateRequest
}

// NewMockProvider creates a new MockProvider with a static response string.
func NewMockProvider(staticResponse string) *MockProvider {
	return &MockProvider{
		ResponseFunc: func(ctx context.Context, req *model.GenerateRequest) (*model.GenerateResponse, error) {
			return &model.GenerateResponse{
				Response:     staticResponse,
				Model:        req.Model,
				PromptTokens: 10,
				OutputTokens: 20,
			}, nil
		},
		Calls: make([]*model.GenerateRequest, 0),
	}
}

// Name returns "mock".
func (m *MockProvider) Name() string {
	return "mock"
}

// Generate captures the request and returns the configured response.
func (m *MockProvider) Generate(ctx context.Context, req *model.GenerateRequest) (*model.GenerateResponse, error) {
	m.Calls = append(m.Calls, req)
	if m.ResponseFunc != nil {
		return m.ResponseFunc(ctx, req)
	}
	return nil, fmt.Errorf("mock response func not configured")
}

// GenerateStream captures the request, emits the response via onToken, and returns the configured response.
func (m *MockProvider) GenerateStream(ctx context.Context, req *model.GenerateRequest, onToken func(token string)) (*model.GenerateResponse, error) {
	resp, err := m.Generate(ctx, req)
	if err != nil {
		return nil, err
	}
	if onToken != nil && resp.Response != "" {
		onToken(resp.Response)
	}
	return resp, nil
}

// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/shared/constant"

	"github.com/md-indexer/internal/ai/model"
)

// fallbackModels accept Anthropic's server-side refusal fallback in its "default" form: a declined
// request is re-served by another Claude model within the same call (the answering model is reported).
var fallbackModels = map[string]bool{
	"claude-fable-5-1":  true,
	"claude-opus-5-5":   true,
	"claude-opus-5":     true,
	"claude-sonnet-5-5": true,
}

// ClaudeProvider calls the Anthropic Messages API through the official Go SDK.
type ClaudeProvider struct {
	name      string
	client    anthropic.Client
	effortFor func(model string) string
}

// NewClaudeProvider creates a Claude provider. baseURL may be empty (SDK default); effortFor returns
// the configured effort (low|medium|high|xhigh|max) per model.
func NewClaudeProvider(name, apiKey, baseURL string, timeout time.Duration, effortFor func(string) string, httpClient *http.Client) *ClaudeProvider {
	opts := []option.RequestOption{
		option.WithAPIKey(apiKey),
		option.WithRequestTimeout(timeout),
		option.WithMaxRetries(2),
	}
	if baseURL != "" {
		opts = append(opts, option.WithBaseURL(baseURL))
	}
	if httpClient != nil {
		opts = append(opts, option.WithHTTPClient(httpClient))
	}
	return &ClaudeProvider{name: name, client: anthropic.NewClient(opts...), effortFor: effortFor}
}

// Name returns the provider ID ("claude").
func (p *ClaudeProvider) Name() string { return p.name }

// Generate requests a complete answer (streamed internally to avoid HTTP timeouts on long answers).
func (p *ClaudeProvider) Generate(ctx context.Context, req *model.GenerateRequest) (*model.GenerateResponse, error) {
	return p.GenerateStream(ctx, req, nil)
}

// GenerateStream streams the answer and invokes onToken for each text delta. Temperature is not
// sent: current Claude models reject sampling parameters. Thinking blocks are not used.
func (p *ClaudeProvider) GenerateStream(ctx context.Context, req *model.GenerateRequest, onToken func(token string)) (*model.GenerateResponse, error) {
	params := anthropic.BetaMessageNewParams{
		Model:     anthropic.Model(req.Model),
		MaxTokens: int64(req.MaxTokens),
		Messages:  []anthropic.BetaMessageParam{anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock(req.Prompt))},
	}
	if req.System != "" {
		params.System = []anthropic.BetaTextBlockParam{{Text: req.System}}
	}
	if p.effortFor != nil {
		if effort := p.effortFor(req.Model); effort != "" {
			params.OutputConfig = anthropic.BetaOutputConfigParam{Effort: anthropic.BetaOutputConfigEffort(effort)}
		}
	}
	if fallbackModels[req.Model] {
		params.Betas = []anthropic.AnthropicBeta{anthropic.AnthropicBetaServerSideFallback2026_07_01}
		params.Fallbacks = anthropic.BetaFallbacksParamUnion{OfDefault: constant.ValueOf[constant.Default]()}
	}

	stream := p.client.Beta.Messages.NewStreaming(ctx, params)
	defer stream.Close()
	msg := anthropic.BetaMessage{}
	for stream.Next() {
		event := stream.Current()
		if err := msg.Accumulate(event); err != nil {
			return nil, &Error{Provider: p.name, Kind: ErrKindUnavailable, Detail: err.Error()}
		}
		if onToken == nil {
			continue
		}
		if delta, ok := event.AsAny().(anthropic.BetaRawContentBlockDeltaEvent); ok {
			if text, ok := delta.Delta.AsAny().(anthropic.BetaTextDelta); ok && text.Text != "" {
				onToken(text.Text)
			}
		}
	}
	if err := stream.Err(); err != nil {
		return nil, p.classify(err)
	}

	out := &model.GenerateResponse{
		Model:        string(msg.Model),
		PromptTokens: int(msg.Usage.InputTokens),
		OutputTokens: int(msg.Usage.OutputTokens),
	}
	if out.Model == "" {
		out.Model = req.Model
	}
	for _, block := range msg.Content {
		if text, ok := block.AsAny().(anthropic.BetaTextBlock); ok {
			out.Response += text.Text
		}
	}

	switch msg.StopReason {
	case anthropic.BetaStopReasonRefusal:
		detail := "request declined"
		if c := string(msg.StopDetails.Category); c != "" {
			detail = "request declined (category: " + c + ")"
		}
		return out, &Error{Provider: p.name, Kind: ErrKindRefusal, Detail: detail}
	case anthropic.BetaStopReasonMaxTokens:
		return out, &Error{Provider: p.name, Kind: ErrKindTruncated, Detail: "answer reached max_tokens"}
	}
	return out, nil
}

func (p *ClaudeProvider) classify(err error) *Error {
	var apiErr *anthropic.Error
	if errors.As(err, &apiErr) {
		return statusError(p.name, apiErr.StatusCode, apiErr.Error())
	}
	return transportError(p.name, err)
}

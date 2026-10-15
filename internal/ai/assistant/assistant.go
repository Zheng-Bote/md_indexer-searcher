// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

package assistant

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/md-indexer/internal/ai/citations"
	"github.com/md-indexer/internal/ai/classifier"
	"github.com/md-indexer/internal/ai/config"
	"github.com/md-indexer/internal/ai/contextbuilder"
	"github.com/md-indexer/internal/ai/conversation"
	"github.com/md-indexer/internal/ai/gaps"
	"github.com/md-indexer/internal/ai/hybrid"
	"github.com/md-indexer/internal/ai/model"
	"github.com/md-indexer/internal/ai/prompt"
	"github.com/md-indexer/internal/ai/provider"
	"github.com/md-indexer/internal/ai/retriever"
)

// Retriever fetches evidence for a classified question.
type Retriever interface {
	Retrieve(ctx context.Context, query string, intent model.Intent) (*model.RetrievalResult, error)
}

// NewRetriever creates the retriever selected by cfg.Retriever on a read-only knowledge.db connection.
func NewRetriever(db *sql.DB, cfg *config.Config) Retriever {
	if cfg.Retriever == config.RetrieverClassic {
		return retriever.NewRetriever(db, cfg.GraphDepth)
	}
	weights := hybrid.Weights{FTS: cfg.FusionFTSWeight, Graph: cfg.FusionGraphWeight, Entity: cfg.FusionEntityWeight}
	return hybrid.NewSQLiteRetriever(db, cfg.GraphDepth, weights, cfg.TopKFTS)
}

// Assistant orchestrates all AI subcomponents into a coherent Q&A pipeline.
type Assistant struct {
	cfg            *config.Config
	classifier     classifier.Classifier
	retriever      Retriever
	contextBuilder *contextbuilder.ContextBuilder
	provider       provider.LLMProvider
}

// NewAssistant creates a new Assistant instance.
func NewAssistant(
	cfg *config.Config,
	clf classifier.Classifier,
	ret Retriever,
	cb *contextbuilder.ContextBuilder,
	prov provider.LLMProvider,
) *Assistant {
	return &Assistant{
		cfg:            cfg,
		classifier:     clf,
		retriever:      ret,
		contextBuilder: cb,
		provider:       prov,
	}
}

// Ask processes a single question and returns the grounded answer.
func (a *Assistant) Ask(ctx context.Context, question string, session *conversation.Session) (*model.Answer, error) {
	// 1. Classify intent
	intent := a.classifier.Classify(question)

	// 2. Retrieve knowledge
	retrievalRes, err := a.retriever.Retrieve(ctx, question, intent)
	if err != nil {
		return nil, fmt.Errorf("retrieval failed: %w", err)
	}

	// 3. Build prioritized context
	blocks := a.contextBuilder.BuildContext(retrievalRes)
	formattedContext := a.contextBuilder.FormatContext(blocks)

	// 4. Inject session history if available
	fullQuestion := question
	if session != nil {
		history := session.FormatHistory()
		if history != "" {
			fullQuestion = fmt.Sprintf("Prior Conversation:\n%s\n\nCurrent Question:\n%s", history, question)
		}
	}

	// 5. Construct prompts
	sysPrompt, userPrompt := prompt.BuildPrompt(fullQuestion, formattedContext, intent)

	// 6. Generate answer via LLM Provider
	req := &model.GenerateRequest{
		Model:         a.cfg.Model,
		Prompt:        userPrompt,
		System:        sysPrompt,
		Temperature:   a.cfg.Temperature,
		MaxTokens:     a.cfg.MaxTokens,
		ContextWindow: contextWindow(sysPrompt, userPrompt, a.cfg.MaxTokens),
	}

	genResp, err := a.provider.Generate(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("llm generation failed: %w", err)
	}

	// 7. Post-process citations and knowledge gaps
	cites := citations.ExtractCitations(genResp.Response)
	detectedGaps := gaps.DetectGaps(genResp.Response)

	// Attach snippets to citations if matching paths exist
	for i, c := range cites {
		for _, s := range retrievalRes.Snippets {
			if s.DocumentPath == c.Path {
				snip := s.Content
				if len(snip) > 200 {
					snip = snip[:200] + "..."
				}
				cites[i].Snippet = strings.TrimSpace(snip)
				break
			}
		}
	}

	answer := &model.Answer{
		Question: question,
		Answer:   genResp.Response,
		Intent:   intent,
		Model:    genResp.Model,
		Sources:  cites,
		Gaps:     detectedGaps,
	}

	if session != nil {
		session.AddTurn(question, genResp.Response)
	}

	return answer, nil
}

// contextWindowStep rounds the window up so that small prompt size changes do not force Ollama to reload the model.
const contextWindowStep = 8192

// contextWindow sizes the model window for the prompt plus the generated answer.
// It assumes ~3 bytes per token, which is conservative for mixed German/English Markdown and code.
func contextWindow(systemPrompt, userPrompt string, maxTokens int) int {
	needed := (len(systemPrompt)+len(userPrompt))/3 + maxTokens
	return (needed/contextWindowStep + 1) * contextWindowStep
}

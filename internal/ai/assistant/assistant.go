// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

package assistant

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
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
	"github.com/md-indexer/internal/embedding"
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
	var vectors hybrid.VectorRetriever
	if cfg.EnableVectors {
		embedder := embedding.NewOllamaEmbedder(cfg.OllamaHost, cfg.EmbeddingModel, &http.Client{Timeout: cfg.Timeout()})
		vectors = hybrid.NewSQLiteVectors(db, embedder, cfg.MinVectorScore)
	}
	weights := hybrid.Weights{FTS: cfg.FusionFTSWeight, Vector: cfg.FusionVectorWeight, Graph: cfg.FusionGraphWeight, Entity: cfg.FusionEntityWeight}
	return hybrid.NewSQLiteRetriever(db, cfg.GraphDepth, vectors, weights, cfg.TopKFTS, cfg.TopKVector)
}

// ModelResolver maps a model reference to the provider serving it and the model's locality.
type ModelResolver interface {
	Resolve(ref model.ModelRef) (provider.LLMProvider, string, error)
}

// Assistant orchestrates all AI subcomponents into a coherent Q&A pipeline.
type Assistant struct {
	cfg            *config.Config
	classifier     classifier.Classifier
	retriever      Retriever
	contextBuilder *contextbuilder.ContextBuilder
	provider       provider.LLMProvider
	// models and cloudContext are set by WithModels to answer with per-request models.
	models       ModelResolver
	cloudContext *contextbuilder.ContextBuilder
	defaultRef   *model.ModelRef
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

// WithModels enables per-request model selection; cloudContext budgets the context for cloud models.
func (a *Assistant) WithModels(r ModelResolver, cloudContext *contextbuilder.ContextBuilder) *Assistant {
	a.models = r
	a.cloudContext = cloudContext
	return a
}

// WithDefaultModel answers requests without an explicit model with ref (e.g. md-ai --provider claude).
func (a *Assistant) WithDefaultModel(ref model.ModelRef) *Assistant {
	a.defaultRef = &ref
	return a
}

// Ask processes a single question and returns the grounded answer.
func (a *Assistant) Ask(ctx context.Context, question string, session *conversation.Session) (*model.Answer, error) {
	return a.AskStream(ctx, question, session, nil, nil)
}

// AskModel answers with the referenced model; nil uses the configured default model.
func (a *Assistant) AskModel(ctx context.Context, question string, session *conversation.Session, ref *model.ModelRef) (*model.Answer, error) {
	return a.ask(ctx, question, session, ref, nil, nil)
}

// AskStream processes a single question, optionally reporting retrieval plan telemetry via onPlan
// and streaming generated tokens via onToken when the provider supports StreamingLLMProvider.
func (a *Assistant) AskStream(
	ctx context.Context,
	question string,
	session *conversation.Session,
	onPlan func(plan *model.RetrievalPlanInfo, blocks int),
	onToken func(token string),
) (*model.Answer, error) {
	return a.ask(ctx, question, session, nil, onPlan, onToken)
}

// target describes which provider answers and with which budgets.
type target struct {
	provider  provider.LLMProvider
	model     string
	locality  string
	builder   *contextbuilder.ContextBuilder
	maxTokens int
}

// resolve picks the provider for ref. Cloud models get the cloud context and output budgets;
// errors never fall back to another provider.
func (a *Assistant) resolve(ref *model.ModelRef) (target, error) {
	local := target{provider: a.provider, model: a.cfg.Model, locality: model.LocalityLocal, builder: a.contextBuilder, maxTokens: a.cfg.MaxTokens}
	if ref == nil {
		ref = a.defaultRef
	}
	if ref == nil || a.models == nil {
		if ref != nil {
			return target{}, fmt.Errorf("model selection is not configured")
		}
		return local, nil
	}
	p, locality, err := a.models.Resolve(*ref)
	if err != nil {
		return target{}, err
	}
	t := target{provider: p, model: ref.Name, locality: locality, builder: a.contextBuilder, maxTokens: a.cfg.MaxTokens}
	if locality == model.LocalityCloud && a.cloudContext != nil {
		t.builder, t.maxTokens = a.cloudContext, a.cfg.CloudMaxTokens
	}
	return t, nil
}

func (a *Assistant) ask(
	ctx context.Context,
	question string,
	session *conversation.Session,
	ref *model.ModelRef,
	onPlan func(plan *model.RetrievalPlanInfo, blocks int),
	onToken func(token string),
) (*model.Answer, error) {
	tgt, err := a.resolve(ref)
	if err != nil {
		return nil, err
	}

	// 1. Classify intent
	intent := a.classifier.Classify(question)

	// 2. Retrieve knowledge
	retrievalRes, err := a.retriever.Retrieve(ctx, question, intent)
	if err != nil {
		return nil, fmt.Errorf("retrieval failed: %w", err)
	}

	// 3. Build prioritized context
	blocks := tgt.builder.BuildContext(retrievalRes)
	formattedContext := tgt.builder.FormatContext(blocks)
	if onPlan != nil {
		onPlan(retrievalRes.Plan, len(blocks))
	}

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
		Model:         tgt.model,
		Prompt:        userPrompt,
		System:        sysPrompt,
		Temperature:   a.cfg.Temperature,
		MaxTokens:     tgt.maxTokens,
		ContextWindow: contextWindow(sysPrompt, userPrompt, tgt.maxTokens),
	}

	var genResp *model.GenerateResponse
	if onToken != nil {
		if sp, ok := tgt.provider.(provider.StreamingLLMProvider); ok {
			genResp, err = sp.GenerateStream(ctx, req, onToken)
		} else {
			genResp, err = tgt.provider.Generate(ctx, req)
			if err == nil && genResp.Response != "" {
				onToken(genResp.Response)
			}
		}
	} else {
		genResp, err = tgt.provider.Generate(ctx, req)
	}
	if err != nil {
		return nil, fmt.Errorf("llm generation failed: %w", err)
	}

	// 7. Post-process citations and knowledge gaps
	cites := collectSources(genResp.Response, blocks, retrievalRes.Snippets)
	detectedGaps := gaps.MergeGaps(retrievalRes.Gaps, gaps.DetectGaps(genResp.Response))

	answer := &model.Answer{
		Question: question,
		Answer:   genResp.Response,
		Intent:   intent,
		Plan:     retrievalRes.Plan,
		Model:    genResp.Model,
		Provider: tgt.provider.Name(),
		Locality: tgt.locality,
		Sources:  cites,
		Gaps:     detectedGaps,
	}

	if session != nil {
		session.AddTurn(question, genResp.Response)
	}

	return answer, nil
}

// RetrieveOnly executes intent classification, query planning, hybrid retrieval, and context budgeting
// without calling the LLM provider. Useful for fast (<50 ms) retrieval inspection and debugging.
func (a *Assistant) RetrieveOnly(ctx context.Context, question string) (*model.Answer, error) {
	intent := a.classifier.Classify(question)
	retrievalRes, err := a.retriever.Retrieve(ctx, question, intent)
	if err != nil {
		return nil, fmt.Errorf("retrieval failed: %w", err)
	}

	blocks := a.contextBuilder.BuildContext(retrievalRes)
	formattedContext := a.contextBuilder.FormatContext(blocks)
	approxTokens := (len(formattedContext) + 2) / 3

	var summary strings.Builder
	if retrievalRes.Plan != nil {
		fmt.Fprintf(&summary, "[Retrieve-Only] Plan: %s (FTS=%v, Vectors=%v, Entities=%v, Graph=%v depth=%d)\n",
			retrievalRes.Plan.QueryType,
			retrievalRes.Plan.UseFTS,
			retrievalRes.Plan.UseVectors,
			retrievalRes.Plan.UseEntities,
			retrievalRes.Plan.UseGraph,
			retrievalRes.Plan.GraphDepth,
		)
	} else {
		fmt.Fprintf(&summary, "[Retrieve-Only] Intent: %s\n", intent)
	}
	fmt.Fprintf(&summary, "Context blocks: %d (~%d tokens) | Entities: %d | Relations: %d | Reasoning chains: %d",
		len(blocks), approxTokens, len(retrievalRes.Entities), len(retrievalRes.Relations), len(retrievalRes.Chains))
	for _, b := range blocks {
		if b.Source == "architecture-model" || (b.Source == "knowledge-graph" && b.Section == "reasoning-chains") {
			summary.WriteString("\n\n" + strings.TrimSpace(b.Content))
		}
	}

	return &model.Answer{
		Question: question,
		Answer:   summary.String(),
		Intent:   intent,
		Plan:     retrievalRes.Plan,
		Model:    "none (retrieve-only)",
		Sources:  collectSources("", blocks, retrievalRes.Snippets),
		Gaps:     retrievalRes.Gaps,
	}, nil
}

// collectSources merges explicit [Doc: ...] citations from the LLM answer with the document
// context blocks that were deterministically provided in the prompt budget.
func collectSources(response string, blocks []model.ContextBlock, snippets []model.DocumentSnippet) []model.SourceCitation {
	cites := citations.ExtractCitations(response)
	seen := make(map[string]bool, len(cites))

	for i, c := range cites {
		seen[c.Path+"#"+c.Section] = true
		seen[c.Path] = true
		for _, s := range snippets {
			if s.DocumentPath == c.Path && (c.Section == "" || s.SectionTitle == c.Section) {
				snip := contextbuilder.RedactSecrets(s.Content)
				if len(snip) > 200 {
					snip = snip[:200] + "..."
				}
				cites[i].Snippet = strings.TrimSpace(snip)
				if cites[i].Section == "" {
					cites[i].Section = s.SectionTitle
				}
				break
			}
		}
	}

	for _, b := range blocks {
		if b.Source == "" || b.Source == "knowledge-graph" || b.Source == "architecture-model" {
			continue
		}
		key := b.Source + "#" + b.Section
		if seen[key] || (b.Section == "" && seen[b.Source]) {
			continue
		}
		seen[key] = true
		seen[b.Source] = true
		cites = append(cites, model.SourceCitation{
			Path:    b.Source,
			Section: b.Section,
			Snippet: b.Snippet,
		})
	}
	return cites
}

// contextWindowStep rounds the window up so that small prompt size changes do not force Ollama to reload the model.
const contextWindowStep = 8192

// contextWindow sizes the model window for the prompt plus the generated answer.
// It assumes ~3 bytes per token, which is conservative for mixed German/English Markdown and code.
func contextWindow(systemPrompt, userPrompt string, maxTokens int) int {
	needed := (len(systemPrompt)+len(userPrompt))/3 + maxTokens
	return (needed/contextWindowStep + 1) * contextWindowStep
}

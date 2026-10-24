// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

package model

// Intent represents the classified intention of a user query.
type Intent string

const (
	IntentSearch       Intent = "search"
	IntentEntity       Intent = "entity"
	IntentRelationship Intent = "relationship"
	IntentADR          Intent = "adr"
	IntentImpact       Intent = "impact"
	IntentSummary      Intent = "summary"
	IntentCompare      Intent = "compare"
	IntentKnowledgeGap Intent = "knowledge_gap"
)

// DocumentSnippet represents a specific section or content slice of a document.
type DocumentSnippet struct {
	DocumentPath string   `json:"document_path"`
	Title        string   `json:"title"`
	SectionTitle string   `json:"section_title,omitempty"`
	Content      string   `json:"content"`
	Tags         []string `json:"tags,omitempty"`
	Score        float64  `json:"score,omitempty"`
	Strategy     string   `json:"strategy"` // "fts", "entity", "adr", "graph"
}

// EntityMatch represents an entity discovered in the knowledge base.
type EntityMatch struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	DocumentURI string `json:"document_uri"`
	Description string `json:"description,omitempty"`
}

// EntityRelationMatch represents a relationship between two entities.
type EntityRelationMatch struct {
	SourceEntity string `json:"source_entity"`
	RelationType string `json:"relation_type"`
	TargetEntity string `json:"target_entity"`
	DocumentURI  string `json:"document_uri"`
}

// KnowledgeGap represents a missing piece of documentation or architecture knowledge for an entity.
type KnowledgeGap struct {
	Entity string `json:"entity"`
	Reason string `json:"reason"`
}

// RetrievalPlanInfo exposes the executed retrieval plan for debugging and JSON output.
type RetrievalPlanInfo struct {
	QueryType   string `json:"query_type"`
	UseFTS      bool   `json:"use_fts"`
	UseEntities bool   `json:"use_entities"`
	UseGraph    bool   `json:"use_graph"`
	UseVectors  bool   `json:"use_vectors"`
	GraphDepth  int    `json:"graph_depth"`
	MaxResults  int    `json:"max_results"`
}

// ArchitectureContext summarizes architectural building blocks discovered during retrieval.
type ArchitectureContext struct {
	Services     []string              `json:"services,omitempty"`
	Components   []string              `json:"components,omitempty"`
	Pipelines    []string              `json:"pipelines,omitempty"`
	ADRs         []string              `json:"adrs,omitempty"`
	Entities     []string              `json:"entities,omitempty"`
	Dependencies []EntityRelationMatch `json:"dependencies,omitempty"`
}

// RetrievalResult holds all evidence retrieved from the knowledge base.
type RetrievalResult struct {
	Query        string                `json:"query"`
	Intent       Intent                `json:"intent"`
	Plan         *RetrievalPlanInfo    `json:"plan,omitempty"`
	Architecture *ArchitectureContext  `json:"architecture,omitempty"`
	Snippets     []DocumentSnippet     `json:"snippets"`
	Entities     []EntityMatch         `json:"entities"`
	Relations    []EntityRelationMatch `json:"relations"`
	Chains       []ReasoningChain      `json:"chains,omitempty"`
	Gaps         []KnowledgeGap        `json:"gaps,omitempty"`
}

// ReasoningStep is one directed edge of a reasoning chain, e.g. "Transformation Engine" USES "Encryption".
type ReasoningStep struct {
	Source   string `json:"source"`
	Relation string `json:"relation"`
	Target   string `json:"target"`
}

// ReasoningChain is a multi-hop path through the knowledge graph from a retrieved document to a related one.
type ReasoningChain struct {
	Steps []ReasoningStep `json:"steps"`
}

// ContextBlock represents a prioritized and formatted piece of evidence for prompt assembly.
type ContextBlock struct {
	Priority int     `json:"priority"` // 1 (Highest) to 6 (Lowest)
	Score    float64 `json:"score,omitempty"`
	Source   string  `json:"source"`
	Section  string  `json:"section,omitempty"`
	Snippet  string  `json:"snippet,omitempty"`
	Content  string  `json:"content"`
	Tokens   int     `json:"tokens"`
}

// GenerateRequest represents the request passed to an LLM provider.
type GenerateRequest struct {
	Model       string  `json:"model"`
	Prompt      string  `json:"prompt"`
	System      string  `json:"system,omitempty"`
	Temperature float64 `json:"temperature"`
	MaxTokens   int     `json:"max_tokens"`
	// ContextWindow is the total token window (prompt + output) the model must allocate; 0 keeps the provider default.
	ContextWindow int `json:"context_window,omitempty"`
}

// GenerateResponse represents the response received from an LLM provider.
type GenerateResponse struct {
	Response     string `json:"response"`
	Model        string `json:"model"`
	PromptTokens int    `json:"prompt_tokens,omitempty"`
	OutputTokens int    `json:"output_tokens,omitempty"`
}

// Model localities: local models run on this machine, cloud models send the prompt to a provider.
const (
	LocalityLocal = "local"
	LocalityCloud = "cloud"
)

// Model kinds: generation models answer questions, embedding models vectorise sections.
const (
	KindGeneration = "generation"
	KindEmbedding  = "embedding"
)

// ModelInfo describes a model offered by an LLM backend.
type ModelInfo struct {
	Name          string `json:"name"`
	Size          int64  `json:"size"`
	Family        string `json:"family,omitempty"`
	Provider      string `json:"provider,omitempty"`
	ProviderName  string `json:"provider_name,omitempty"`
	Locality      string `json:"locality,omitempty"`
	Kind          string `json:"kind,omitempty"`
	ParameterSize string `json:"parameter_size,omitempty"`
	// Loaded reports that a local model is currently held in memory by the daemon.
	Loaded bool `json:"loaded,omitempty"`
}

// ModelRef selects a model of a provider for one request.
type ModelRef struct {
	Provider string `json:"provider"`
	Name     string `json:"name"`
}

// SourceCitation represents a cited document or section in the answer.
type SourceCitation struct {
	Path    string `json:"path"`
	Section string `json:"section,omitempty"`
	Snippet string `json:"snippet,omitempty"`
}

// Answer represents the final grounded answer returned to the user or caller.
type Answer struct {
	Question string             `json:"question"`
	Answer   string             `json:"answer"`
	Intent   Intent             `json:"intent"`
	Plan     *RetrievalPlanInfo `json:"plan,omitempty"`
	Model    string             `json:"model"`
	Provider string             `json:"provider,omitempty"`
	Locality string             `json:"locality,omitempty"`
	Sources  []SourceCitation   `json:"sources"`
	Gaps     []KnowledgeGap     `json:"gaps,omitempty"`
}

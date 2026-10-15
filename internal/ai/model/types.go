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

// RetrievalResult holds all evidence retrieved from the knowledge base.
type RetrievalResult struct {
	Query     string                `json:"query"`
	Intent    Intent                `json:"intent"`
	Snippets  []DocumentSnippet     `json:"snippets"`
	Entities  []EntityMatch         `json:"entities"`
	Relations []EntityRelationMatch `json:"relations"`
	Chains    []ReasoningChain      `json:"chains,omitempty"`
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
	Priority int    `json:"priority"` // 1 (Highest) to 6 (Lowest)
	Source   string `json:"source"`
	Content  string `json:"content"`
	Tokens   int    `json:"tokens"`
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

// SourceCitation represents a cited document or section in the answer.
type SourceCitation struct {
	Path    string `json:"path"`
	Section string `json:"section,omitempty"`
	Snippet string `json:"snippet,omitempty"`
}

// Answer represents the final grounded answer returned to the user or caller.
type Answer struct {
	Question string           `json:"question"`
	Answer   string           `json:"answer"`
	Intent   Intent           `json:"intent"`
	Model    string           `json:"model"`
	Sources  []SourceCitation `json:"sources"`
	Gaps     []string         `json:"gaps,omitempty"`
}

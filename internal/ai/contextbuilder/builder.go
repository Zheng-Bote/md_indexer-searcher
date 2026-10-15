// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

package contextbuilder

import (
	"fmt"
	"sort"
	"strings"

	"github.com/md-indexer/internal/ai/model"
)

// ContextBuilder constructs prioritized and token-budgeted prompt contexts.
type ContextBuilder struct {
	maxTokens int
}

// NewContextBuilder creates a new ContextBuilder.
func NewContextBuilder(maxTokens int) *ContextBuilder {
	if maxTokens <= 0 {
		maxTokens = 16000
	}
	return &ContextBuilder{
		maxTokens: maxTokens,
	}
}

// BuildContext converts retrieved evidence into ordered blocks within the token budget.
func (b *ContextBuilder) BuildContext(res *model.RetrievalResult) []model.ContextBlock {
	var blocks []model.ContextBlock

	// 1. Entities (Priority 1)
	for _, e := range res.Entities {
		content := fmt.Sprintf("Entity: %s (Type: %s, Defined in: %s)\nDescription: %s", e.Name, e.Type, e.DocumentURI, e.Description)
		blocks = append(blocks, model.ContextBlock{
			Priority: 1,
			Source:   e.DocumentURI,
			Content:  content,
			Tokens:   estimateTokens(content),
		})
	}

	// 2. Relations (Priority 2)
	for _, r := range res.Relations {
		content := fmt.Sprintf("Relation: [%s] --(%s)--> [%s] (Source: %s)", r.SourceEntity, r.RelationType, r.TargetEntity, r.DocumentURI)
		blocks = append(blocks, model.ContextBlock{
			Priority: 2,
			Source:   r.DocumentURI,
			Content:  content,
			Tokens:   estimateTokens(content),
		})
	}

	// Reasoning chains (Priority 2): explicit multi-hop paths through the knowledge graph.
	for _, c := range res.Chains {
		var sb strings.Builder
		sb.WriteString("Reasoning chain:")
		for _, s := range c.Steps {
			sb.WriteString(fmt.Sprintf("\n- [%s] --(%s)--> [%s]", s.Source, s.Relation, s.Target))
		}
		content := sb.String()
		blocks = append(blocks, model.ContextBlock{
			Priority: 2,
			Source:   "knowledge-graph",
			Content:  content,
			Tokens:   estimateTokens(content),
		})
	}

	// 3. Document Snippets (Priority 3 to 6 based on strategy)
	for _, s := range res.Snippets {
		prio := 6 // Default FTS
		switch s.Strategy {
		case "entity":
			prio = 1
		case "adr":
			prio = 3
		case "graph":
			prio = 5
		}

		secHeader := ""
		if s.SectionTitle != "" {
			secHeader = fmt.Sprintf(" > %s", s.SectionTitle)
		}
		content := fmt.Sprintf("Document: %s%s\nTitle: %s\nContent:\n%s", s.DocumentPath, secHeader, s.Title, s.Content)
		blocks = append(blocks, model.ContextBlock{
			Priority: prio,
			Source:   s.DocumentPath,
			Content:  content,
			Tokens:   estimateTokens(content),
		})
	}

	// Sort blocks by Priority ascending (1 is highest priority)
	sort.SliceStable(blocks, func(i, j int) bool {
		return blocks[i].Priority < blocks[j].Priority
	})

	// Budget enforcement
	var budgeted []model.ContextBlock
	currentTokens := 0
	for _, block := range blocks {
		if currentTokens+block.Tokens > b.maxTokens {
			break
		}
		budgeted = append(budgeted, block)
		currentTokens += block.Tokens
	}

	return budgeted
}

// FormatContext formats context blocks into a string ready for prompt injection.
func (b *ContextBuilder) FormatContext(blocks []model.ContextBlock) string {
	if len(blocks) == 0 {
		return "No relevant context found in knowledge base."
	}

	var sb strings.Builder
	for i, block := range blocks {
		sb.WriteString(fmt.Sprintf("--- Source [%d]: %s ---\n", i+1, block.Source))
		sb.WriteString(block.Content)
		sb.WriteString("\n\n")
	}
	return strings.TrimSpace(sb.String())
}

// estimateTokens provides a fast, standard approximation (~4 characters per token).
func estimateTokens(s string) int {
	return (len(s) + 3) / 4
}

// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

package contextbuilder

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/md-indexer/internal/ai/model"
)

const nearDuplicateThreshold = 0.80

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
		content := RedactSecrets(fmt.Sprintf("Entity: %s (Type: %s, Defined in: %s)\nDescription: %s", e.Name, e.Type, e.DocumentURI, e.Description))
		blocks = append(blocks, model.ContextBlock{
			Priority: 1,
			Source:   e.DocumentURI,
			Content:  content,
			Tokens:   estimateTokens(content),
		})
	}

	// 2. Architecture Context (Priority 2)
	if archContent := formatArchitecture(res.Architecture); archContent != "" {
		archContent = RedactSecrets(archContent)
		blocks = append(blocks, model.ContextBlock{
			Priority: 2,
			Source:   "architecture-model",
			Content:  archContent,
			Tokens:   estimateTokens(archContent),
		})
	}

	// 3. Relations (Priority 2)
	for _, r := range res.Relations {
		content := RedactSecrets(fmt.Sprintf("Relation: [%s] --(%s)--> [%s] (Source: %s)", r.SourceEntity, r.RelationType, r.TargetEntity, r.DocumentURI))
		blocks = append(blocks, model.ContextBlock{
			Priority: 2,
			Source:   r.DocumentURI,
			Content:  content,
			Tokens:   estimateTokens(content),
		})
	}

	// 4. Reasoning chains (Priority 2): explicit multi-hop paths through the knowledge graph.
	for _, c := range res.Chains {
		var sb strings.Builder
		sb.WriteString("Reasoning chain:")
		for _, s := range c.Steps {
			sb.WriteString(fmt.Sprintf("\n- [%s] --(%s)--> [%s]", s.Source, s.Relation, s.Target))
		}
		content := RedactSecrets(sb.String())
		blocks = append(blocks, model.ContextBlock{
			Priority: 2,
			Source:   "knowledge-graph",
			Content:  content,
			Tokens:   estimateTokens(content),
		})
	}

	// 5. Document Snippets: prioritize direct entity docs (1) and ADRs (3),
	// keep general fused snippets (fts/vector/graph) in one tier (4) ordered by fused Score.
	var snippetBlocks []model.ContextBlock
	for _, s := range res.Snippets {
		prio := 4
		switch s.Strategy {
		case "entity":
			prio = 1
		case "adr":
			prio = 3
		}

		secHeader := ""
		if s.SectionTitle != "" {
			secHeader = fmt.Sprintf(" > %s", s.SectionTitle)
		}
		redactedBody := RedactSecrets(s.Content)
		content := fmt.Sprintf("Document: %s%s\nTitle: %s\nContent:\n%s", s.DocumentPath, secHeader, s.Title, redactedBody)
		snippetBlocks = append(snippetBlocks, model.ContextBlock{
			Priority: prio,
			Score:    s.Score,
			Source:   s.DocumentPath,
			Section:  s.SectionTitle,
			Snippet:  previewSnippet(redactedBody),
			Content:  content,
			Tokens:   estimateTokens(content),
		})
	}

	sort.SliceStable(snippetBlocks, func(i, j int) bool {
		if snippetBlocks[i].Priority != snippetBlocks[j].Priority {
			return snippetBlocks[i].Priority < snippetBlocks[j].Priority
		}
		return snippetBlocks[i].Score > snippetBlocks[j].Score
	})

	// Drop exact and near-duplicate snippets before budgeting.
	var keptTokenSets []map[string]struct{}
	for _, sb := range snippetBlocks {
		tset := tokenSet(sb.Snippet + " " + sb.Content)
		if isNearDuplicate(tset, keptTokenSets) {
			continue
		}
		keptTokenSets = append(keptTokenSets, tset)
		blocks = append(blocks, sb)
	}

	// Sort all blocks by Priority ascending, then Score descending (stable preserves order on ties).
	sort.SliceStable(blocks, func(i, j int) bool {
		if blocks[i].Priority != blocks[j].Priority {
			return blocks[i].Priority < blocks[j].Priority
		}
		return blocks[i].Score > blocks[j].Score
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

func formatArchitecture(arch *model.ArchitectureContext) string {
	if arch == nil {
		return ""
	}
	var lines []string
	if len(arch.Services) > 0 {
		lines = append(lines, "Services: "+strings.Join(arch.Services, ", "))
	}
	if len(arch.Components) > 0 {
		lines = append(lines, "Components: "+strings.Join(arch.Components, ", "))
	}
	if len(arch.Pipelines) > 0 {
		lines = append(lines, "Pipelines: "+strings.Join(arch.Pipelines, ", "))
	}
	if len(arch.ADRs) > 0 {
		lines = append(lines, "ADRs: "+strings.Join(arch.ADRs, ", "))
	}
	if len(arch.Entities) > 0 {
		lines = append(lines, "Entities: "+strings.Join(arch.Entities, ", "))
	}
	if len(arch.Dependencies) > 0 {
		var deps []string
		for _, d := range arch.Dependencies {
			deps = append(deps, fmt.Sprintf("[%s] --(%s)--> [%s]", d.SourceEntity, d.RelationType, d.TargetEntity))
		}
		lines = append(lines, "Dependencies:\n- "+strings.Join(deps, "\n- "))
	}
	if len(lines) == 0 {
		return ""
	}
	return "Architecture Context:\n" + strings.Join(lines, "\n")
}

func previewSnippet(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 200 {
		return s[:200] + "..."
	}
	return s
}

func tokenSet(s string) map[string]struct{} {
	words := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	set := make(map[string]struct{}, len(words))
	for _, w := range words {
		if len(w) >= 2 {
			set[w] = struct{}{}
		}
	}
	return set
}

func isNearDuplicate(candidate map[string]struct{}, existing []map[string]struct{}) bool {
	if len(candidate) == 0 {
		return false
	}
	for _, ex := range existing {
		if jaccard(candidate, ex) >= nearDuplicateThreshold {
			return true
		}
	}
	return false
}

func jaccard(a, b map[string]struct{}) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	inter := 0
	for k := range a {
		if _, ok := b[k]; ok {
			inter++
		}
	}
	union := len(a) + len(b) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

var (
	secretKeyValRe = regexp.MustCompile(`(?i)\b([a-z0-9_]*(?:api_?key|secret|token|password|passwd|access_key)[a-z0-9_]*)\s*([:=])\s*["']?([A-Za-z0-9_\-\.~+/=]{8,})["']?`)
	secretTokenRe  = regexp.MustCompile(`\b(sk-[A-Za-z0-9_\-]{12,}|gh[pousr]_[A-Za-z0-9]{16,}|AIza[0-9A-Za-z\-_]{20,})\b`)
	bearerRe       = regexp.MustCompile(`(?i)(\bAuthorization\s*:\s*Bearer\s+)([A-Za-z0-9_\-\.~+/=]{8,})`)
)

// RedactSecrets masks API keys, passwords, and bearer tokens before passing context to the LLM.
func RedactSecrets(s string) string {
	s = secretKeyValRe.ReplaceAllString(s, "${1}${2}[REDACTED]")
	s = secretTokenRe.ReplaceAllString(s, "[REDACTED]")
	s = bearerRe.ReplaceAllString(s, "${1}[REDACTED]")
	return s
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

// estimateTokens uses ~3 bytes per token, matching assistant.contextWindow for mixed German/English Markdown and code.
func estimateTokens(s string) int {
	return (len(s) + 2) / 3
}

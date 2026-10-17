// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

package contextbuilder

import (
	"strings"
	"testing"

	"github.com/md-indexer/internal/ai/model"
)

func TestContextBuilder_BuildContext(t *testing.T) {
	cb := NewContextBuilder(1000)

	res := &model.RetrievalResult{
		Query:  "sqlite",
		Intent: model.IntentADR,
		Entities: []model.EntityMatch{
			{Name: "SQLite", Type: "Database", DocumentURI: "docs/db.md", Description: "Embedded SQL"},
		},
		Relations: []model.EntityRelationMatch{
			{SourceEntity: "App", RelationType: "uses", TargetEntity: "SQLite", DocumentURI: "docs/app.md"},
		},
		Snippets: DocumentSnippetStub(),
	}

	blocks := cb.BuildContext(res)
	if len(blocks) < 3 {
		t.Fatalf("expected at least 3 context blocks, got %d", len(blocks))
	}

	// Verify priority ordering: Entity (1) -> Relation (2) -> ADR (3) -> Fused Snippets (4)
	if blocks[0].Priority != 1 {
		t.Errorf("expected highest priority 1, got %d", blocks[0].Priority)
	}
	if blocks[1].Priority != 2 {
		t.Errorf("expected priority 2, got %d", blocks[1].Priority)
	}

	formatted := cb.FormatContext(blocks)
	if formatted == "" {
		t.Errorf("expected formatted context, got empty string")
	}
}

func TestContextBuilder_Budgeting(t *testing.T) {
	// Tiny budget to force truncation
	cb := NewContextBuilder(50)

	res := &model.RetrievalResult{
		Entities: []model.EntityMatch{
			{Name: "E1", Type: "Type1", DocumentURI: "doc1.md", Description: "Short description"},
			{Name: "E2", Type: "Type2", DocumentURI: "doc2.md", Description: "Another description with many words to exceed token budget..."},
			{Name: "E3", Type: "Type3", DocumentURI: "doc3.md", Description: "Yet another long description..."},
		},
	}

	blocks := cb.BuildContext(res)
	if len(blocks) >= 3 {
		t.Errorf("expected budgeting to truncate blocks, got %d", len(blocks))
	}
}

func DocumentSnippetStub() []model.DocumentSnippet {
	return []model.DocumentSnippet{
		{DocumentPath: "docs/adr.md", Title: "ADR", Strategy: "adr", Content: "Decision details", Score: 0.5},
		{DocumentPath: "docs/fts.md", Title: "FTS", Strategy: "fts", Content: "Search match", Score: 0.4},
	}
}

func TestContextBuilder_ReasoningChains(t *testing.T) {
	res := &model.RetrievalResult{
		Chains: []model.ReasoningChain{{Steps: []model.ReasoningStep{
			{Source: "Transformation Engine", Relation: "USES", Target: "Encryption"},
			{Source: "Encryption", Relation: "DEPENDS_ON", Target: "KMS"},
		}}},
		Snippets: []model.DocumentSnippet{{DocumentPath: "a.md", Content: "x", Strategy: "fts"}},
	}
	blocks := NewContextBuilder(1000).BuildContext(res)
	if len(blocks) != 2 || blocks[0].Priority != 2 || blocks[0].Source != "knowledge-graph" {
		t.Fatalf("expected chain block first with priority 2, got %+v", blocks)
	}
	want := "Reasoning chain:\n- [Transformation Engine] --(USES)--> [Encryption]\n- [Encryption] --(DEPENDS_ON)--> [KMS]"
	if blocks[0].Content != want {
		t.Errorf("unexpected chain content:\n%s", blocks[0].Content)
	}
}

func TestContextBuilder_PreservesScoreOrderAcrossStrategiesAndDropsNearDuplicates(t *testing.T) {
	res := &model.RetrievalResult{
		Snippets: []model.DocumentSnippet{
			{DocumentPath: "low-fts.md", Title: "Low FTS", SectionTitle: "A", Content: "distinct alpha beta gamma content", Score: 0.20, Strategy: "fts"},
			{DocumentPath: "high-graph.md", Title: "High Graph", SectionTitle: "B", Content: "completely different delta epsilon zeta", Score: 0.80, Strategy: "graph"},
			{DocumentPath: "high-graph-dup.md", Title: "High Graph", SectionTitle: "B", Content: "completely different delta epsilon zeta", Score: 0.75, Strategy: "vector"},
		},
	}
	blocks := NewContextBuilder(1000).BuildContext(res)
	if len(blocks) != 2 {
		t.Fatalf("expected near-duplicate to be dropped (2 blocks left), got %d", len(blocks))
	}
	if blocks[0].Source != "high-graph.md" || blocks[1].Source != "low-fts.md" {
		t.Errorf("expected score order high-graph.md then low-fts.md, got %s then %s", blocks[0].Source, blocks[1].Source)
	}
}

func TestContextBuilder_ArchitectureAndSecretRedaction(t *testing.T) {
	res := &model.RetrievalResult{
		Architecture: &model.ArchitectureContext{
			Services:   []string{"Encryption Service"},
			Components: []string{"REST Delivery"},
			Pipelines:  []string{"Ingestion Pipeline"},
		},
		Snippets: []model.DocumentSnippet{
			{
				DocumentPath: "AI/Ollama Ai.md",
				Title:        "Ollama",
				SectionTitle: "Setup",
				Content:      "export OLLAMA_API_KEY=abcdef1234567890secret and token sk-proj1234567890abcdef",
				Score:        0.9,
				Strategy:     "fts",
			},
		},
	}
	cb := NewContextBuilder(1000)
	blocks := cb.BuildContext(res)
	formatted := cb.FormatContext(blocks)
	if !strings.Contains(formatted, "Architecture Context:") || !strings.Contains(formatted, "Services: Encryption Service") {
		t.Errorf("expected Architecture Context in formatted output, got:\n%s", formatted)
	}
	if strings.Contains(formatted, "abcdef1234567890secret") || strings.Contains(formatted, "sk-proj1234567890abcdef") {
		t.Errorf("expected secrets to be redacted, got:\n%s", formatted)
	}
	if !strings.Contains(formatted, "OLLAMA_API_KEY=[REDACTED]") {
		t.Errorf("expected OLLAMA_API_KEY=[REDACTED], got:\n%s", formatted)
	}
}

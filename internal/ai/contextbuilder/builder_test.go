// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

package contextbuilder

import (
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

	// Verify priority ordering: Entity (1) -> Relation (2) -> ADR (3) -> Graph (5) -> FTS (6)
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
		{DocumentPath: "docs/adr.md", Title: "ADR", Strategy: "adr", Content: "Decision details"},
		{DocumentPath: "docs/fts.md", Title: "FTS", Strategy: "fts", Content: "Search match"},
	}
}

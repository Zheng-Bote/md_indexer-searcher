// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

package retriever

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/md-indexer/internal/ai/model"
	"github.com/md-indexer/internal/entities"
	"github.com/md-indexer/internal/repository"
)

// setupTestDB builds the database through the real indexer migration and write path,
// so any drift between the retriever queries and the indexer schema fails the tests.
func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	// A file-backed DB keeps all pooled connections on the same database.
	repo, err := repository.NewSQLiteRepository(filepath.Join(t.TempDir(), "knowledge.db"))
	if err != nil {
		t.Fatalf("failed to init repository: %v", err)
	}
	t.Cleanup(func() { repo.Close() })

	docs := []*entities.Document{
		{
			ID:      "doc-adr",
			Path:    "docs/adr/001-sqlite.md",
			Title:   "ADR 001: SQLite Choice",
			Content: "Context\nWe chose modernc sqlite because it is CGO-free.",
			Sections: []entities.Section{
				{ID: "doc-adr-title", Heading: "ADR 001: SQLite Choice", Level: 1, Content: ""},
				{ID: "doc-adr-context", Heading: "Context", Level: 2, Content: "We chose modernc sqlite because it is CGO-free."},
			},
		},
		{
			ID:      "doc-parser",
			Path:    "docs/components/parser.md",
			Title:   "Goldmark Parser",
			Content: "Architecture\nThe parser extracts sections and links. See [[ADR 001: SQLite Choice]].",
			Sections: []entities.Section{
				{ID: "doc-parser-arch", Heading: "Architecture", Level: 2, Content: "The parser extracts sections and links. See [[ADR 001: SQLite Choice]]."},
			},
			Links:            []entities.Link{{SourceDocID: "doc-parser", TargetDocID: "ADR 001: SQLite Choice"}},
			IsEntity:         true,
			EntityType:       "Component",
			EntityName:       "Goldmark Parser",
			EntityProperties: map[string]string{"description": "Markdown AST Parser"},
			EntityRelations: []entities.EntityRelation{
				{SourceID: "Goldmark Parser", TargetID: "modernc", Type: "uses"},
			},
		},
		{
			ID:               "doc-modernc",
			Path:             "docs/libs/modernc.md",
			Title:            "modernc",
			Content:          "Overview\nCGO-free sqlite engine.",
			Sections:         []entities.Section{{ID: "doc-modernc-overview", Heading: "Overview", Level: 2, Content: "CGO-free sqlite engine."}},
			IsEntity:         true,
			EntityType:       "Library",
			EntityName:       "modernc",
			EntityProperties: map[string]string{"description": "CGO-free sqlite engine"},
		},
		{
			ID:       "doc-crypto",
			Path:     "docs/security/crypto.md",
			Title:    "Verschlüsselung",
			Content:  "Konzept\nDie Verschlüsselung erfolgt mit AES-GCM.",
			Sections: []entities.Section{{ID: "doc-crypto-concept", Heading: "Konzept", Level: 2, Content: "Die Verschlüsselung erfolgt mit AES-GCM."}},
		},
	}
	for _, d := range docs {
		if err := repo.UpsertDocument(context.Background(), d); err != nil {
			t.Fatalf("failed to upsert %s: %v", d.Path, err)
		}
	}

	return repo.GetDB()
}

func TestRetriever_Retrieve(t *testing.T) {
	db := setupTestDB(t)

	r := NewRetriever(db, 3)
	ctx := context.Background()

	res, err := r.Retrieve(ctx, "sqlite cgo modernc", model.IntentADR)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(res.Snippets) == 0 {
		t.Errorf("expected snippets for sqlite query, got 0")
	}

	if len(res.Entities) == 0 {
		t.Errorf("expected entities to be matched, got 0")
	}

	if len(res.Relations) == 0 {
		t.Errorf("expected relations to be matched, got 0")
	}

	foundADR := false
	for _, snip := range res.Snippets {
		if snip.Strategy == "adr" {
			foundADR = true
		}
		if snip.Content == "" {
			t.Errorf("expected no empty snippets, got one for %s#%s", snip.DocumentPath, snip.SectionTitle)
		}
	}
	if !foundADR {
		t.Errorf("expected at least one snippet marked with strategy 'adr'")
	}

	// Test fallback / empty query
	emptyRes, err := r.Retrieve(ctx, "", model.IntentSearch)
	if err != nil {
		t.Fatalf("unexpected error on empty query: %v", err)
	}
	if len(emptyRes.Snippets) != 0 {
		t.Errorf("expected 0 snippets on empty query")
	}

	// Test LIKE fallback search explicitly
	fallbackRes := &model.RetrievalResult{}
	err = r.fallbackLikeSearch(ctx, "Choice", fallbackRes)
	if err != nil {
		t.Fatalf("fallbackLikeSearch failed: %v", err)
	}
	if len(fallbackRes.Snippets) == 0 {
		t.Errorf("expected fallback LIKE results, got 0")
	}
}

func TestRetriever_EntityResolution(t *testing.T) {
	db := setupTestDB(t)
	r := NewRetriever(db, 3)

	res, err := r.Retrieve(context.Background(), "What is the Goldmark Parser component?", model.IntentEntity)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parser *model.EntityMatch
	count := 0
	for i, e := range res.Entities {
		if e.Name == "Goldmark Parser" {
			parser = &res.Entities[i]
			count++
		}
		if e.Type == "unresolved" {
			t.Errorf("unresolved stub entity %q must not be reported", e.Name)
		}
	}
	if parser == nil {
		t.Fatalf("expected entity 'Goldmark Parser', got %+v", res.Entities)
	}
	if count != 1 {
		t.Errorf("expected entity reported once, got %d", count)
	}
	if parser.Type != "Component" {
		t.Errorf("expected type 'Component', got %q", parser.Type)
	}
	if parser.DocumentURI != "docs/components/parser.md" {
		t.Errorf("expected document 'docs/components/parser.md', got %q", parser.DocumentURI)
	}
	if parser.Description != "Markdown AST Parser" {
		t.Errorf("expected description 'Markdown AST Parser', got %q", parser.Description)
	}

	foundUses := false
	for _, rel := range res.Relations {
		if rel.SourceEntity == "Goldmark Parser" && rel.RelationType == "uses" && rel.TargetEntity == "modernc" {
			foundUses = true
			if rel.DocumentURI != "docs/components/parser.md" {
				t.Errorf("expected relation document 'docs/components/parser.md', got %q", rel.DocumentURI)
			}
		}
	}
	if !foundUses {
		t.Errorf("expected relation Goldmark Parser -uses-> modernc, got %+v", res.Relations)
	}
}

func TestRetriever_GraphNeighbors(t *testing.T) {
	db := setupTestDB(t)
	r := NewRetriever(db, 3)

	res := &model.RetrievalResult{
		Snippets: []model.DocumentSnippet{{DocumentPath: "docs/adr/001-sqlite.md"}},
	}
	if err := r.retrieveGraphNeighbors(context.Background(), res); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, s := range res.Snippets {
		if s.Strategy == "graph" && s.DocumentPath == "docs/components/parser.md" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected backlinked document 'docs/components/parser.md' via graph, got %+v", res.Snippets)
	}
}

func TestRetriever_UnicodeQuery(t *testing.T) {
	db := setupTestDB(t)
	r := NewRetriever(db, 3)

	res, err := r.Retrieve(context.Background(), "Wie funktioniert die Verschlüsselung?", model.IntentSearch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, s := range res.Snippets {
		if s.DocumentPath == "docs/security/crypto.md" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected umlaut query to match 'docs/security/crypto.md', got %+v", res.Snippets)
	}
}

func TestSanitizeFTSQuery(t *testing.T) {
	got := sanitizeFTSQuery("Was ist die Verschlüsselung? (ADR-003)")
	want := "Was OR ist OR die OR Verschlüsselung OR ADR OR 003"
	if got != want {
		t.Errorf("sanitizeFTSQuery() = %q, want %q", got, want)
	}
}

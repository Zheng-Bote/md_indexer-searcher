// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

package hybrid

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/md-indexer/internal/ai/model"
	"github.com/md-indexer/internal/entities"
	"github.com/md-indexer/internal/repository"
)

var testWeights = Weights{FTS: 0.35, Graph: 0.20, Entity: 0.10}

// setupDB builds knowledge.db through the real indexer migration and write path.
func setupDB(t *testing.T) *sql.DB {
	t.Helper()
	repo, err := repository.NewSQLiteRepository(filepath.Join(t.TempDir(), "knowledge.db"))
	if err != nil {
		t.Fatalf("failed to init repository: %v", err)
	}
	t.Cleanup(func() { repo.Close() })

	docs := []*entities.Document{
		{
			ID:    "doc-enc",
			Path:  "docs/components/encryption-service.md",
			Title: "Encryption Service",
			Tags:  []string{"security"},
			Sections: []entities.Section{
				{ID: "enc-title", Heading: "Encryption Service", Level: 1, Content: ""},
				{ID: "enc-overview", Heading: "Overview", Level: 2, Content: "Encrypts packages with AES-GCM envelope encryption."},
				{ID: "enc-api", Heading: "API", Level: 2, Content: "REST endpoint for key rotation."},
			},
			IsEntity:         true,
			EntityType:       "Component",
			EntityName:       "Encryption Service",
			EntityProperties: map[string]string{"description": "Envelope encryption component"},
			EntityRelations:  []entities.EntityRelation{{SourceID: "Encryption Service", TargetID: "KMS", Type: "uses"}},
		},
		{
			ID:    "doc-delivery",
			Path:  "docs/components/rest-delivery.md",
			Title: "REST Delivery",
			Sections: []entities.Section{
				{ID: "del-flow", Heading: "Flow", Level: 2, Content: "Delivers encrypted packages. See [[Encryption Service]]."},
			},
			Links: []entities.Link{{SourceDocID: "doc-delivery", TargetDocID: "Encryption Service"}},
		},
		{
			ID:    "doc-ingest",
			Path:  "docs/pipelines/ingestion.md",
			Title: "Ingestion Pipeline",
			Sections: []entities.Section{
				{ID: "ing-steps", Heading: "Steps", Level: 2, Content: "Collect, transform, package and hand over to [[REST Delivery]]."},
			},
			Links: []entities.Link{{SourceDocID: "doc-ingest", TargetDocID: "REST Delivery"}},
		},
		{
			ID:    "doc-noise",
			Path:  "notes/howto.md",
			Title: "Wie ich die Dinge mache",
			Sections: []entities.Section{
				{ID: "noise-1", Heading: "Allgemein", Level: 2, Content: "Wie ich die Dinge mache, wenn ich sie mache."},
			},
		},
	}
	for _, d := range docs {
		if err := repo.UpsertDocument(context.Background(), d); err != nil {
			t.Fatalf("failed to upsert %s: %v", d.Path, err)
		}
	}
	return repo.GetDB()
}

func paths(res *model.RetrievalResult) map[string]string {
	out := make(map[string]string)
	for _, s := range res.Snippets {
		out[s.DocumentPath+"#"+s.SectionTitle] = s.Strategy
	}
	return out
}

func TestRetriever_StopwordsDoNotMatch(t *testing.T) {
	r := NewSQLiteRetriever(setupDB(t), 3, nil, testWeights, 25, 25)

	res, err := r.Retrieve(context.Background(), "Wie verschlüssele ich mit der Encryption?", model.IntentSearch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := paths(res)
	if _, ok := got["notes/howto.md#Allgemein"]; ok {
		t.Errorf("stopword-only document must not be retrieved, got %v", got)
	}
	if _, ok := got["docs/components/encryption-service.md#Overview"]; !ok {
		t.Errorf("expected encryption overview, got %v", got)
	}
}

func TestRetriever_EntityQuery(t *testing.T) {
	r := NewSQLiteRetriever(setupDB(t), 3, nil, testWeights, 25, 25)

	res, err := r.Retrieve(context.Background(), "Was ist der Encryption Service?", model.IntentEntity)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Entities) != 1 || res.Entities[0].Name != "Encryption Service" {
		t.Fatalf("expected entity 'Encryption Service', got %+v", res.Entities)
	}
	e := res.Entities[0]
	if e.Type != "Component" || e.DocumentURI != "docs/components/encryption-service.md" || e.Description != "Envelope encryption component" {
		t.Errorf("unexpected entity details: %+v", e)
	}
	if len(res.Relations) != 1 || res.Relations[0].TargetEntity != "KMS" {
		t.Errorf("expected relation to KMS, got %+v", res.Relations)
	}
	if len(res.Snippets) == 0 || res.Snippets[0].DocumentPath != "docs/components/encryption-service.md" {
		t.Errorf("expected encryption service section ranked first, got %+v", res.Snippets)
	}
	for _, s := range res.Snippets {
		if s.Score <= 0 {
			t.Errorf("expected fused score > 0, got %v for %s", s.Score, s.DocumentPath)
		}
	}
}

func TestRetriever_ArchitectureQueryExpandsGraph(t *testing.T) {
	r := NewSQLiteRetriever(setupDB(t), 3, nil, testWeights, 25, 25)

	res, err := r.Retrieve(context.Background(), "Wie funktioniert die Ingestion Pipeline?", model.IntentSearch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := paths(res)
	if got["docs/pipelines/ingestion.md#Steps"] != SourceFTS {
		t.Errorf("expected ingestion steps via fts, got %v", got)
	}
	// Two hops: Ingestion -> REST Delivery -> Encryption Service.
	if got["docs/components/rest-delivery.md#Flow"] != SourceGraph {
		t.Errorf("expected REST delivery via graph, got %v", got)
	}
	if got["docs/components/encryption-service.md#Overview"] != SourceGraph {
		t.Errorf("expected encryption service via 2-hop graph, got %v", got)
	}
}

func TestRetriever_FactQuerySkipsGraph(t *testing.T) {
	r := NewSQLiteRetriever(setupDB(t), 3, nil, testWeights, 25, 25)

	res, err := r.Retrieve(context.Background(), "Was ist die Ingestion Pipeline?", model.IntentADR)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, s := range res.Snippets {
		if s.Strategy == SourceGraph {
			t.Errorf("fact plan must not expand the graph, got %s", s.DocumentPath)
		}
	}
	if len(res.Entities) != 0 {
		t.Errorf("fact plan must not run entity retrieval, got %+v", res.Entities)
	}
}

func TestRetriever_EmptyQuery(t *testing.T) {
	r := NewSQLiteRetriever(setupDB(t), 3, nil, testWeights, 25, 25)
	res, err := r.Retrieve(context.Background(), "wie ich die", model.IntentSearch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Snippets) != 0 {
		t.Errorf("expected no snippets for stopword-only query, got %d", len(res.Snippets))
	}
}

func TestSQLiteFTS_FallbackWithoutSectionIndex(t *testing.T) {
	db := setupDB(t)
	if _, err := db.Exec("DROP TABLE fts_sections"); err != nil {
		t.Fatalf("failed to drop fts_sections: %v", err)
	}
	cands, err := NewSQLiteFTS(db).Search(context.Background(), []string{"encryption"}, 10)
	if err != nil {
		t.Fatalf("fallback search failed: %v", err)
	}
	found := false
	for _, c := range cands {
		if c.SectionID == "enc-overview" && c.FTSScore > 0 {
			found = true
		}
		if c.Content == "" {
			t.Errorf("fallback must skip empty sections, got %s", c.SectionID)
		}
	}
	if !found {
		t.Errorf("expected enc-overview from fts_documents fallback, got %+v", cands)
	}
}

func TestSQLiteFTS_TitleOutranksContent(t *testing.T) {
	cands, err := NewSQLiteFTS(setupDB(t)).Search(context.Background(), []string{"delivery"}, 10)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(cands) < 2 {
		t.Fatalf("expected title and content matches, got %+v", cands)
	}
	// "REST Delivery" title match must outrank the ingestion section that only mentions it in content.
	if cands[0].DocumentID != "doc-delivery" {
		t.Errorf("expected title match first, got %s", cands[0].DocumentID)
	}
}

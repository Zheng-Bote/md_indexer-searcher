// SPDX-License-Identifier: Apache-2.0

package search

import (
	"context"
	"testing"
	"time"

	"github.com/md-indexer/internal/entities"
	"github.com/md-indexer/internal/repository"
)

func TestSearchEngine_AllMethods(t *testing.T) {
	repo, err := repository.NewSQLiteRepository(":memory:")
	if err != nil {
		t.Fatalf("Failed to initialize repo: %v", err)
	}
	defer repo.Close()
	ctx := context.Background()

	doc1 := &entities.Document{
		ID:         "doc-service-a",
		Path:       "/vault/ServiceA.md",
		Title:      "ServiceA",
		Content:    "ServiceA handles authentication and validation.",
		Hash:       "hash-sa",
		ModifiedAt: time.Now(),
		IndexedAt:  time.Now(),
		IsEntity:   true,
		EntityType: "microservice",
		EntityName: "ServiceA",
		EntityProperties: map[string]string{
			"team": "core",
		},
		EntityRelations: []entities.EntityRelation{
			{SourceID: "ServiceA", TargetID: "ServiceB", Type: "USES"},
		},
		Links: []entities.Link{
			{SourceDocID: "doc-service-a", TargetDocID: "doc-service-b"},
		},
	}

	doc2 := &entities.Document{
		ID:         "doc-service-b",
		Path:       "/vault/ServiceB.md",
		Title:      "ServiceB",
		Content:    "ServiceB handles database storage and archiving.",
		Hash:       "hash-sb",
		ModifiedAt: time.Now(),
		IndexedAt:  time.Now(),
		IsEntity:   true,
		EntityType: "storage",
		EntityName: "ServiceB",
	}

	for _, d := range []*entities.Document{doc1, doc2} {
		if err := repo.UpsertDocument(ctx, d); err != nil {
			t.Fatalf("Upsert failed: %v", err)
		}
	}

	engine := NewSearchEngine(repo)

	// 1. Search
	res, err := engine.Search(ctx, "authentication")
	if err != nil || len(res) == 0 {
		t.Errorf("Search failed or returned empty: %v", err)
	} else if res[0].Title != "ServiceA" {
		t.Errorf("Expected match for ServiceA, got %s", res[0].Title)
	}

	// 2. FindDocument
	foundDoc, err := engine.FindDocument(ctx, "ServiceA")
	if err != nil || foundDoc == nil {
		t.Fatalf("FindDocument failed: %v", err)
	}
	if foundDoc.ID != "doc-service-a" {
		t.Errorf("Expected ID doc-service-a, got %s", foundDoc.ID)
	}

	// 3. FindEntity
	foundEnt, err := engine.FindEntity(ctx, "ServiceA")
	if err != nil || foundEnt == nil {
		t.Fatalf("FindEntity failed: %v", err)
	}
	if foundEnt.Type != "microservice" || foundEnt.Properties["team"] != "core" {
		t.Errorf("Expected type microservice and team core, got %+v", foundEnt)
	}

	// 4. Backlinks (ServiceA links to ServiceB)
	backlinks, err := engine.Backlinks(ctx, "ServiceB")
	if err != nil {
		t.Fatalf("Backlinks failed: %v", err)
	}
	if len(backlinks) != 1 || backlinks[0] != "ServiceA" {
		t.Errorf("Expected backlink from ServiceA, got %+v", backlinks)
	}

	// 5. Relations
	rels, err := engine.Relations(ctx, "ServiceA")
	if err != nil {
		t.Fatalf("Relations failed: %v", err)
	}
	if len(rels) != 1 || rels[0].TargetID != "ServiceB" {
		t.Errorf("Expected relation to ServiceB, got %+v", rels)
	}

	// 6. Impact
	impactEdges, err := engine.Impact(ctx, "ServiceB")
	if err != nil {
		t.Fatalf("Impact failed: %v", err)
	}
	if len(impactEdges) != 1 || impactEdges[0].Source != "ServiceA" {
		t.Errorf("Expected impact edge from ServiceA, got %+v", impactEdges)
	}

	// 7. Graph
	graphEdges, err := engine.Graph(ctx, "ServiceA")
	if err != nil {
		t.Fatalf("Graph failed: %v", err)
	}
	if len(graphEdges) != 1 || graphEdges[0].Target != "ServiceB" {
		t.Errorf("Expected graph edge to ServiceB, got %+v", graphEdges)
	}

	// 8. Non-existent entity queries return nil/empty without error
	nonExistent, err := engine.FindEntity(ctx, "GhostEntity")
	if err != nil || nonExistent != nil {
		t.Errorf("Expected nil for GhostEntity, got %v, err: %v", nonExistent, err)
	}
	emptyRels, err := engine.Relations(ctx, "GhostEntity")
	if err != nil || len(emptyRels) != 0 {
		t.Errorf("Expected nil/empty rels for GhostEntity, got %+v, err: %v", emptyRels, err)
	}
}

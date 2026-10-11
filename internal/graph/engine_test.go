// SPDX-License-Identifier: Apache-2.0

package graph

import (
	"context"
	"testing"
	"time"

	"github.com/md-indexer/internal/entities"
	"github.com/md-indexer/internal/repository"
)

func setupTestGraph(t *testing.T) (*repository.SQLiteRepository, GraphEngine) {
	repo, err := repository.NewSQLiteRepository(":memory:")
	if err != nil {
		t.Fatalf("Failed to initialize in-memory SQLite: %v", err)
	}

	// Setup graph:
	// App -> Gateway -> AuthDB
	// Worker -> Gateway
	ctx := context.Background()

	appDoc := &entities.Document{
		ID:         "doc-app",
		Path:       "/vault/App.md",
		Title:      "App",
		Hash:       "hash-app",
		ModifiedAt: time.Now(),
		IndexedAt:  time.Now(),
		IsEntity:   true,
		EntityType: "component",
		EntityName: "App",
		EntityRelations: []entities.EntityRelation{
			{SourceID: "App", TargetID: "Gateway", Type: "CALLS"},
		},
	}

	workerDoc := &entities.Document{
		ID:         "doc-worker",
		Path:       "/vault/Worker.md",
		Title:      "Worker",
		Hash:       "hash-worker",
		ModifiedAt: time.Now(),
		IndexedAt:  time.Now(),
		IsEntity:   true,
		EntityType: "component",
		EntityName: "Worker",
		EntityRelations: []entities.EntityRelation{
			{SourceID: "Worker", TargetID: "Gateway", Type: "CALLS"},
		},
	}

	gatewayDoc := &entities.Document{
		ID:         "doc-gateway",
		Path:       "/vault/Gateway.md",
		Title:      "Gateway",
		Hash:       "hash-gateway",
		ModifiedAt: time.Now(),
		IndexedAt:  time.Now(),
		IsEntity:   true,
		EntityType: "service",
		EntityName: "Gateway",
		EntityRelations: []entities.EntityRelation{
			{SourceID: "Gateway", TargetID: "AuthDB", Type: "CONNECTS_TO"},
		},
	}

	authDoc := &entities.Document{
		ID:         "doc-authdb",
		Path:       "/vault/AuthDB.md",
		Title:      "AuthDB",
		Hash:       "hash-authdb",
		ModifiedAt: time.Now(),
		IndexedAt:  time.Now(),
		IsEntity:   true,
		EntityType: "database",
		EntityName: "AuthDB",
	}

	for _, d := range []*entities.Document{appDoc, workerDoc, gatewayDoc, authDoc} {
		if err := repo.UpsertDocument(ctx, d); err != nil {
			t.Fatalf("UpsertDocument failed for %s: %v", d.Title, err)
		}
	}

	engine := NewGraphEngine(repo)
	return repo, engine
}

func TestGraphEngine_DirectAndReverseRelations(t *testing.T) {
	repo, engine := setupTestGraph(t)
	defer repo.Close()
	ctx := context.Background()

	// Direct relations from App (App -> Gateway)
	direct, err := engine.DirectRelations(ctx, "App")
	if err != nil {
		t.Fatalf("DirectRelations failed: %v", err)
	}
	if len(direct) != 1 || direct[0].TargetID != "Gateway" || direct[0].Type != "CALLS" {
		t.Errorf("Expected 1 relation to Gateway CALLS, got %+v", direct)
	}

	// Reverse relations to Gateway (App -> Gateway, Worker -> Gateway)
	reverse, err := engine.ReverseRelations(ctx, "Gateway")
	if err != nil {
		t.Fatalf("ReverseRelations failed: %v", err)
	}
	if len(reverse) != 2 {
		t.Errorf("Expected 2 incoming relations for Gateway, got %d: %+v", len(reverse), reverse)
	}
}

func TestGraphEngine_DependencyTree(t *testing.T) {
	repo, engine := setupTestGraph(t)
	defer repo.Close()
	ctx := context.Background()

	// DependencyTree for App: App -> Gateway -> AuthDB
	edges, err := engine.DependencyTree(ctx, "App")
	if err != nil {
		t.Fatalf("DependencyTree failed: %v", err)
	}

	if len(edges) != 2 {
		t.Fatalf("Expected 2 edges in dependency tree for App, got %d: %+v", len(edges), edges)
	}

	// Edge 1: App -> Gateway
	if edges[0].Source != "App" || edges[0].Target != "Gateway" {
		t.Errorf("Expected edge App -> Gateway, got %+v", edges[0])
	}
	// Edge 2: Gateway -> AuthDB
	if edges[1].Source != "Gateway" || edges[1].Target != "AuthDB" {
		t.Errorf("Expected edge Gateway -> AuthDB, got %+v", edges[1])
	}
}

func TestGraphEngine_ImpactAnalysis(t *testing.T) {
	repo, engine := setupTestGraph(t)
	defer repo.Close()
	ctx := context.Background()

	// ImpactAnalysis for AuthDB: Gateway depends on AuthDB, and App & Worker depend on Gateway
	edges, err := engine.ImpactAnalysis(ctx, "AuthDB")
	if err != nil {
		t.Fatalf("ImpactAnalysis failed: %v", err)
	}

	// We expect 3 edges: Gateway -> AuthDB, App -> Gateway, Worker -> Gateway
	if len(edges) != 3 {
		t.Fatalf("Expected 3 reverse impact edges, got %d: %+v", len(edges), edges)
	}
}

func TestGraphEngine_ShortestPath(t *testing.T) {
	repo, engine := setupTestGraph(t)
	defer repo.Close()
	ctx := context.Background()

	// ShortestPath from App to AuthDB: App -> Gateway -> AuthDB
	path, err := engine.ShortestPath(ctx, "App", "AuthDB")
	if err != nil {
		t.Fatalf("ShortestPath failed: %v", err)
	}

	if len(path) != 3 || path[0] != "App" || path[1] != "Gateway" || path[2] != "AuthDB" {
		t.Errorf("Expected path [App, Gateway, AuthDB], got %+v", path)
	}

	// ShortestPath when no path exists
	noPath, err := engine.ShortestPath(ctx, "AuthDB", "App")
	if err != nil {
		t.Fatalf("ShortestPath error for non-existent path: %v", err)
	}
	if len(noPath) != 0 {
		t.Errorf("Expected empty path, got %+v", noPath)
	}
}

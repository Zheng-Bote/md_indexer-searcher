// SPDX-License-Identifier: Apache-2.0

package examples

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/md-indexer/internal/repository"
	"github.com/md-indexer/internal/search"
	"github.com/md-indexer/internal/service"
)

func TestPhysicalVaultUseCase(t *testing.T) {
	ctx := context.Background()

	// 1. Initialize an in-memory SQLite database
	repo, err := repository.NewSQLiteRepository(":memory:")
	if err != nil {
		t.Fatalf("Failed to initialize SQLite repository: %v", err)
	}
	defer repo.Close()

	// 2. Index the physically existing markdown vault in ./vault
	vaultDir, err := filepath.Abs("vault")
	if err != nil {
		t.Fatalf("Failed to resolve vault path: %v", err)
	}

	indexer := service.NewIndexerService(repo, vaultDir)
	if err := indexer.IndexAll(ctx); err != nil {
		t.Fatalf("IndexAll on physical vault failed: %v", err)
	}

	// 3. Verify all 3 physical documents were indexed
	allDocs, err := repo.GetAllDocuments(ctx)
	if err != nil {
		t.Fatalf("GetAllDocuments failed: %v", err)
	}
	if len(allDocs) != 3 {
		t.Fatalf("Expected 3 indexed documents, got %d", len(allDocs))
	}

	searchEngine := search.NewSearchEngine(repo)

	// 4. Test Full-Text Search (FTS5 BM25) on physical document contents
	t.Run("FTS5 Full-Text Search", func(t *testing.T) {
		res, err := searchEngine.Search(ctx, "Ed25519")
		if err != nil {
			t.Fatalf("Search failed: %v", err)
		}
		if len(res) == 0 || res[0].Title != "TokenSigner" {
			t.Errorf("Expected search match for 'TokenSigner', got %+v", res)
		}
	})

	// 5. Test Entity Metadata & Properties extraction from Frontmatter
	t.Run("Entity Metadata Extraction", func(t *testing.T) {
		ent, err := searchEngine.FindEntity(ctx, "AuthService")
		if err != nil || ent == nil {
			t.Fatalf("FindEntity failed for AuthService: %v", err)
		}
		if ent.Type != "service" {
			t.Errorf("Expected entity type 'service', got '%s'", ent.Type)
		}
		if ent.Properties["owner"] != "security-team" {
			t.Errorf("Expected owner 'security-team', got '%s'", ent.Properties["owner"])
		}
	})

	// 6. Test Graph Dependency Tree (outgoing relations)
	t.Run("Graph Dependency Tree", func(t *testing.T) {
		edges, err := searchEngine.Graph(ctx, "AuthService")
		if err != nil {
			t.Fatalf("Graph traversal failed: %v", err)
		}
		// AuthService uses UserDatabase and TokenSigner
		if len(edges) < 2 {
			t.Fatalf("Expected at least 2 dependency edges from AuthService, got %d: %+v", len(edges), edges)
		}
		hasDB := false
		hasSigner := false
		for _, e := range edges {
			if e.Target == "UserDatabase" {
				hasDB = true
			}
			if e.Target == "TokenSigner" {
				hasSigner = true
			}
		}
		if !hasDB || !hasSigner {
			t.Errorf("Expected dependencies on UserDatabase and TokenSigner, got %+v", edges)
		}
	})

	// 7. Test Impact Analysis (incoming relations)
	t.Run("Graph Impact Analysis", func(t *testing.T) {
		impact, err := searchEngine.Impact(ctx, "UserDatabase")
		if err != nil {
			t.Fatalf("Impact analysis failed: %v", err)
		}
		if len(impact) == 0 {
			t.Fatalf("Expected incoming impact on UserDatabase from AuthService, got 0 edges")
		}
		if impact[0].Source != "AuthService" {
			t.Errorf("Expected impact source 'AuthService', got '%s'", impact[0].Source)
		}
	})

	// 8. Test Backlinks (Wikilinks in body text)
	t.Run("Document Backlinks", func(t *testing.T) {
		backlinks, err := searchEngine.Backlinks(ctx, "TokenSigner")
		if err != nil {
			t.Fatalf("Backlinks failed: %v", err)
		}
		if len(backlinks) == 0 || backlinks[0] != "AuthService" {
			t.Errorf("Expected backlink from 'AuthService', got %+v", backlinks)
		}
	})
}

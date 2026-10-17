// SPDX-License-Identifier: Apache-2.0

package repository

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/md-indexer/internal/entities"
)

func TestSQLiteRepository_UpsertEntityAndRelations(t *testing.T) {
	ctx := context.Background()
	repo, err := NewSQLiteRepository(":memory:")
	if err != nil {
		t.Fatalf("Failed to initialize in-memory SQLite: %v", err)
	}
	defer repo.Close()

	doc := &entities.Document{
		ID:         "doc-1",
		Path:       "/vault/AuthGateway.md",
		Title:      "AuthGateway",
		Hash:       "hash-123",
		ModifiedAt: time.Now(),
		IndexedAt:  time.Now(),
		IsEntity:   true,
		EntityType: "service",
		EntityName: "AuthGateway",
		Tags:       []string{"security", "gateway"},
		EntityProperties: map[string]string{
			"owner": "sec-ops",
			"tier":  "critical",
		},
		EntityRelations: []entities.EntityRelation{
			{SourceID: "AuthGateway", TargetID: "Database", Type: "USES"},
			{SourceID: "AuthGateway", TargetID: "Cache", Type: "USES"},
		},
	}

	// 1. Upsert entity with forward relations (Database and Cache do not exist yet)
	if err := repo.UpsertDocument(ctx, doc); err != nil {
		t.Fatalf("UpsertDocument failed: %v", err)
	}

	db := repo.GetDB()

	// 2. Verify entity row
	var entType, entName string
	err = db.QueryRowContext(ctx, "SELECT type_id, name FROM entities WHERE id = ?", "AuthGateway").Scan(&entType, &entName)
	if err != nil {
		t.Fatalf("Failed to query inserted entity: %v", err)
	}
	if entType != "service" || entName != "AuthGateway" {
		t.Errorf("Expected type 'service' and name 'AuthGateway', got %s, %s", entType, entName)
	}

	// 3. Verify stub target entities exist
	var stubType string
	err = db.QueryRowContext(ctx, "SELECT type_id FROM entities WHERE id = ?", "Database").Scan(&stubType)
	if err != nil {
		t.Fatalf("Expected stub entity for Database: %v", err)
	}
	if stubType != "unresolved" {
		t.Errorf("Expected stub type 'unresolved', got %s", stubType)
	}

	// 4. Verify relations
	rows, err := db.QueryContext(ctx, "SELECT target_entity_id, relation_type FROM entity_relations WHERE source_entity_id = ?", "AuthGateway")
	if err != nil {
		t.Fatalf("Failed to query relations: %v", err)
	}
	defer rows.Close()

	relCount := 0
	for rows.Next() {
		var target, relType string
		if err := rows.Scan(&target, &relType); err != nil {
			t.Fatalf("Failed to scan relation: %v", err)
		}
		relCount++
		if relType != "USES" {
			t.Errorf("Expected relation_type 'USES', got %s", relType)
		}
	}
	if relCount != 2 {
		t.Errorf("Expected 2 relations, got %d", relCount)
	}

	// 5. Verify properties
	var propValue string
	err = db.QueryRowContext(ctx, "SELECT property_value FROM entity_properties WHERE entity_id = ? AND property_key = ?", "AuthGateway", "owner").Scan(&propValue)
	if err != nil || propValue != "sec-ops" {
		t.Errorf("Expected property owner='sec-ops', got %s, err: %v", propValue, err)
	}

	// 6. Test forward entity resolution when Database is indexed later
	dbDoc := &entities.Document{
		ID:         "doc-2",
		Path:       "/vault/Database.md",
		Title:      "Database",
		Hash:       "hash-456",
		ModifiedAt: time.Now(),
		IndexedAt:  time.Now(),
		IsEntity:   true,
		EntityType: "storage",
		EntityName: "Database",
	}
	if err := repo.UpsertDocument(ctx, dbDoc); err != nil {
		t.Fatalf("UpsertDocument for Database failed: %v", err)
	}

	err = db.QueryRowContext(ctx, "SELECT type_id FROM entities WHERE id = ?", "Database").Scan(&stubType)
	if err != nil {
		t.Fatalf("Failed to query resolved Database entity: %v", err)
	}
	if stubType != "storage" {
		t.Errorf("Expected resolved type 'storage', got %s", stubType)
	}
}

func TestSQLiteRepository_CRUD_Search_AndBacklinks(t *testing.T) {
	ctx := context.Background()
	repo, err := NewSQLiteRepository(":memory:")
	if err != nil {
		t.Fatalf("Failed to initialize repo: %v", err)
	}
	defer repo.Close()

	doc1 := &entities.Document{
		ID:         "doc-101",
		Path:       "/vault/DocA.md",
		Title:      "Document Alpha",
		Content:    "Alpha is an encryption component handling cipher algorithms.",
		Hash:       "hash-alpha",
		ModifiedAt: time.Now(),
		IndexedAt:  time.Now(),
		Tags:       []string{"crypto", "security"},
		Links: []entities.Link{
			{SourceDocID: "doc-101", TargetDocID: "doc-102"},
		},
		Sections: []entities.Section{
			{ID: "doc-101-s1", DocumentID: "doc-101", Heading: "Overview", Level: 1, Content: "Alpha overview content."},
		},
	}

	doc2 := &entities.Document{
		ID:         "doc-102",
		Path:       "/vault/DocB.md",
		Title:      "Document Beta",
		Content:    "Beta is a database adapter with connection pooling.",
		Hash:       "hash-beta",
		ModifiedAt: time.Now(),
		IndexedAt:  time.Now(),
		Tags:       []string{"db"},
	}

	if err := repo.UpsertDocument(ctx, doc1); err != nil {
		t.Fatalf("Upsert doc1 failed: %v", err)
	}
	if err := repo.UpsertDocument(ctx, doc2); err != nil {
		t.Fatalf("Upsert doc2 failed: %v", err)
	}

	// 1. GetDocumentByPath
	foundDoc, err := repo.GetDocumentByPath(ctx, "/vault/DocA.md")
	if err != nil || foundDoc == nil {
		t.Fatalf("Expected to find DocA by path, got %v, err: %v", foundDoc, err)
	}
	if foundDoc.ID != "doc-101" {
		t.Errorf("Expected ID 'doc-101', got %s", foundDoc.ID)
	}

	notFound, err := repo.GetDocumentByPath(ctx, "/non/existent.md")
	if err != nil || notFound != nil {
		t.Errorf("Expected nil for non-existent path, got %v, err: %v", notFound, err)
	}

	// 2. GetAllDocuments
	allDocs, err := repo.GetAllDocuments(ctx)
	if err != nil {
		t.Fatalf("GetAllDocuments failed: %v", err)
	}
	if len(allDocs) != 2 {
		t.Errorf("Expected 2 documents, got %d", len(allDocs))
	}

	// 3. FTS Search
	searchResults, err := repo.Search(ctx, "encryption")
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(searchResults) == 0 {
		t.Errorf("Expected search results for 'encryption', got 0")
	} else if searchResults[0].DocumentID != "doc-101" {
		t.Errorf("Expected match for doc-101, got %s", searchResults[0].DocumentID)
	}

	// 4. SearchRepository tests
	searchRepo := NewSearchRepository(repo)

	// FindDocumentByTitle
	byTitle, err := searchRepo.FindDocumentByTitle(ctx, "Document Beta")
	if err != nil || byTitle == nil {
		t.Fatalf("FindDocumentByTitle failed: %v", err)
	}
	if byTitle.ID != "doc-102" {
		t.Errorf("Expected ID doc-102, got %s", byTitle.ID)
	}

	// GetBacklinks (doc1 links to doc2)
	backlinks, err := searchRepo.GetBacklinks(ctx, "doc-102")
	if err != nil {
		t.Fatalf("GetBacklinks failed: %v", err)
	}
	if len(backlinks) != 1 || backlinks[0] != "Document Alpha" {
		t.Errorf("Expected backlink from 'Document Alpha', got %+v", backlinks)
	}

	// FindEntityByName (non-existent) and FindDocumentByTitle (non-existent)
	ghostEnt, err := searchRepo.FindEntityByName(ctx, "Ghost")
	if err != nil || ghostEnt != nil {
		t.Errorf("Expected nil for non-existent entity, got %v, err: %v", ghostEnt, err)
	}
	ghostDoc, err := searchRepo.FindDocumentByTitle(ctx, "Ghost")
	if err != nil || ghostDoc != nil {
		t.Errorf("Expected nil for non-existent document, got %v, err: %v", ghostDoc, err)
	}

	doc1.Metadata = map[string]interface{}{"author": "alice", "version": 2}
	if err := repo.UpsertDocument(ctx, doc1); err != nil {
		t.Fatalf("Re-upsert doc1 with metadata failed: %v", err)
	}
	var metaCount int
	repo.GetDB().QueryRowContext(ctx, "SELECT count(*) FROM metadata WHERE document_id = 'doc-101'").Scan(&metaCount)
	if metaCount != 2 {
		t.Errorf("Expected 2 metadata rows for doc-101, got %d", metaCount)
	}

	// 5. RemoveDocument (must also cascade-delete sections, links, tags, metadata, fts)
	if err := repo.RemoveDocument(ctx, "/vault/DocA.md"); err != nil {
		t.Fatalf("RemoveDocument failed: %v", err)
	}
	deletedDoc, err := repo.GetDocumentByPath(ctx, "/vault/DocA.md")
	if err != nil || deletedDoc != nil {
		t.Errorf("Expected nil after removal, got %v", deletedDoc)
	}
	for _, query := range []string{
		"SELECT count(*) FROM sections WHERE document_id = 'doc-101'",
		"SELECT count(*) FROM links WHERE source_document_id = 'doc-101'",
		"SELECT count(*) FROM document_tags WHERE document_id = 'doc-101'",
		"SELECT count(*) FROM metadata WHERE document_id = 'doc-101'",
		"SELECT count(*) FROM fts_documents WHERE id = 'doc-101'",
		"SELECT count(*) FROM fts_sections WHERE document_id = 'doc-101'",
	} {
		var n int
		repo.GetDB().QueryRowContext(ctx, query).Scan(&n)
		if n != 0 {
			t.Errorf("Expected 0 orphan rows for (%s), got %d", query, n)
		}
	}
}

func TestSQLiteRepository_FTSSections(t *testing.T) {
	ctx := context.Background()
	repo, err := NewSQLiteRepository(filepath.Join(t.TempDir(), "knowledge.db"))
	if err != nil {
		t.Fatalf("Failed to initialize SQLite: %v", err)
	}
	defer repo.Close()

	doc := &entities.Document{
		ID:               "doc-1",
		Path:             "/vault/Crypto.md",
		Title:            "Crypto",
		Tags:             []string{"security"},
		IsEntity:         true,
		EntityType:       "Note",
		EntityName:       "Crypto",
		EntityProperties: map[string]string{"notebook": "infra"},
		Sections: []entities.Section{
			{ID: "doc-1-title", Heading: "Crypto", Level: 1, Content: ""},
			{ID: "doc-1-aes", Heading: "AES", Level: 2, Content: "AES-GCM envelope encryption."},
		},
	}
	if err := repo.UpsertDocument(ctx, doc); err != nil {
		t.Fatalf("UpsertDocument failed: %v", err)
	}

	db := repo.GetDB()
	countFor := func(match string) int {
		var n int
		if err := db.QueryRowContext(ctx, "SELECT count(*) FROM fts_sections WHERE fts_sections MATCH ?", match).Scan(&n); err != nil {
			t.Fatalf("FTS query %q failed: %v", match, err)
		}
		return n
	}

	// Empty sections are not indexed; title, heading, tags, content and metadata are searchable.
	var total int
	db.QueryRowContext(ctx, "SELECT count(*) FROM fts_sections").Scan(&total)
	if total != 1 {
		t.Errorf("Expected 1 indexed section, got %d", total)
	}
	for _, m := range []string{"title:Crypto", "heading:AES", "tags:security", "content:envelope", "metadata:infra"} {
		if countFor(m) != 1 {
			t.Errorf("Expected match for %q", m)
		}
	}

	// Re-upsert replaces instead of duplicating.
	if err := repo.UpsertDocument(ctx, doc); err != nil {
		t.Fatalf("UpsertDocument failed: %v", err)
	}
	db.QueryRowContext(ctx, "SELECT count(*) FROM fts_sections").Scan(&total)
	if total != 1 {
		t.Errorf("Expected 1 indexed section after re-upsert, got %d", total)
	}

	// Backfill: an empty fts_sections table is rebuilt by the migration.
	if _, err := db.ExecContext(ctx, "DELETE FROM fts_sections"); err != nil {
		t.Fatalf("Failed to clear fts_sections: %v", err)
	}
	if err := migrate(db); err != nil {
		t.Fatalf("migrate failed: %v", err)
	}
	if countFor("heading:AES AND tags:security AND metadata:infra") != 1 {
		t.Errorf("Expected backfilled section with tags and metadata")
	}
}

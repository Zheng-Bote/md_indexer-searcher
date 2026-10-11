// SPDX-License-Identifier: Apache-2.0

package repository

import (
	"context"
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

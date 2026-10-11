// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

package retriever

import (
	"context"
	"database/sql"
	"testing"

	"github.com/md-indexer/internal/ai/model"
	_ "modernc.org/sqlite"
)

func setupTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory db: %v", err)
	}

	schema := `
	CREATE TABLE documents (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		path TEXT UNIQUE NOT NULL,
		title TEXT NOT NULL,
		body TEXT NOT NULL
	);

	CREATE VIRTUAL TABLE documents_fts USING fts5(
		title,
		body,
		content='documents',
		content_rowid='id'
	);

	CREATE TABLE sections (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		document_id INTEGER NOT NULL,
		heading TEXT NOT NULL,
		content TEXT NOT NULL
	);

	CREATE TABLE entities (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		type TEXT NOT NULL,
		document_uri TEXT NOT NULL,
		description TEXT
	);

	CREATE TABLE entity_relations (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		source_entity TEXT NOT NULL,
		relation_type TEXT NOT NULL,
		target_entity TEXT NOT NULL,
		document_uri TEXT NOT NULL
	);

	CREATE TABLE links (
		source_path TEXT NOT NULL,
		target_path TEXT NOT NULL
	);
	`
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	// Insert test data
	_, err = db.Exec(`
		INSERT INTO documents (id, path, title, body) VALUES
		(1, 'docs/adr/001-sqlite.md', 'ADR 001: SQLite Choice', 'We chose modernc sqlite because it is CGO-free.'),
		(2, 'docs/components/parser.md', 'Goldmark Parser', 'The parser extracts sections, links, and AST nodes.');

		INSERT INTO documents_fts (rowid, title, body) VALUES
		(1, 'ADR 001: SQLite Choice', 'We chose modernc sqlite because it is CGO-free.'),
		(2, 'Goldmark Parser', 'The parser extracts sections, links, and AST nodes.');

		INSERT INTO sections (document_id, heading, content) VALUES
		(1, 'Context', 'SQLite is lightweight.'),
		(2, 'Architecture', 'Parser connects to SQLite repository.');

		INSERT INTO entities (name, type, document_uri, description) VALUES
		('GoldmarkParser', 'Component', 'docs/components/parser.md', 'Markdown AST Parser'),
		('modernc', 'Library', 'docs/adr/001-sqlite.md', 'CGO-free sqlite engine');

		INSERT INTO entity_relations (source_entity, relation_type, target_entity, document_uri) VALUES
		('GoldmarkParser', 'uses', 'modernc', 'docs/components/parser.md');

		INSERT INTO links (source_path, target_path) VALUES
		('docs/components/parser.md', 'docs/adr/001-sqlite.md');
	`)
	if err != nil {
		t.Fatalf("failed to insert test data: %v", err)
	}

	return db
}

func TestRetriever_Retrieve(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

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
			break
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

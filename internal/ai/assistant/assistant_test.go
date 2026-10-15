// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

package assistant

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/md-indexer/internal/ai/classifier"
	"github.com/md-indexer/internal/ai/config"
	"github.com/md-indexer/internal/ai/contextbuilder"
	"github.com/md-indexer/internal/ai/conversation"
	"github.com/md-indexer/internal/ai/model"
	"github.com/md-indexer/internal/ai/provider"
	"github.com/md-indexer/internal/ai/retriever"
	_ "modernc.org/sqlite"
)

func setupAssistantTestDB(t *testing.T) *sql.DB {
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

	_, err = db.Exec(`
		INSERT INTO documents (id, path, title, body) VALUES
		(1, 'docs/adr/001-sqlite.md', 'ADR 001: SQLite Choice', 'We chose modernc sqlite.');
		INSERT INTO documents_fts (rowid, title, body) VALUES
		(1, 'ADR 001: SQLite Choice', 'We chose modernc sqlite.');
		INSERT INTO sections (document_id, heading, content) VALUES
		(1, 'Decision', 'Modernc sqlite avoids CGO.');
	`)
	if err != nil {
		t.Fatalf("failed to insert data: %v", err)
	}
	return db
}

func TestAssistant_Ask(t *testing.T) {
	db := setupAssistantTestDB(t)
	defer db.Close()

	cfg := config.DefaultConfig()
	clf := classifier.NewRuleClassifier()
	ret := retriever.NewRetriever(db, 3)
	cb := contextbuilder.NewContextBuilder(1000)
	mockLLM := provider.NewMockProvider("We use SQLite without CGO according to [Doc: docs/adr/001-sqlite.md#Decision]. However, there is insufficient information regarding replication.")

	asst := NewAssistant(cfg, clf, ret, cb, mockLLM)
	ctx := context.Background()

	ans, err := asst.Ask(ctx, "What ADR was decided for SQLite?", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if ans.Intent != model.IntentADR {
		t.Errorf("expected ADR intent, got %s", ans.Intent)
	}
	if len(ans.Sources) != 1 {
		t.Fatalf("expected 1 source citation, got %d", len(ans.Sources))
	}
	if ans.Sources[0].Path != "docs/adr/001-sqlite.md" {
		t.Errorf("expected source path 'docs/adr/001-sqlite.md', got %s", ans.Sources[0].Path)
	}
	if len(ans.Gaps) != 1 {
		t.Errorf("expected 1 detected knowledge gap, got %d", len(ans.Gaps))
	}

	// Test multi-turn with conversation session
	sess := conversation.NewSession()
	ans2, err := asst.Ask(ctx, "Can you clarify that?", sess)
	if err != nil {
		t.Fatalf("unexpected error on 2nd turn: %v", err)
	}
	if len(sess.Messages()) != 2 {
		t.Errorf("expected session to contain 2 messages, got %d", len(sess.Messages()))
	}
	if !strings.Contains(ans2.Answer, "SQLite") {
		t.Errorf("expected answer content, got %s", ans2.Answer)
	}
}

func TestContextWindow(t *testing.T) {
	if got := contextWindow("sys", "short prompt", 2048); got != 8192 {
		t.Errorf("expected minimum window 8192, got %d", got)
	}
	// 60000 bytes ~ 20000 tokens + 2048 output -> next 8192 step above 22048.
	if got := contextWindow("", strings.Repeat("a", 60000), 2048); got != 24576 {
		t.Errorf("expected window 24576, got %d", got)
	}
}

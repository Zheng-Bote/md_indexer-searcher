// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

package assistant

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/md-indexer/internal/ai/classifier"
	"github.com/md-indexer/internal/ai/config"
	"github.com/md-indexer/internal/ai/contextbuilder"
	"github.com/md-indexer/internal/ai/conversation"
	"github.com/md-indexer/internal/ai/model"
	"github.com/md-indexer/internal/ai/provider"
	"github.com/md-indexer/internal/ai/retriever"
	"github.com/md-indexer/internal/entities"
	"github.com/md-indexer/internal/repository"
)

func setupAssistantTestDB(t *testing.T) *sql.DB {
	t.Helper()
	// Use the real indexer migration and write path; a file-backed DB keeps pooled connections consistent.
	repo, err := repository.NewSQLiteRepository(filepath.Join(t.TempDir(), "knowledge.db"))
	if err != nil {
		t.Fatalf("failed to init repository: %v", err)
	}
	t.Cleanup(func() { repo.Close() })

	err = repo.UpsertDocument(context.Background(), &entities.Document{
		ID:       "doc-adr",
		Path:     "docs/adr/001-sqlite.md",
		Title:    "ADR 001: SQLite Choice",
		Content:  "Decision\nModernc sqlite avoids CGO.",
		Sections: []entities.Section{{ID: "doc-adr-decision", Heading: "Decision", Level: 2, Content: "Modernc sqlite avoids CGO."}},
	})
	if err != nil {
		t.Fatalf("failed to insert data: %v", err)
	}
	return repo.GetDB()
}

func TestAssistant_Ask(t *testing.T) {
	db := setupAssistantTestDB(t)

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

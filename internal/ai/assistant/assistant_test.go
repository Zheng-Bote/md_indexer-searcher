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
	"github.com/md-indexer/internal/ai/hybrid"
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
	if len(ans.Gaps) != 1 || ans.Gaps[0].Reason == "" {
		t.Errorf("expected 1 detected knowledge gap, got %+v", ans.Gaps)
	}

	// Deterministic sources: even when the LLM omits [Doc: ...] tags, budgeted context documents are returned.
	mockNoCite := provider.NewMockProvider("SQLite is used without CGO.")
	asstNoCite := NewAssistant(cfg, clf, NewRetriever(db, cfg), cb, mockNoCite)
	ansDet, err := asstNoCite.Ask(ctx, "What ADR was decided for SQLite?", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ansDet.Sources) == 0 || ansDet.Sources[0].Path != "docs/adr/001-sqlite.md" {
		t.Errorf("expected deterministic source docs/adr/001-sqlite.md, got %+v", ansDet.Sources)
	}
	if ansDet.Plan == nil || ansDet.Plan.QueryType != "fact" {
		t.Errorf("expected plan with query_type 'fact', got %+v", ansDet.Plan)
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

func TestNewRetriever_SelectsImplementation(t *testing.T) {
	db := setupAssistantTestDB(t)
	cfg := config.DefaultConfig()

	if _, ok := NewRetriever(db, cfg).(*hybrid.Retriever); !ok {
		t.Errorf("expected hybrid retriever by default")
	}
	cfg.Retriever = config.RetrieverClassic
	if _, ok := NewRetriever(db, cfg).(*retriever.Retriever); !ok {
		t.Errorf("expected classic retriever")
	}
}

func TestAssistant_RetrieveOnlyAndAskStream(t *testing.T) {
	db := setupAssistantTestDB(t)
	cfg := config.DefaultConfig()
	clf := classifier.NewRuleClassifier()
	ret := NewRetriever(db, cfg)
	cb := contextbuilder.NewContextBuilder(2000)
	mockLLM := provider.NewMockProvider("Streamed grounded answer [Doc: docs/adr/001-sqlite.md#Decision].")

	asst := NewAssistant(cfg, clf, ret, cb, mockLLM)
	ctx := context.Background()

	// 1. RetrieveOnly must not call the LLM provider.
	dryAns, err := asst.RetrieveOnly(ctx, "What ADR was decided for SQLite?")
	if err != nil {
		t.Fatalf("RetrieveOnly failed: %v", err)
	}
	if len(mockLLM.Calls) != 0 {
		t.Errorf("expected 0 LLM calls during RetrieveOnly, got %d", len(mockLLM.Calls))
	}
	if dryAns.Plan == nil || dryAns.Plan.QueryType != "fact" {
		t.Errorf("expected plan in RetrieveOnly answer, got %+v", dryAns.Plan)
	}
	if len(dryAns.Sources) == 0 || dryAns.Sources[0].Path != "docs/adr/001-sqlite.md" {
		t.Errorf("expected deterministic source in RetrieveOnly answer, got %+v", dryAns.Sources)
	}

	// 2. AskStream must invoke onPlan and onToken callbacks.
	var gotPlan *model.RetrievalPlanInfo
	var gotBlocks int
	var streamed strings.Builder
	streamAns, err := asst.AskStream(
		ctx,
		"What ADR was decided for SQLite?",
		nil,
		func(p *model.RetrievalPlanInfo, blocks int) {
			gotPlan = p
			gotBlocks = blocks
		},
		func(token string) {
			streamed.WriteString(token)
		},
	)
	if err != nil {
		t.Fatalf("AskStream failed: %v", err)
	}
	if gotPlan == nil || gotBlocks == 0 {
		t.Errorf("expected onPlan callback with non-nil plan and blocks > 0, got plan=%+v blocks=%d", gotPlan, gotBlocks)
	}
	if streamed.String() != streamAns.Answer {
		t.Errorf("expected streamed tokens to match Answer, got %q vs %q", streamed.String(), streamAns.Answer)
	}
}

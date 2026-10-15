// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/md-indexer/internal/entities"
	"github.com/md-indexer/internal/repository"
)

type fakeEmbedder struct {
	calls int
	texts []string
}

func (f *fakeEmbedder) Model() string { return "fake-embed" }

func (f *fakeEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	f.calls++
	f.texts = append(f.texts, texts...)
	out := make([][]float32, len(texts))
	for i, t := range texts {
		out[i] = []float32{float32(len(t)), 1}
	}
	return out, nil
}

func TestEmbeddingService_EmbedPending(t *testing.T) {
	ctx := context.Background()
	repo, err := repository.NewSQLiteRepository(filepath.Join(t.TempDir(), "knowledge.db"))
	if err != nil {
		t.Fatalf("failed to init repo: %v", err)
	}
	defer repo.Close()

	doc := &entities.Document{
		ID:    "doc-1",
		Path:  "/vault/a.md",
		Title: "Note A",
		Sections: []entities.Section{
			{ID: "s-title", Heading: "Note A", Level: 1, Content: ""},
			{ID: "s-1", Heading: "One", Level: 2, Content: "first"},
			{ID: "s-2", Heading: "Two", Level: 2, Content: "second"},
		},
	}
	if err := repo.UpsertDocument(ctx, doc); err != nil {
		t.Fatalf("upsert failed: %v", err)
	}

	emb := &fakeEmbedder{}
	svc := NewEmbeddingService(repo, emb)
	count := func() int {
		var n int
		repo.GetDB().QueryRowContext(ctx, "SELECT count(*) FROM section_embeddings WHERE model = 'fake-embed'").Scan(&n)
		return n
	}

	// 1. Initial run embeds the two non-empty sections with title and heading context.
	n, err := svc.EmbedPending(ctx)
	if err != nil || n != 2 || count() != 2 {
		t.Fatalf("expected 2 embedded sections, got n=%d stored=%d err=%v", n, count(), err)
	}
	if emb.texts[0] != "Note A\nOne\nfirst" {
		t.Errorf("unexpected embedding text %q", emb.texts[0])
	}

	// 2. Unchanged content is not re-embedded.
	if n, _ := svc.EmbedPending(ctx); n != 0 {
		t.Errorf("expected 0 re-embedded sections, got %d", n)
	}

	// 3. Only the changed section is re-embedded.
	doc.Sections[2].Content = "second, edited"
	if err := repo.UpsertDocument(ctx, doc); err != nil {
		t.Fatalf("upsert failed: %v", err)
	}
	if n, _ := svc.EmbedPending(ctx); n != 1 {
		t.Errorf("expected 1 re-embedded section, got %d", n)
	}

	// 4. Embeddings of removed documents are deleted.
	if err := repo.RemoveDocument(ctx, doc.Path); err != nil {
		t.Fatalf("remove failed: %v", err)
	}
	if _, err := svc.EmbedPending(ctx); err != nil {
		t.Fatalf("embed pending failed: %v", err)
	}
	if count() != 0 {
		t.Errorf("expected orphan embeddings to be deleted, got %d", count())
	}
}

// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/md-indexer/internal/repository"
)

func newTestIndexer(t *testing.T) (*IndexerService, *repository.SQLiteRepository, string) {
	t.Helper()
	dir := t.TempDir()
	repo, err := repository.NewSQLiteRepository(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { repo.Close() })
	return NewIndexerService(repo, dir), repo, dir
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func indexedPaths(t *testing.T, repo *repository.SQLiteRepository) []string {
	t.Helper()
	paths, err := repo.ListDocumentPaths(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(paths)
	return paths
}

func sectionContents(t *testing.T, repo *repository.SQLiteRepository, path string) []string {
	t.Helper()
	rows, err := repo.GetDB().Query(`SELECT s.content FROM sections s JOIN documents d ON d.id = s.document_id WHERE d.path = ? ORDER BY s.id`, path)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var c string
		rows.Scan(&c)
		out = append(out, c)
	}
	return out
}

func TestDocumentID(t *testing.T) {
	root := t.TempDir()
	a, _ := DocumentID(root, filepath.Join(root, "Daily", "A.md"))
	b, _ := DocumentID(root, filepath.Join(root, "Daily", "B.md"))
	if a == b || len(a) != 64 {
		t.Fatalf("expected distinct 64-hex IDs, got %s / %s", a, b)
	}

	// A relative root (md-indexer -dir .) yields the same ID as the absolute one.
	wd, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(wd) })
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	rel, _ := DocumentID(".", filepath.Join("Daily", "A.md"))
	if rel != a {
		t.Errorf("relative root: %s, absolute root: %s", rel, a)
	}
}

func TestIndexer_DuplicateContentNotesAreIndependent(t *testing.T) {
	svc, repo, dir := newTestIndexer(t)
	ctx := context.Background()
	a, b := filepath.Join(dir, "Daily", "A.md"), filepath.Join(dir, "Daily", "B.md")
	write(t, a, "# Daily\n\nidentical template text\n")
	write(t, b, "# Daily\n\nidentical template text\n")

	if err := svc.IndexAll(ctx); err != nil {
		t.Fatal(err)
	}
	if got := indexedPaths(t, repo); len(got) != 2 {
		t.Fatalf("expected both duplicates indexed, got %v", got)
	}

	write(t, b, "# Daily\n\nedited\n")
	if err := svc.IndexFile(ctx, b); err != nil {
		t.Fatal(err)
	}
	if got := sectionContents(t, repo, a); len(got) != 1 || got[0] != "identical template text" {
		t.Errorf("editing B changed A: %v", got)
	}

	if err := svc.RemoveFile(ctx, b); err != nil {
		t.Fatal(err)
	}
	if got := indexedPaths(t, repo); len(got) != 1 || got[0] != a {
		t.Errorf("removing B affected A: %v", got)
	}
	if got := sectionContents(t, repo, a); len(got) != 1 {
		t.Errorf("A lost its sections: %v", got)
	}
}

func TestIndexer_RenameInBothEventOrders(t *testing.T) {
	for _, createFirst := range []bool{true, false} {
		svc, repo, dir := newTestIndexer(t)
		ctx := context.Background()
		oldPath, newPath := filepath.Join(dir, "Broker.md"), filepath.Join(dir, "MQTT Broker.md")
		write(t, oldPath, "# Broker\n\ncontent\n")
		if err := svc.IndexAll(ctx); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(oldPath, newPath); err != nil {
			t.Fatal(err)
		}

		if createFirst {
			svc.IndexFile(ctx, newPath)
			svc.RemoveFile(ctx, oldPath)
		} else {
			svc.RemoveFile(ctx, oldPath)
			svc.IndexFile(ctx, newPath)
		}
		if got := indexedPaths(t, repo); len(got) != 1 || got[0] != newPath {
			t.Errorf("createFirst=%v: expected only the new path, got %v", createFirst, got)
		}
		if got := sectionContents(t, repo, newPath); len(got) != 1 || got[0] != "content" {
			t.Errorf("createFirst=%v: new document sections %v", createFirst, got)
		}
	}
}

func TestIndexer_MigratesContentHashIDsWithoutReembedding(t *testing.T) {
	svc, repo, dir := newTestIndexer(t)
	ctx := context.Background()
	write(t, filepath.Join(dir, "A.md"), "# A\n\nalpha\n\n## Details\n\nmore alpha\n")
	write(t, filepath.Join(dir, "B.md"), "# B\n\nbeta\n")
	emb := &fakeEmbedder{}
	svc.WithEmbeddings(NewEmbeddingService(repo, emb))
	if err := svc.IndexAll(ctx); err != nil {
		t.Fatal(err)
	}
	countEmbeddings := func() int {
		var n int
		repo.GetDB().QueryRow(`SELECT count(*) FROM section_embeddings`).Scan(&n)
		return n
	}
	before := countEmbeddings()
	if before == 0 {
		t.Fatal("expected seeded embeddings")
	}

	// Rewrite every ID to a legacy content-hash style ID, as written by earlier versions.
	db := repo.GetDB()
	rows, _ := db.Query(`SELECT id, hash FROM documents`)
	type pair struct{ id, legacy string }
	var docs []pair
	for rows.Next() {
		var p pair
		rows.Scan(&p.id, &p.legacy)
		docs = append(docs, p)
	}
	rows.Close()
	for _, d := range docs {
		if _, err := db.Exec(`UPDATE section_embeddings SET section_id = ? || substr(section_id, 65) WHERE substr(section_id, 1, 64) = ?`, d.legacy, d.id); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`UPDATE sections SET id = ? || substr(id, 65), document_id = ? WHERE document_id = ?`, d.legacy, d.legacy, d.id); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`UPDATE documents SET id = ? WHERE id = ?`, d.legacy, d.id); err != nil {
			t.Fatal(err)
		}
	}

	migrated := &fakeEmbedder{}
	svc2 := NewIndexerService(repo, dir).WithEmbeddings(NewEmbeddingService(repo, migrated))
	if err := svc2.IndexAll(ctx); err != nil {
		t.Fatal(err)
	}
	if migrated.calls != 0 {
		t.Errorf("expected no re-embedding, embedder was called %d times for %v", migrated.calls, migrated.texts)
	}
	if after := countEmbeddings(); after != before {
		t.Errorf("embeddings before %d, after %d", before, after)
	}
	for _, d := range docs {
		var n int
		db.QueryRow(`SELECT count(*) FROM documents WHERE id = ?`, d.id).Scan(&n)
		if n != 1 {
			t.Errorf("document %s not migrated back to its path-based ID", d.id)
		}
	}
}

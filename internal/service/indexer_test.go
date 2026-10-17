// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/md-indexer/internal/repository"
)

func TestIndexerService_IndexAllAndIndexFile(t *testing.T) {
	tempDir := t.TempDir()
	fileA := filepath.Join(tempDir, "NoteA.md")
	fileB := filepath.Join(tempDir, "NoteB.md")
	fileNonMD := filepath.Join(tempDir, "image.png")

	if err := os.WriteFile(fileA, []byte("# Note A\nThis is content of note A with [[NoteB]]."), 0644); err != nil {
		t.Fatalf("Failed to write NoteA: %v", err)
	}
	if err := os.WriteFile(fileB, []byte("# Note B\nThis is content of note B."), 0644); err != nil {
		t.Fatalf("Failed to write NoteB: %v", err)
	}
	if err := os.WriteFile(fileNonMD, []byte("binary data"), 0644); err != nil {
		t.Fatalf("Failed to write non-md file: %v", err)
	}
	tmplDir := filepath.Join(tempDir, "_templates_")
	if err := os.MkdirAll(tmplDir, 0755); err != nil {
		t.Fatalf("Failed to create _templates_: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmplDir, "entity.md"), []byte("---\ntype: \"<% tp.system.suggester %>\"\n---\n# Template"), 0644); err != nil {
		t.Fatalf("Failed to write template file: %v", err)
	}

	repo, err := repository.NewSQLiteRepository(":memory:")
	if err != nil {
		t.Fatalf("Failed to initialize repo: %v", err)
	}
	defer repo.Close()

	svc := NewIndexerService(repo, tempDir)
	ctx := context.Background()

	// 1. IndexAll (must skip _templates_/entity.md)
	if err := svc.IndexAll(ctx); err != nil {
		t.Fatalf("IndexAll failed: %v", err)
	}

	allDocs, err := repo.GetAllDocuments(ctx)
	if err != nil {
		t.Fatalf("GetAllDocuments failed: %v", err)
	}
	if len(allDocs) != 2 {
		t.Errorf("Expected 2 indexed markdown documents (skipping _templates_), got %d", len(allDocs))
	}

	// 2. IndexFile again (unchanged file should skip cleanly)
	if err := svc.IndexFile(ctx, fileA); err != nil {
		t.Fatalf("IndexFile failed on unchanged file: %v", err)
	}

	// 3. Modify fileA and index again
	if err := os.WriteFile(fileA, []byte("# Note A Updated\nUpdated content."), 0644); err != nil {
		t.Fatalf("Failed to update NoteA: %v", err)
	}
	if err := svc.IndexFile(ctx, fileA); err != nil {
		t.Fatalf("IndexFile failed on modified file: %v", err)
	}

	docA, err := repo.GetDocumentByPath(ctx, fileA)
	if err != nil || docA == nil {
		t.Fatalf("GetDocumentByPath failed: %v", err)
	}

	// 4. RemoveFile
	if err := svc.RemoveFile(ctx, fileA); err != nil {
		t.Fatalf("RemoveFile failed: %v", err)
	}
	removedDoc, err := repo.GetDocumentByPath(ctx, fileA)
	if err != nil || removedDoc != nil {
		t.Errorf("Expected nil after RemoveFile, got %v", removedDoc)
	}

	// 5. IndexFile on non-existent file returns error
	if err := svc.IndexFile(ctx, "/path/does/not/exist.md"); err == nil {
		t.Errorf("Expected error for non-existent file, got nil")
	}
}

func TestIndexerService_ForceReindexPreservesEmbeddingsAndCleansTemplates(t *testing.T) {
	tempDir := t.TempDir()
	fileA := filepath.Join(tempDir, "NoteA.md")
	tmplDir := filepath.Join(tempDir, "_templates_")
	if err := os.MkdirAll(tmplDir, 0755); err != nil {
		t.Fatalf("Failed to create _templates_: %v", err)
	}
	tmplFile := filepath.Join(tmplDir, "entity.md")

	if err := os.WriteFile(fileA, []byte("---\ntype: Component\nname: CustomA\n---\n# Note A\n\n## Overview\nContent of overview."), 0644); err != nil {
		t.Fatalf("Failed to write NoteA: %v", err)
	}
	if err := os.WriteFile(tmplFile, []byte("# Template"), 0644); err != nil {
		t.Fatalf("Failed to write template: %v", err)
	}

	repo, err := repository.NewSQLiteRepository(":memory:")
	if err != nil {
		t.Fatalf("Failed to initialize repo: %v", err)
	}
	defer repo.Close()

	ctx := context.Background()
	svc := NewIndexerService(repo, tempDir)
	if err := svc.IndexAll(ctx); err != nil {
		t.Fatalf("IndexAll failed: %v", err)
	}

	docA, err := repo.GetDocumentByPath(ctx, fileA)
	if err != nil || docA == nil {
		t.Fatalf("GetDocumentByPath failed: %v", err)
	}

	// Simulate a pre-existing database where:
	// 1) _templates_/entity.md had been indexed before the filter existed
	// 2) entities.document_id was empty
	// 3) section_embeddings already holds a pre-computed embedding for docA#1
	db := repo.GetDB()
	_, err = db.Exec(`INSERT INTO documents (id, path, title, hash) VALUES ('doc-tmpl', ?, 'entity', 'h')`, tmplFile)
	if err != nil {
		t.Fatalf("failed to seed stale template doc: %v", err)
	}
	_, err = db.Exec(`UPDATE entities SET document_id = '' WHERE id = 'CustomA'`)
	if err != nil {
		t.Fatalf("failed to clear document_id: %v", err)
	}
	secID := docA.ID + "#1"
	_, err = db.Exec(`INSERT INTO section_embeddings (section_id, embedding, dimensions, model, content_hash, created_at)
		VALUES (?, X'0000803F', 1, 'bge-m3', 'hash1', CURRENT_TIMESTAMP)`, secID)
	if err != nil {
		t.Fatalf("failed to seed section_embeddings: %v", err)
	}

	// Run with WithForce(true): must re-parse unchanged NoteA (restoring entities.document_id),
	// purge the stale _templates_/entity.md row, and keep the existing section_embeddings row.
	if err := NewIndexerService(repo, tempDir).WithForce(true).IndexAll(ctx); err != nil {
		t.Fatalf("Force IndexAll failed: %v", err)
	}

	tmplDoc, err := repo.GetDocumentByPath(ctx, tmplFile)
	if err != nil || tmplDoc != nil {
		t.Errorf("Expected stale template document to be removed, got %v (err=%v)", tmplDoc, err)
	}

	var gotDocID string
	if err := db.QueryRow(`SELECT document_id FROM entities WHERE id = 'CustomA'`).Scan(&gotDocID); err != nil || gotDocID != docA.ID {
		t.Errorf("Expected force re-index to restore entities.document_id=%s, got %q (err=%v)", docA.ID, gotDocID, err)
	}

	var embCount int
	if err := db.QueryRow(`SELECT count(*) FROM section_embeddings WHERE section_id = ?`, secID).Scan(&embCount); err != nil || embCount != 1 {
		t.Errorf("Expected existing section_embeddings row to be preserved across force re-index, got %d (err=%v)", embCount, err)
	}
}

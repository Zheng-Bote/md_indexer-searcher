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

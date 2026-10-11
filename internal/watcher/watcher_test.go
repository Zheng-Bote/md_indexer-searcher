// SPDX-License-Identifier: Apache-2.0

package watcher

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/md-indexer/internal/repository"
	"github.com/md-indexer/internal/service"
)

func TestWatcher_Lifecycle(t *testing.T) {
	tempDir := t.TempDir()
	subDir := filepath.Join(tempDir, "notes")
	if err := osMkdir(subDir); err != nil {
		t.Fatalf("Failed to create subDir: %v", err)
	}

	repo, err := repository.NewSQLiteRepository(":memory:")
	if err != nil {
		t.Fatalf("Failed to initialize repo: %v", err)
	}
	defer repo.Close()

	svc := service.NewIndexerService(repo, tempDir)
	w, err := NewWatcher(tempDir, svc)
	if err != nil {
		t.Fatalf("NewWatcher failed: %v", err)
	}

	// Start watcher in background goroutine
	go w.Start()

	// Wait briefly then close
	time.Sleep(20 * time.Millisecond)
	if err := w.Close(); err != nil {
		t.Fatalf("Watcher Close failed: %v", err)
	}
}

func osMkdir(path string) error {
	return os.Mkdir(path, 0755)
}

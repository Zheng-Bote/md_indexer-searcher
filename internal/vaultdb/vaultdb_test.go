// SPDX-License-Identifier: Apache-2.0

package vaultdb

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setup(t *testing.T) (vault, dataDir string) {
	t.Helper()
	root := t.TempDir()
	vault = filepath.Join(root, "vault")
	dataDir = filepath.Join(root, "data")
	if err := os.Mkdir(vault, 0o755); err != nil {
		t.Fatal(err)
	}
	resolved, err := Vault(vault)
	if err != nil {
		t.Fatal(err)
	}
	return resolved, dataDir
}

func TestForWriter_UsesVaultLocalDatabaseAndIgnoresItInGit(t *testing.T) {
	vault, dataDir := setup(t)

	path, fallback, err := ForWriter(vault, dataDir)
	if err != nil || fallback {
		t.Fatalf("ForWriter: %q fallback=%v err=%v", path, fallback, err)
	}
	if path != filepath.Join(vault, ".md_indexer", "md_indexer.db") {
		t.Errorf("unexpected path %s", path)
	}
	if got, _ := os.ReadFile(filepath.Join(vault, ".md_indexer", ".gitignore")); string(got) != "*\n" {
		t.Errorf(".gitignore = %q", got)
	}
	entries, _ := os.ReadDir(filepath.Join(vault, ".md_indexer"))
	if len(entries) != 1 {
		t.Errorf("expected only .gitignore after probing, got %v", entries)
	}
	if _, err := os.Stat(dataDir); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("fallback dir must not be created for a writable vault")
	}
}

func TestForWriter_KeepsExistingGitignore(t *testing.T) {
	vault, dataDir := setup(t)
	gitignore := filepath.Join(vault, ".md_indexer", ".gitignore")
	if err := os.MkdirAll(filepath.Dir(gitignore), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(gitignore, []byte("custom\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, _, err := ForWriter(vault, dataDir); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(gitignore); string(got) != "custom\n" {
		t.Errorf("existing .gitignore overwritten: %q", got)
	}
}

func TestForWriter_FallsBackForReadOnlyVault(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	vault, dataDir := setup(t)
	if err := os.Chmod(vault, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(vault, 0o755) })

	path, fallback, err := ForWriter(vault, dataDir)
	if err != nil || !fallback {
		t.Fatalf("ForWriter: %q fallback=%v err=%v", path, fallback, err)
	}
	if path != FallbackPath(vault, dataDir) || !strings.HasPrefix(path, filepath.Join(dataDir, "md-indexer", "vaults")) {
		t.Errorf("unexpected fallback path %s", path)
	}
	if _, err := os.Stat(filepath.Dir(path)); err != nil {
		t.Errorf("fallback directory not created: %v", err)
	}
}

func TestForReader_PrefersLocalThenFallbackAndCreatesNothing(t *testing.T) {
	vault, dataDir := setup(t)

	_, err := ForReader(vault, dataDir)
	if !errors.Is(err, ErrNoIndex) || !strings.Contains(err.Error(), LocalPath(vault)) || !strings.Contains(err.Error(), FallbackPath(vault, dataDir)) {
		t.Fatalf("expected ErrNoIndex naming both paths, got %v", err)
	}
	for _, p := range []string{filepath.Join(vault, DirName), dataDir} {
		if _, err := os.Stat(p); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("reader created %s", p)
		}
	}

	fallback := FallbackPath(vault, dataDir)
	writeFile(t, fallback)
	if got, err := ForReader(vault, dataDir); err != nil || got != fallback {
		t.Errorf("expected fallback %s, got %s (%v)", fallback, got, err)
	}

	local := LocalPath(vault)
	writeFile(t, local)
	if got, err := ForReader(vault, dataDir); err != nil || got != local {
		t.Errorf("expected local %s, got %s (%v)", local, got, err)
	}
}

func TestVault_ResolvesSymlinks(t *testing.T) {
	vault, dataDir := setup(t)
	link := filepath.Join(filepath.Dir(vault), "link")
	if err := os.Symlink(vault, link); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	writeFile(t, LocalPath(vault))

	if got, err := ForReader(link, dataDir); err != nil || got != LocalPath(vault) {
		t.Errorf("symlinked vault resolved to %s (%v)", got, err)
	}
}

func TestMigrateLegacy(t *testing.T) {
	vault, dataDir := setup(t)
	legacy := filepath.Join(dataDir, "legacy", "abc.db")
	for _, suffix := range []string{"", "-wal", "-shm"} {
		writeFile(t, legacy+suffix)
	}
	target := LocalPath(vault)

	moved, err := MigrateLegacy(legacy, target)
	if err != nil || !moved {
		t.Fatalf("MigrateLegacy: moved=%v err=%v", moved, err)
	}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if got, _ := os.ReadFile(target + suffix); string(got) != filepath.Base(legacy+suffix) {
			t.Errorf("target%s content %q", suffix, got)
		}
		if _, err := os.Stat(legacy + suffix); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("legacy%s still exists", suffix)
		}
	}

	writeFile(t, legacy)
	if moved, err := MigrateLegacy(legacy, target); err != nil || moved {
		t.Errorf("must not overwrite an existing target: moved=%v err=%v", moved, err)
	}
	if moved, err := MigrateLegacy(filepath.Join(dataDir, "missing.db"), target+"2"); err != nil || moved {
		t.Errorf("missing legacy must be a no-op: moved=%v err=%v", moved, err)
	}
}

func TestMove_KeepsSourceWhenRenameAndCopyFail(t *testing.T) {
	_, dataDir := setup(t)
	src := filepath.Join(dataDir, "src.db")
	writeFile(t, src)
	// A missing destination directory makes both os.Rename and the copy fallback fail.
	dst := filepath.Join(dataDir, "missing-dir", "dst.db")
	if err := move(src, dst); err == nil {
		t.Fatal("expected error for missing destination directory")
	}
	if _, err := os.Stat(src); err != nil {
		t.Errorf("source must survive a failed move: %v", err)
	}
}

// writeFile creates path (and its parents) with its base name as content.
func writeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(filepath.Base(path)), 0o644); err != nil {
		t.Fatal(err)
	}
}

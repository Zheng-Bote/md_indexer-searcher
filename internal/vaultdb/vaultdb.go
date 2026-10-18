// SPDX-License-Identifier: Apache-2.0

// Package vaultdb resolves where the index database of a vault lives: inside the vault at
// <vault>/.md_indexer/md_indexer.db, or in the user data directory when the vault is not writable.
package vaultdb

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
)

const (
	// DirName is the only folder md-indexer writes inside a vault.
	DirName = ".md_indexer"
	// FileName is the index database inside DirName.
	FileName = "md_indexer.db"
)

// ErrNoIndex is returned by ForReader when neither the vault-local nor the fallback database exists.
var ErrNoIndex = errors.New("no index found for vault")

// sqliteCompanions are the files SQLite keeps next to a WAL-mode database.
var sqliteCompanions = []string{"", "-wal", "-shm"}

// Vault returns the absolute, symlink-resolved path of the vault directory.
func Vault(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("resolve vault %s: %w", dir, err)
	}
	return resolved, nil
}

// LocalPath returns <vault>/.md_indexer/md_indexer.db for an already resolved vault path.
func LocalPath(vault string) string {
	return filepath.Join(vault, DirName, FileName)
}

// FallbackPath returns <dataDir>/md-indexer/vaults/<key>.db, where key is the first 16 hex
// characters of the SHA-256 of the resolved vault path.
func FallbackPath(vault, dataDir string) string {
	sum := sha256.Sum256([]byte(vault))
	return filepath.Join(dataDir, "md-indexer", "vaults", hex.EncodeToString(sum[:8])+".db")
}

// UserDataDir returns the per-user data directory of the current platform.
func UserDataDir() (string, error) {
	switch runtime.GOOS {
	case "windows":
		if dir := os.Getenv("LocalAppData"); dir != "" {
			return dir, nil
		}
		return "", errors.New("%LocalAppData% is not set")
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, "Library", "Application Support"), nil
	default:
		if dir := os.Getenv("XDG_DATA_HOME"); dir != "" {
			return dir, nil
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, ".local", "share"), nil
	}
}

// ForWriter returns the database path md-indexer should use for the vault at dir. It creates
// <vault>/.md_indexer (with a .gitignore) when the vault is writable; otherwise it creates the
// fallback directory and reports fallback=true.
func ForWriter(dir, dataDir string) (path string, fallback bool, err error) {
	vault, err := Vault(dir)
	if err != nil {
		return "", false, err
	}
	if prepareLocalDir(filepath.Join(vault, DirName)) == nil {
		return LocalPath(vault), false, nil
	}
	path = FallbackPath(vault, dataDir)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", true, fmt.Errorf("create fallback directory: %w", err)
	}
	return path, true, nil
}

// prepareLocalDir creates the vault-local folder, keeps it out of git and probes that it is writable.
func prepareLocalDir(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	gitignore := filepath.Join(dir, ".gitignore")
	if _, err := os.Stat(gitignore); errors.Is(err, os.ErrNotExist) {
		if err := os.WriteFile(gitignore, []byte("*\n"), 0o644); err != nil {
			return err
		}
	}
	probe, err := os.CreateTemp(dir, ".probe-*")
	if err != nil {
		return err
	}
	probe.Close()
	return os.Remove(probe.Name())
}

// ForReader returns the existing database for the vault at dir: the vault-local one first, then
// the fallback. It never creates files; without an index it returns an error wrapping ErrNoIndex.
func ForReader(dir, dataDir string) (string, error) {
	vault, err := Vault(dir)
	if err != nil {
		return "", err
	}
	local, fallback := LocalPath(vault), FallbackPath(vault, dataDir)
	for _, candidate := range []string{local, fallback} {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("%w %s (checked %s and %s); run md-indexer -dir %s first", ErrNoIndex, vault, local, fallback, vault)
}

// MigrateLegacy moves a legacy database and its -wal/-shm files to target when the legacy
// database exists and target does not. It falls back to copy-and-remove across file systems.
func MigrateLegacy(legacy, target string) (bool, error) {
	if _, err := os.Stat(legacy); err != nil {
		return false, nil
	}
	if _, err := os.Stat(target); err == nil {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return false, err
	}
	for _, suffix := range sqliteCompanions {
		src := legacy + suffix
		if _, err := os.Stat(src); err != nil {
			continue
		}
		if err := move(src, target+suffix); err != nil {
			return false, fmt.Errorf("move %s: %w", src, err)
		}
	}
	return true, nil
}

func move(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(dst)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(dst)
		return err
	}
	return os.Remove(src)
}

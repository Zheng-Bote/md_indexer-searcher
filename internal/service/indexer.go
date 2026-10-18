// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"

	"github.com/md-indexer/internal/parser"
	"github.com/md-indexer/internal/repository"
	"github.com/rs/zerolog/log"
)

type IndexerService struct {
	repo       *repository.SQLiteRepository
	dir        string
	parser     *parser.Parser
	embeddings *EmbeddingService
	force      bool
}

func NewIndexerService(repo *repository.SQLiteRepository, dir string) *IndexerService {
	return &IndexerService{
		repo:   repo,
		dir:    dir,
		parser: parser.NewParser(),
	}
}

// WithEmbeddings enables keeping section embeddings in sync after indexing.
func (s *IndexerService) WithEmbeddings(e *EmbeddingService) *IndexerService {
	s.embeddings = e
	return s
}

// WithForce forces re-parsing and upserting all Markdown documents even when their SHA-256 hash is unchanged.
func (s *IndexerService) WithForce(force bool) *IndexerService {
	s.force = force
	return s
}

func (s *IndexerService) IndexAll(ctx context.Context) error {
	seenPaths := make(map[string]bool)
	err := filepath.Walk(s.dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if path != s.dir && ShouldSkipPath(path) {
				return filepath.SkipDir
			}
			return nil
		}
		if ShouldSkipPath(path) {
			return nil
		}
		if strings.HasSuffix(strings.ToLower(path), ".md") {
			seenPaths[path] = true
			return s.indexFile(ctx, path)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if err := s.cleanupStaleDocuments(ctx, seenPaths); err != nil {
		return err
	}
	s.embedPending(ctx)
	return nil
}

// cleanupStaleDocuments removes previously indexed documents that now match ShouldSkipPath
// (e.g. _templates_, .obsidian) or no longer exist on disk under the indexed directory.
func (s *IndexerService) cleanupStaleDocuments(ctx context.Context, seenPaths map[string]bool) error {
	dbPaths, err := s.repo.ListDocumentPaths(ctx)
	if err != nil {
		return err
	}
	for _, dbPath := range dbPaths {
		if ShouldSkipPath(dbPath) || !seenPaths[dbPath] {
			log.Info().Str("path", dbPath).Msg("Removing stale or excluded document from index")
			if err := s.repo.RemoveDocument(ctx, dbPath); err != nil {
				return err
			}
		}
	}
	return nil
}

// ShouldSkipPath reports whether a directory or file belongs to a template or hidden folder (e.g. _templates_, .obsidian, .git).
func ShouldSkipPath(path string) bool {
	for _, part := range strings.Split(filepath.ToSlash(path), "/") {
		if part == "" || part == "." || part == ".." {
			continue
		}
		lower := strings.ToLower(part)
		if lower == "_templates_" || lower == "_template_" || (strings.HasPrefix(part, ".") && len(part) > 1) {
			return true
		}
	}
	return false
}

// IndexFile indexes a single file and then embeds its changed sections.
func (s *IndexerService) IndexFile(ctx context.Context, path string) error {
	if ShouldSkipPath(path) {
		return nil
	}
	if err := s.indexFile(ctx, path); err != nil {
		return err
	}
	s.embedPending(ctx)
	return nil
}

// embedPending is best effort: the text index stays usable when Ollama is unavailable.
func (s *IndexerService) embedPending(ctx context.Context) {
	if s.embeddings == nil {
		return
	}
	if _, err := s.embeddings.EmbedPending(ctx); err != nil {
		log.Error().Err(err).Msg("Failed to update section embeddings")
	}
}

// DocumentID returns the hex SHA-256 of path relative to the vault root, with "/" separators,
// so every file gets its own stable ID regardless of its content.
func DocumentID(root, path string) (string, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(absRoot, absPath)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(filepath.ToSlash(rel)))
	return hex.EncodeToString(sum[:]), nil
}

func (s *IndexerService) indexFile(ctx context.Context, path string) error {
	existingDoc, err := s.repo.GetDocumentByPath(ctx, path)
	if err != nil {
		return err
	}

	id, err := DocumentID(s.dir, path)
	if err != nil {
		return err
	}
	doc, err := s.parser.ParseFile(path, id)
	if err != nil {
		log.Error().Err(err).Str("path", path).Msg("Failed to parse file")
		return err
	}

	// Skip if unchanged unless force mode is enabled or the row predates raw Markdown storage or path-based IDs
	if !s.force && existingDoc != nil && existingDoc.ID == doc.ID && existingDoc.Hash == doc.Hash && existingDoc.HasMarkdown {
		log.Debug().Str("path", path).Msg("File unchanged, skipping")
		return nil
	}

	// Rows from before path-based IDs carry a content-hash ID; move their embeddings to the new ID
	// so EmbedPending finds them unchanged instead of re-embedding the sections.
	if existingDoc != nil && existingDoc.ID != doc.ID {
		if _, err := s.repo.RekeyEmbeddings(ctx, existingDoc.ID, doc.ID); err != nil {
			log.Error().Err(err).Str("path", path).Msg("Failed to re-key section embeddings")
			return nil
		}
	}

	log.Info().Str("path", path).Msg("Indexing file")
	// A failed upsert is rolled back and logged; the other files are still indexed.
	if err := s.repo.UpsertDocument(ctx, doc); err != nil {
		log.Error().Err(err).Str("path", path).Msg("Failed to index file")
	}
	return nil
}

func (s *IndexerService) RemoveFile(ctx context.Context, path string) error {
	log.Info().Str("path", path).Msg("Removing file from index")
	return s.repo.RemoveDocument(ctx, path)
}

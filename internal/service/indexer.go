// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
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

func (s *IndexerService) IndexAll(ctx context.Context) error {
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
			return s.indexFile(ctx, path)
		}
		return nil
	})
	if err != nil {
		return err
	}
	s.embedPending(ctx)
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

func (s *IndexerService) indexFile(ctx context.Context, path string) error {
	existingDoc, err := s.repo.GetDocumentByPath(ctx, path)
	if err != nil {
		return err
	}

	doc, err := s.parser.ParseFile(path)
	if err != nil {
		log.Error().Err(err).Str("path", path).Msg("Failed to parse file")
		return err
	}

	// Skip if unchanged
	if existingDoc != nil && existingDoc.Hash == doc.Hash {
		log.Debug().Str("path", path).Msg("File unchanged, skipping")
		return nil
	}

	log.Info().Str("path", path).Msg("Indexing file")
	return s.repo.UpsertDocument(ctx, doc)
}

func (s *IndexerService) RemoveFile(ctx context.Context, path string) error {
	log.Info().Str("path", path).Msg("Removing file from index")
	return s.repo.RemoveDocument(ctx, path)
}

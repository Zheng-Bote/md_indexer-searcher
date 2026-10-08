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
	repo   *repository.SQLiteRepository
	dir    string
	parser *parser.Parser
}

func NewIndexerService(repo *repository.SQLiteRepository, dir string) *IndexerService {
	return &IndexerService{
		repo:   repo,
		dir:    dir,
		parser: parser.NewParser(),
	}
}

func (s *IndexerService) IndexAll(ctx context.Context) error {
	return filepath.Walk(s.dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(strings.ToLower(path), ".md") {
			return s.IndexFile(ctx, path)
		}
		return nil
	})
}

func (s *IndexerService) IndexFile(ctx context.Context, path string) error {
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

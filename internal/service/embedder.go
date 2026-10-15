// SPDX-License-Identifier: Apache-2.0

package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/md-indexer/internal/embedding"
	"github.com/md-indexer/internal/repository"
	"github.com/rs/zerolog/log"
)

const (
	embedBatchSize = 32
	// maxEmbedRunes bounds the text per section; longer sections are truncated to keep CPU embedding fast.
	maxEmbedRunes = 8000
)

// EmbeddingService keeps section_embeddings in sync with the indexed sections.
type EmbeddingService struct {
	repo     *repository.SQLiteRepository
	embedder embedding.Embedder
}

// NewEmbeddingService creates an EmbeddingService.
func NewEmbeddingService(repo *repository.SQLiteRepository, embedder embedding.Embedder) *EmbeddingService {
	return &EmbeddingService{repo: repo, embedder: embedder}
}

// EmbedPending embeds every section that has no embedding for the model or whose content changed,
// and removes embeddings of deleted sections. It returns the number of embedded sections.
func (s *EmbeddingService) EmbedPending(ctx context.Context) (int, error) {
	model := s.embedder.Model()
	if _, err := s.repo.DeleteOrphanEmbeddings(ctx); err != nil {
		return 0, fmt.Errorf("delete orphan embeddings: %w", err)
	}
	sections, err := s.repo.EmbeddableSections(ctx)
	if err != nil {
		return 0, fmt.Errorf("load sections: %w", err)
	}
	existing, err := s.repo.EmbeddingHashes(ctx, model)
	if err != nil {
		return 0, fmt.Errorf("load embedding hashes: %w", err)
	}

	type pendingSection struct {
		id, text, hash string
	}
	var pending []pendingSection
	for _, sec := range sections {
		text := embeddingText(model, sec)
		hash := contentHash(text)
		if existing[sec.ID] != hash {
			pending = append(pending, pendingSection{id: sec.ID, text: text, hash: hash})
		}
	}
	if len(pending) == 0 {
		return 0, nil
	}
	log.Info().Str("model", model).Int("sections", len(pending)).Msg("Embedding sections")

	done := 0
	for start := 0; start < len(pending); start += embedBatchSize {
		batch := pending[start:min(start+embedBatchSize, len(pending))]
		texts := make([]string, len(batch))
		for i, p := range batch {
			texts[i] = p.text
		}
		vectors, err := s.embedder.Embed(ctx, texts)
		if err != nil {
			return done, fmt.Errorf("embed batch: %w", err)
		}
		for i, p := range batch {
			if err := s.repo.SaveEmbedding(ctx, p.id, embedding.Encode(vectors[i]), len(vectors[i]), model, p.hash); err != nil {
				return done, fmt.Errorf("save embedding %s: %w", p.id, err)
			}
		}
		done += len(batch)
		log.Info().Int("done", done).Int("total", len(pending)).Msg("Embedding progress")
	}
	return done, nil
}

// embeddingText combines document title, heading and content so a section is understood in context.
func embeddingText(model string, sec repository.EmbeddableSection) string {
	text := []rune(sec.Title + "\n" + sec.Heading + "\n" + sec.Content)
	if len(text) > maxEmbedRunes {
		text = text[:maxEmbedRunes]
	}
	return embedding.DocumentText(model, string(text))
}

func contentHash(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

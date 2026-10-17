// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

package hybrid

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/md-indexer/internal/embedding"
)

// VectorRetriever returns sections semantically similar to a query.
type VectorRetriever interface {
	Search(ctx context.Context, query string, limit int) ([]Candidate, error)
}

type cachedEmbedding struct {
	sectionID string
	vec       []float32
}

// SQLiteVectors ranks the stored section embeddings of one model by cosine similarity (brute force)
// with an automatically invalidated in-memory vector cache for fast repeated queries in md-api and chat.
type SQLiteVectors struct {
	db       *sql.DB
	embedder embedding.Embedder
	minScore float64

	mu         sync.RWMutex
	cacheModel string
	cacheCount int
	cacheStamp string
	cacheVecs  []cachedEmbedding
}

// NewSQLiteVectors creates a vector retriever; candidates below minScore cosine similarity are dropped,
// so unrelated sections do not fill the context when nothing relevant exists.
func NewSQLiteVectors(db *sql.DB, embedder embedding.Embedder, minScore float64) *SQLiteVectors {
	return &SQLiteVectors{db: db, embedder: embedder, minScore: minScore}
}

type scoredSection struct {
	id    string
	score float64
}

// Search embeds the query and returns the top sections. It returns no candidates, without calling the
// embedder, when the database holds no embeddings for the model (e.g. the indexer ran without embeddings).
func (v *SQLiteVectors) Search(ctx context.Context, query string, limit int) ([]Candidate, error) {
	model := v.embedder.Model()
	var count int
	var stamp string
	err := v.db.QueryRowContext(ctx, "SELECT count(*), COALESCE(max(created_at), '') FROM section_embeddings WHERE model = ?", model).Scan(&count, &stamp)
	if err != nil && strings.Contains(err.Error(), "no such table") {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("vector lookup: %w", err)
	}
	if count == 0 {
		return nil, nil
	}

	vecs, err := v.embedder.Embed(ctx, []string{embedding.QueryText(model, query)})
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}
	q := vecs[0]

	cached, err := v.getEmbeddings(ctx, model, count, stamp)
	if err != nil {
		return nil, err
	}

	var scored []scoredSection
	for _, ce := range cached {
		if len(ce.vec) != len(q) {
			continue
		}
		if s := embedding.Cosine(q, ce.vec); s >= v.minScore {
			scored = append(scored, scoredSection{id: ce.sectionID, score: s})
		}
	}

	sort.Slice(scored, func(i, j int) bool { return scored[i].score > scored[j].score })
	if len(scored) > limit {
		scored = scored[:limit]
	}
	return v.load(ctx, scored)
}

func (v *SQLiteVectors) getEmbeddings(ctx context.Context, model string, count int, stamp string) ([]cachedEmbedding, error) {
	v.mu.RLock()
	if v.cacheModel == model && v.cacheCount == count && v.cacheStamp == stamp && v.cacheVecs != nil {
		out := v.cacheVecs
		v.mu.RUnlock()
		return out, nil
	}
	v.mu.RUnlock()

	// Join sections and documents so embeddings of removed sections never take a top-k slot.
	rows, err := v.db.QueryContext(ctx, `
		SELECT e.section_id, e.embedding
		FROM section_embeddings e
		JOIN sections s ON s.id = e.section_id
		JOIN documents d ON d.id = s.document_id
		WHERE e.model = ?`, model)
	if err != nil {
		return nil, fmt.Errorf("vector scan: %w", err)
	}
	defer rows.Close()

	loaded := make([]cachedEmbedding, 0, count)
	for rows.Next() {
		var id string
		var blob []byte
		if err := rows.Scan(&id, &blob); err != nil {
			return nil, fmt.Errorf("vector scan: %w", err)
		}
		vec, err := embedding.Decode(blob)
		if err != nil {
			return nil, fmt.Errorf("vector decode %s: %w", id, err)
		}
		loaded = append(loaded, cachedEmbedding{sectionID: id, vec: vec})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("vector scan: %w", err)
	}

	v.mu.Lock()
	v.cacheModel = model
	v.cacheCount = count
	v.cacheStamp = stamp
	v.cacheVecs = loaded
	v.mu.Unlock()

	return loaded, nil
}

// load fetches section details for the scored IDs, keeping score order.
func (v *SQLiteVectors) load(ctx context.Context, scored []scoredSection) ([]Candidate, error) {
	if len(scored) == 0 {
		return nil, nil
	}
	args := make([]interface{}, len(scored))
	for i, s := range scored {
		args[i] = s.id
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(scored)), ",")
	rows, err := v.db.QueryContext(ctx, `
		SELECT s.id, s.document_id, d.path, d.title, s.heading, s.content
		FROM sections s
		JOIN documents d ON d.id = s.document_id
		WHERE s.id IN (`+placeholders+`)`, args...)
	if err != nil {
		return nil, fmt.Errorf("vector load: %w", err)
	}
	defer rows.Close()

	byID := make(map[string]Candidate)
	for rows.Next() {
		c := Candidate{SourceType: SourceVector}
		if err := rows.Scan(&c.SectionID, &c.DocumentID, &c.Path, &c.Title, &c.Heading, &c.Content); err != nil {
			return nil, fmt.Errorf("vector load scan: %w", err)
		}
		byID[c.SectionID] = c
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("vector load: %w", err)
	}

	out := make([]Candidate, 0, len(scored))
	for _, s := range scored {
		if c, ok := byID[s.id]; ok {
			c.VectorScore = s.score
			out = append(out, c)
		}
	}
	return out, nil
}

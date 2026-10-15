// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

package hybrid

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/md-indexer/internal/embedding"
)

// VectorRetriever returns sections semantically similar to a query.
type VectorRetriever interface {
	Search(ctx context.Context, query string, limit int) ([]Candidate, error)
}

// SQLiteVectors ranks the stored section embeddings of one model by cosine similarity (brute force).
type SQLiteVectors struct {
	db       *sql.DB
	embedder embedding.Embedder
	minScore float64
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
	var one int
	err := v.db.QueryRowContext(ctx, "SELECT 1 FROM section_embeddings WHERE model = ? LIMIT 1", model).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) || (err != nil && strings.Contains(err.Error(), "no such table")) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("vector lookup: %w", err)
	}

	vecs, err := v.embedder.Embed(ctx, []string{embedding.QueryText(model, query)})
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}
	q := vecs[0]

	// Join sections and documents so embeddings of removed sections never take a top-k slot.
	rows, err := v.db.QueryContext(ctx, `
		SELECT e.section_id, e.embedding
		FROM section_embeddings e
		JOIN sections s ON s.id = e.section_id
		JOIN documents d ON d.id = s.document_id
		WHERE e.model = ? AND e.dimensions = ?`, model, len(q))
	if err != nil {
		return nil, fmt.Errorf("vector scan: %w", err)
	}
	var scored []scoredSection
	for rows.Next() {
		var id string
		var blob []byte
		if err := rows.Scan(&id, &blob); err != nil {
			rows.Close()
			return nil, fmt.Errorf("vector scan: %w", err)
		}
		vec, err := embedding.Decode(blob)
		if err != nil {
			rows.Close()
			return nil, fmt.Errorf("vector decode %s: %w", id, err)
		}
		if s := embedding.Cosine(q, vec); s >= v.minScore {
			scored = append(scored, scoredSection{id: id, score: s})
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("vector scan: %w", err)
	}

	sort.Slice(scored, func(i, j int) bool { return scored[i].score > scored[j].score })
	if len(scored) > limit {
		scored = scored[:limit]
	}
	return v.load(ctx, scored)
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

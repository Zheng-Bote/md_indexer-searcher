// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

package hybrid

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// FTSRetriever returns sections ranked by full-text relevance.
type FTSRetriever interface {
	Search(ctx context.Context, terms []string, limit int) ([]Candidate, error)
}

// SQLiteFTS searches the fts_sections index with field weights.
type SQLiteFTS struct {
	db *sql.DB
}

// NewSQLiteFTS creates a section-level FTS retriever.
func NewSQLiteFTS(db *sql.DB) *SQLiteFTS {
	return &SQLiteFTS{db: db}
}

// bm25 column weights: section_id, document_id (unindexed), title 1.0, heading 0.8, tags 0.7, content 0.5, metadata 0.4.
const sectionQuery = `
	SELECT f.section_id, f.document_id, d.path, d.title, f.heading, f.content,
		-bm25(fts_sections, 0, 0, 1.0, 0.8, 0.7, 0.5, 0.4) AS score
	FROM fts_sections f
	JOIN documents d ON d.id = f.document_id
	WHERE fts_sections MATCH ?
	ORDER BY score DESC
	LIMIT ?;
`

// documentQuery is the fallback for databases built before fts_sections existed.
// bm25 column weights: id (unindexed), title 1.0, content 0.5, tags 0.7.
const documentQuery = `
	WITH top_docs AS (
		SELECT id, -bm25(fts_documents, 0, 1.0, 0.5, 0.7) AS score
		FROM fts_documents
		WHERE fts_documents MATCH ?
		ORDER BY score DESC
		LIMIT ?
	)
	SELECT s.id, d.id, d.path, d.title, s.heading, s.content, t.score
	FROM top_docs t
	JOIN documents d ON d.id = t.id
	JOIN sections s ON s.document_id = d.id AND s.content != ''
	ORDER BY t.score DESC;
`

// Search runs a weighted OR query over the terms.
func (f *SQLiteFTS) Search(ctx context.Context, terms []string, limit int) ([]Candidate, error) {
	if len(terms) == 0 {
		return nil, nil
	}
	match := ftsMatch(terms)
	rows, err := f.db.QueryContext(ctx, sectionQuery, match, limit)
	if err != nil && strings.Contains(err.Error(), "no such table") {
		rows, err = f.db.QueryContext(ctx, documentQuery, match, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("fts search: %w", err)
	}
	defer rows.Close()

	var out []Candidate
	for rows.Next() {
		c := Candidate{SourceType: SourceFTS}
		if err := rows.Scan(&c.SectionID, &c.DocumentID, &c.Path, &c.Title, &c.Heading, &c.Content, &c.FTSScore); err != nil {
			return nil, fmt.Errorf("fts scan: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

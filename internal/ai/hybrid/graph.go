// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

package hybrid

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// GraphRetriever expands seed documents to their graph neighborhood.
type GraphRetriever interface {
	Expand(ctx context.Context, seedDocIDs []string, depth, limit int) ([]Candidate, error)
}

// SQLiteGraph walks wikilinks in both directions with a recursive CTE.
type SQLiteGraph struct {
	db *sql.DB
}

// NewSQLiteGraph creates a link-graph retriever.
func NewSQLiteGraph(db *sql.DB) *SQLiteGraph {
	return &SQLiteGraph{db: db}
}

// graphQuery walks links up to a depth; %[1]s is the seed placeholder list.
// links.target_document_id holds the raw wikilink target and is resolved via the document title.
const graphQuery = `
	WITH RECURSIVE
	resolved(a, b) AS (
		SELECT l.source_document_id, d.id
		FROM links l
		JOIN documents d ON d.title = l.target_document_id
	),
	edges(a, b) AS (
		SELECT a, b FROM resolved
		UNION
		SELECT b, a FROM resolved
	),
	walk(id, depth) AS (
		SELECT b, 1 FROM edges WHERE a IN (%[1]s)
		UNION
		SELECT e.b, w.depth + 1 FROM edges e JOIN walk w ON e.a = w.id WHERE w.depth < ?
	)
	SELECT s.id, d.id, d.path, d.title, s.heading, s.content, MIN(w.depth) AS depth
	FROM walk w
	JOIN documents d ON d.id = w.id
	JOIN sections s ON s.document_id = d.id AND s.content != ''
	WHERE d.id NOT IN (%[1]s)
	GROUP BY s.id
	ORDER BY depth ASC
	LIMIT ?;
`

// Expand returns sections of documents reachable from the seeds; GraphScore is 1/depth.
func (g *SQLiteGraph) Expand(ctx context.Context, seedDocIDs []string, depth, limit int) ([]Candidate, error) {
	if len(seedDocIDs) == 0 || depth < 1 {
		return nil, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(seedDocIDs)), ",")
	query := fmt.Sprintf(graphQuery, placeholders)

	args := make([]interface{}, 0, 2*len(seedDocIDs)+2)
	for _, id := range seedDocIDs {
		args = append(args, id)
	}
	args = append(args, depth)
	for _, id := range seedDocIDs {
		args = append(args, id)
	}
	args = append(args, limit)

	rows, err := g.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("graph expand: %w", err)
	}
	defer rows.Close()

	var out []Candidate
	for rows.Next() {
		var hops int
		c := Candidate{SourceType: SourceGraph}
		if err := rows.Scan(&c.SectionID, &c.DocumentID, &c.Path, &c.Title, &c.Heading, &c.Content, &hops); err != nil {
			return nil, fmt.Errorf("graph scan: %w", err)
		}
		c.GraphScore = 1 / float64(hops)
		out = append(out, c)
	}
	return out, rows.Err()
}

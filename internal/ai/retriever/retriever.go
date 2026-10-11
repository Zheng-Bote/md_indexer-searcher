// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

package retriever

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/md-indexer/internal/ai/model"
)

// Retriever fetches relevant contextual information across multiple strategies from knowledge.db.
type Retriever struct {
	db         *sql.DB
	graphDepth int
}

// NewRetriever creates a new Retriever instance.
func NewRetriever(db *sql.DB, graphDepth int) *Retriever {
	if graphDepth < 1 {
		graphDepth = 3
	}
	return &Retriever{
		db:         db,
		graphDepth: graphDepth,
	}
}

// Retrieve queries the database using FTS5, entities, relations, and graph traversal.
func (r *Retriever) Retrieve(ctx context.Context, query string, intent model.Intent) (*model.RetrievalResult, error) {
	result := &model.RetrievalResult{
		Query:     query,
		Intent:    intent,
		Snippets:  make([]model.DocumentSnippet, 0),
		Entities:  make([]model.EntityMatch, 0),
		Relations: make([]model.EntityRelationMatch, 0),
	}

	// 1. Retrieve Entities and Relations
	if err := r.retrieveEntitiesAndRelations(ctx, query, result); err != nil {
		return nil, fmt.Errorf("failed retrieving entities: %w", err)
	}

	// 2. FTS Search across sections / documents
	if err := r.retrieveFTSSnippets(ctx, query, result); err != nil {
		return nil, fmt.Errorf("failed retrieving fts snippets: %w", err)
	}

	// 3. Graph Neighbors (if any documents were matched)
	if err := r.retrieveGraphNeighbors(ctx, result); err != nil {
		return nil, fmt.Errorf("failed retrieving graph neighbors: %w", err)
	}

	return result, nil
}

func (r *Retriever) retrieveEntitiesAndRelations(ctx context.Context, query string, res *model.RetrievalResult) error {
	words := strings.Fields(query)
	if len(words) == 0 {
		return nil
	}

	// Entity query
	entityQuery := `
		SELECT name, type, document_uri, COALESCE(description, '')
		FROM entities
		WHERE name LIKE ? OR ? LIKE '%' || name || '%'
		LIMIT 10;
	`
	for _, word := range words {
		if len(word) < 3 {
			continue
		}
		pattern := "%" + word + "%"
		rows, err := r.db.QueryContext(ctx, entityQuery, pattern, query)
		if err != nil {
			// If entities table doesn't exist or errors, ignore gracefully
			continue
		}
		for rows.Next() {
			var e model.EntityMatch
			if err := rows.Scan(&e.Name, &e.Type, &e.DocumentURI, &e.Description); err == nil {
				res.Entities = append(res.Entities, e)
			}
		}
		rows.Close()
	}

	// Entity Relations
	if len(res.Entities) > 0 {
		relQuery := `
			SELECT source_entity, relation_type, target_entity, document_uri
			FROM entity_relations
			WHERE source_entity = ? OR target_entity = ?
			LIMIT 15;
		`
		for _, e := range res.Entities {
			rows, err := r.db.QueryContext(ctx, relQuery, e.Name, e.Name)
			if err != nil {
				continue
			}
			for rows.Next() {
				var rel model.EntityRelationMatch
				if err := rows.Scan(&rel.SourceEntity, &rel.RelationType, &rel.TargetEntity, &rel.DocumentURI); err == nil {
					res.Relations = append(res.Relations, rel)
				}
			}
			rows.Close()
		}
	}

	return nil
}

func (r *Retriever) retrieveFTSSnippets(ctx context.Context, query string, res *model.RetrievalResult) error {
	sanitized := sanitizeFTSQuery(query)
	if sanitized == "" {
		return nil
	}

	ftsQuery := `
		SELECT d.path, d.title, COALESCE(s.heading, ''), COALESCE(s.content, d.body), bm25(documents_fts) AS rank
		FROM documents_fts f
		JOIN documents d ON f.rowid = d.id
		LEFT JOIN sections s ON s.document_id = d.id
		WHERE documents_fts MATCH ?
		ORDER BY rank ASC
		LIMIT 10;
	`
	rows, err := r.db.QueryContext(ctx, ftsQuery, sanitized)
	if err != nil {
		// Fallback to simple LIKE if FTS fails or yields syntax error
		return r.fallbackLikeSearch(ctx, sanitized, res)
	}
	defer rows.Close()

	seen := make(map[string]bool)
	for rows.Next() {
		var snip model.DocumentSnippet
		var rank float64
		if err := rows.Scan(&snip.DocumentPath, &snip.Title, &snip.SectionTitle, &snip.Content, &rank); err == nil {
			key := fmt.Sprintf("%s#%s", snip.DocumentPath, snip.SectionTitle)
			if !seen[key] {
				seen[key] = true
				snip.Score = rank
				if strings.Contains(strings.ToLower(snip.DocumentPath), "adr") || strings.Contains(strings.ToLower(snip.Title), "adr") {
					snip.Strategy = "adr"
				} else {
					snip.Strategy = "fts"
				}
				res.Snippets = append(res.Snippets, snip)
			}
		}
	}

	return nil
}

func (r *Retriever) fallbackLikeSearch(ctx context.Context, word string, res *model.RetrievalResult) error {
	likeQuery := `
		SELECT d.path, d.title, COALESCE(s.heading, ''), COALESCE(s.content, d.body)
		FROM documents d
		LEFT JOIN sections s ON s.document_id = d.id
		WHERE d.title LIKE ? OR d.body LIKE ?
		LIMIT 10;
	`
	pattern := "%" + word + "%"
	rows, err := r.db.QueryContext(ctx, likeQuery, pattern, pattern)
	if err != nil {
		return nil
	}
	defer rows.Close()

	for rows.Next() {
		var snip model.DocumentSnippet
		if err := rows.Scan(&snip.DocumentPath, &snip.Title, &snip.SectionTitle, &snip.Content); err == nil {
			snip.Strategy = "fts"
			res.Snippets = append(res.Snippets, snip)
		}
	}
	return nil
}

func (r *Retriever) retrieveGraphNeighbors(ctx context.Context, res *model.RetrievalResult) error {
	if len(res.Snippets) == 0 {
		return nil
	}

	targetPath := res.Snippets[0].DocumentPath
	graphQuery := `
		WITH RECURSIVE graph_cte(source_path, target_path, depth) AS (
			SELECT source_path, target_path, 1
			FROM links
			WHERE source_path = ? OR target_path = ?
			UNION
			SELECT l.source_path, l.target_path, g.depth + 1
			FROM links l
			JOIN graph_cte g ON l.source_path = g.target_path OR l.target_path = g.source_path
			WHERE g.depth < ?
		)
		SELECT DISTINCT d.path, d.title, COALESCE(s.heading, ''), COALESCE(s.content, d.body)
		FROM graph_cte g
		JOIN documents d ON d.path = g.target_path OR d.path = g.source_path
		LEFT JOIN sections s ON s.document_id = d.id
		WHERE d.path != ?
		LIMIT 10;
	`

	rows, err := r.db.QueryContext(ctx, graphQuery, targetPath, targetPath, r.graphDepth, targetPath)
	if err != nil {
		// Graph links may be empty, gracefully ignore
		return nil
	}
	defer rows.Close()

	seen := make(map[string]bool)
	for rows.Next() {
		var snip model.DocumentSnippet
		if err := rows.Scan(&snip.DocumentPath, &snip.Title, &snip.SectionTitle, &snip.Content); err == nil {
			key := fmt.Sprintf("%s#%s", snip.DocumentPath, snip.SectionTitle)
			if !seen[key] {
				seen[key] = true
				snip.Strategy = "graph"
				res.Snippets = append(res.Snippets, snip)
			}
		}
	}

	return nil
}

func sanitizeFTSQuery(q string) string {
	terms := strings.Fields(q)
	var clean []string
	for _, t := range terms {
		t = strings.Map(func(r rune) rune {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
				return r
			}
			return -1
		}, t)
		if len(t) >= 2 {
			clean = append(clean, t)
		}
	}
	return strings.Join(clean, " OR ")
}

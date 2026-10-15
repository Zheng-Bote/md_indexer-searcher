// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

package retriever

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"unicode"

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
		SELECT e.name, COALESCE(t.name, e.type_id, ''),
			COALESCE((SELECT d.path FROM documents d WHERE d.title = e.id LIMIT 1), ''),
			COALESCE((SELECT p.property_value FROM entity_properties p WHERE p.entity_id = e.id AND p.property_key = 'description' LIMIT 1), '')
		FROM entities e
		LEFT JOIN entity_types t ON t.id = e.type_id
		WHERE e.type_id != 'unresolved' AND (e.name LIKE ? OR ? LIKE '%' || e.name || '%')
		LIMIT 10;
	`
	seen := make(map[string]bool)
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
			if err := rows.Scan(&e.Name, &e.Type, &e.DocumentURI, &e.Description); err == nil && !seen[e.Name] {
				seen[e.Name] = true
				res.Entities = append(res.Entities, e)
			}
		}
		rows.Close()
	}

	// Entity Relations
	if len(res.Entities) > 0 {
		relQuery := `
			SELECT r.source_entity_id, r.relation_type, r.target_entity_id,
				COALESCE((SELECT d.path FROM documents d WHERE d.title = r.source_entity_id LIMIT 1), '')
			FROM entity_relations r
			WHERE r.source_entity_id = ? OR r.target_entity_id = ?
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
		WITH top_docs AS (
			SELECT id, content, bm25(fts_documents) AS rank
			FROM fts_documents
			WHERE fts_documents MATCH ?
			ORDER BY rank ASC
			LIMIT 10
		)
		SELECT d.path, d.title, COALESCE(s.heading, ''), COALESCE(s.content, f.content), f.rank
		FROM top_docs f
		JOIN documents d ON d.id = f.id
		LEFT JOIN sections s ON s.document_id = d.id AND s.content != ''
		ORDER BY f.rank ASC;
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
		SELECT d.path, d.title, COALESCE(s.heading, ''), COALESCE(s.content, '')
		FROM documents d
		LEFT JOIN sections s ON s.document_id = d.id AND s.content != ''
		WHERE d.title LIKE ? OR s.content LIKE ?
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
		WITH RECURSIVE
		edges(source_id, target_id) AS (
			-- links.target_document_id holds the raw wikilink target; resolve it via the document title.
			SELECT l.source_document_id, d.id
			FROM links l
			JOIN documents d ON d.title = l.target_document_id
		),
		start(id) AS (
			SELECT id FROM documents WHERE path = ?1
		),
		graph_cte(source_id, target_id, depth) AS (
			SELECT source_id, target_id, 1
			FROM edges
			WHERE source_id = (SELECT id FROM start) OR target_id = (SELECT id FROM start)
			UNION
			SELECT e.source_id, e.target_id, g.depth + 1
			FROM edges e
			JOIN graph_cte g ON e.source_id = g.target_id OR e.target_id = g.source_id
			WHERE g.depth < ?2
		)
		SELECT DISTINCT d.path, d.title, COALESCE(s.heading, ''), COALESCE(s.content, '')
		FROM graph_cte g
		JOIN documents d ON d.id = g.target_id OR d.id = g.source_id
		LEFT JOIN sections s ON s.document_id = d.id AND s.content != ''
		WHERE d.path != ?1
		LIMIT 10;
	`

	rows, err := r.db.QueryContext(ctx, graphQuery, targetPath, r.graphDepth)
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
	// Split on every non letter/digit rune, mirroring the FTS5 unicode61 tokenizer (e.g. "ADR-003" -> "ADR", "003").
	terms := strings.FieldsFunc(q, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	var clean []string
	for _, t := range terms {
		if len([]rune(t)) >= 2 {
			clean = append(clean, t)
		}
	}
	return strings.Join(clean, " OR ")
}

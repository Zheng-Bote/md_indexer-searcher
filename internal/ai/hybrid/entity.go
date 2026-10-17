// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

package hybrid

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/md-indexer/internal/ai/model"
)

// EntityResult holds matched entities, their relations, the sections of their defining documents,
// and any detected knowledge gaps for those entities.
type EntityResult struct {
	Candidates []Candidate
	Entities   []model.EntityMatch
	Relations  []model.EntityRelationMatch
	Gaps       []model.KnowledgeGap
}

// EntityRetriever finds entities named in a query.
type EntityRetriever interface {
	Find(ctx context.Context, query string) (*EntityResult, error)
}

// SQLiteEntities matches entities, entity_properties and entity_relations.
type SQLiteEntities struct {
	db *sql.DB
}

// NewSQLiteEntities creates an entity retriever.
func NewSQLiteEntities(db *sql.DB) *SQLiteEntities {
	return &SQLiteEntities{db: db}
}

const (
	maxEntities          = 10
	maxRelations         = 15
	maxSectionsPerEntity = 3
)

// entityMatchQuery finds entities whose full name occurs in the query (names shorter than 3 characters are too ambiguous).
// Partial name matches are left to FTS, which already weights titles highest.
// Longer names rank first because they are more specific.
const entityMatchQuery = `
	SELECT e.id, e.name, COALESCE(t.name, e.type_id, ''),
		COALESCE((SELECT d.id FROM documents d WHERE d.title = e.id LIMIT 1), ''),
		COALESCE((SELECT d.path FROM documents d WHERE d.title = e.id LIMIT 1), ''),
		COALESCE((SELECT p.property_value FROM entity_properties p WHERE p.entity_id = e.id AND p.property_key = 'description' LIMIT 1), ''),
		COALESCE((SELECT p.property_value FROM entity_properties p WHERE p.entity_id = e.id AND p.property_key = 'api' LIMIT 1), '')
	FROM entities e
	LEFT JOIN entity_types t ON t.id = e.type_id
	WHERE e.type_id != 'unresolved' AND length(e.name) >= 3 AND ? LIKE '%' || e.name || '%'
	ORDER BY length(e.name) DESC, e.name
	LIMIT ?;
`

type entityHit struct {
	id, docID string
	apiProp   string
	match     model.EntityMatch
}

// entityScore is the EntityScore of every section of a named entity's defining document.
const entityScore = 1.0

// Find returns the entities named in the query, their relations and the sections of their defining documents.
func (s *SQLiteEntities) Find(ctx context.Context, query string) (*EntityResult, error) {
	rows, err := s.db.QueryContext(ctx, entityMatchQuery, query, maxEntities)
	if err != nil {
		return nil, fmt.Errorf("entity match: %w", err)
	}
	var hits []entityHit
	for rows.Next() {
		var h entityHit
		if err := rows.Scan(&h.id, &h.match.Name, &h.match.Type, &h.docID, &h.match.DocumentURI, &h.match.Description, &h.apiProp); err != nil {
			rows.Close()
			return nil, fmt.Errorf("entity scan: %w", err)
		}
		hits = append(hits, h)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("entity match: %w", err)
	}

	res := &EntityResult{}
	for i := range hits {
		h := &hits[i]
		res.Entities = append(res.Entities, h.match)
		var cands []Candidate
		if h.docID != "" {
			var err error
			cands, err = s.sections(ctx, h)
			if err != nil {
				return nil, err
			}
			res.Candidates = append(res.Candidates, cands...)
		}
		var rels []model.EntityRelationMatch
		if len(res.Relations) < maxRelations {
			var err error
			rels, err = s.relations(ctx, h.id, maxRelations-len(res.Relations))
			if err != nil {
				return nil, err
			}
			res.Relations = append(res.Relations, rels...)
		}
		gaps, err := s.detectEntityGaps(ctx, h, cands, rels)
		if err != nil {
			return nil, err
		}
		res.Gaps = append(res.Gaps, gaps...)
	}
	return res, nil
}

func (s *SQLiteEntities) detectEntityGaps(ctx context.Context, h *entityHit, cands []Candidate, rels []model.EntityRelationMatch) ([]model.KnowledgeGap, error) {
	var gaps []model.KnowledgeGap
	if strings.TrimSpace(h.match.Description) == "" && len(cands) == 0 {
		gaps = append(gaps, model.KnowledgeGap{
			Entity: h.match.Name,
			Reason: "no description or content sections found",
		})
	}

	typ := strings.ToLower(strings.TrimSpace(h.match.Type))
	isArchEntity := typ == "service" || typ == "component" || typ == "pipeline" ||
		strings.HasSuffix(typ, "-service") || strings.HasSuffix(typ, "_service")
	if !isArchEntity {
		return gaps, nil
	}

	hasADR := false
	for _, r := range rels {
		if strings.EqualFold(r.RelationType, "ADR") || adrPattern.MatchString(r.TargetEntity) || adrPattern.MatchString(r.SourceEntity) {
			hasADR = true
			break
		}
	}
	if !hasADR && h.docID != "" {
		var count int
		err := s.db.QueryRowContext(ctx, `
			SELECT count(*) FROM links l
			WHERE (l.source_document_id = ? AND lower(l.target_document_id) LIKE '%adr%')
			   OR (l.target_document_id = ? AND EXISTS (SELECT 1 FROM documents d WHERE d.id = l.source_document_id AND (lower(d.title) LIKE '%adr%' OR lower(d.path) LIKE '%adr%')))`,
			h.docID, h.id).Scan(&count)
		if err == nil && count > 0 {
			hasADR = true
		}
	}
	if !hasADR {
		gaps = append(gaps, model.KnowledgeGap{
			Entity: h.match.Name,
			Reason: "no ADR linked or documented",
		})
	}

	if typ == "service" || typ == "component" || strings.HasSuffix(typ, "-service") || strings.HasSuffix(typ, "_service") {
		hasAPI := strings.TrimSpace(h.apiProp) != ""
		if !hasAPI && h.docID != "" {
			var apiCount int
			err := s.db.QueryRowContext(ctx, `
				SELECT count(*) FROM sections
				WHERE document_id = ? AND content != '' AND (lower(heading) LIKE '%api%' OR lower(heading) LIKE '%schnittstelle%' OR lower(heading) LIKE '%endpoint%' OR lower(content) LIKE '%endpoint%')`,
				h.docID).Scan(&apiCount)
			if err == nil && apiCount > 0 {
				hasAPI = true
			}
		}
		if !hasAPI {
			gaps = append(gaps, model.KnowledgeGap{
				Entity: h.match.Name,
				Reason: "no API specification or section found",
			})
		}
	}
	return gaps, nil
}

func (s *SQLiteEntities) sections(ctx context.Context, h *entityHit) ([]Candidate, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT s.id, s.heading, s.content, d.title
		FROM sections s
		JOIN documents d ON d.id = s.document_id
		WHERE s.document_id = ? AND s.content != ''
		LIMIT ?;`, h.docID, maxSectionsPerEntity)
	if err != nil {
		return nil, fmt.Errorf("entity sections: %w", err)
	}
	defer rows.Close()

	var out []Candidate
	for rows.Next() {
		c := Candidate{DocumentID: h.docID, Path: h.match.DocumentURI, EntityScore: entityScore, SourceType: SourceEntity}
		if err := rows.Scan(&c.SectionID, &c.Heading, &c.Content, &c.Title); err != nil {
			return nil, fmt.Errorf("entity sections scan: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *SQLiteEntities) relations(ctx context.Context, entityID string, limit int) ([]model.EntityRelationMatch, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT r.source_entity_id, r.relation_type, r.target_entity_id,
			COALESCE((SELECT d.path FROM documents d WHERE d.title = r.source_entity_id LIMIT 1), '')
		FROM entity_relations r
		WHERE r.source_entity_id = ? OR r.target_entity_id = ?
		LIMIT ?;`, entityID, entityID, limit)
	if err != nil {
		return nil, fmt.Errorf("entity relations: %w", err)
	}
	defer rows.Close()

	var out []model.EntityRelationMatch
	for rows.Next() {
		var r model.EntityRelationMatch
		if err := rows.Scan(&r.SourceEntity, &r.RelationType, &r.TargetEntity, &r.DocumentURI); err != nil {
			return nil, fmt.Errorf("entity relations scan: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// SPDX-License-Identifier: Apache-2.0

package repository

import (
	"context"
	"database/sql"
	"path/filepath"
	"sort"

	"github.com/md-indexer/internal/entities"
)

type SearchRepository struct {
	db *sql.DB
}

func NewSearchRepository(repo *SQLiteRepository) *SearchRepository {
	return &SearchRepository{db: repo.GetDB()}
}

func (r *SearchRepository) FindEntityByName(ctx context.Context, name string) (*entities.Entity, error) {
	var ent entities.Entity
	var typeID string
	err := r.db.QueryRowContext(ctx, "SELECT id, type_id, name FROM entities WHERE name = ?", name).Scan(&ent.ID, &typeID, &ent.Name)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	err = r.db.QueryRowContext(ctx, "SELECT name FROM entity_types WHERE id = ?", typeID).Scan(&ent.Type)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}

	rows, err := r.db.QueryContext(ctx, "SELECT property_key, property_value FROM entity_properties WHERE entity_id = ?", ent.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ent.Properties = make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err == nil {
			ent.Properties[k] = v
		}
	}
	return &ent, nil
}

func (r *SearchRepository) FindDocumentByTitle(ctx context.Context, title string) (*entities.Document, error) {
	row := r.db.QueryRowContext(ctx, "SELECT id, path, title FROM documents WHERE title = ?", title)
	var doc entities.Document
	err := row.Scan(&doc.ID, &doc.Path, &doc.Title)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &doc, nil
}

func (r *SearchRepository) GetBacklinks(ctx context.Context, targetDocID string) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT d.title 
		FROM links l 
		JOIN documents d ON l.source_document_id = d.id 
		WHERE l.target_document_id = ? OR l.target_document_id = (SELECT title FROM documents WHERE id = ?)`, targetDocID, targetDocID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var links []string
	for rows.Next() {
		var title string
		if err := rows.Scan(&title); err == nil {
			links = append(links, title)
		}
	}
	return links, nil
}

// DocumentByPath returns the indexed document stored at exactly path, or nil when it is not indexed.
func (r *SearchRepository) DocumentByPath(ctx context.Context, path string) (*entities.DocumentView, error) {
	var doc entities.DocumentView
	var markdown sql.NullString
	err := r.db.QueryRowContext(ctx, "SELECT id, path, title, markdown FROM documents WHERE path = ?", path).
		Scan(&doc.ID, &doc.Path, &doc.Title, &markdown)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	doc.Markdown = markdown.String
	if doc.Links, err = r.resolveLinks(ctx, doc.ID, doc.Path); err != nil {
		return nil, err
	}
	return &doc, nil
}

// DocumentByTitle resolves a wikilink title case-insensitively as seen from the document at from.
func (r *SearchRepository) DocumentByTitle(ctx context.Context, title, from string) (*entities.DocumentView, error) {
	path, err := r.resolveTitle(ctx, title, from)
	if err != nil || path == "" {
		return nil, err
	}
	return r.DocumentByPath(ctx, path)
}

// resolveTitle picks the document path for title: same folder as from, then shortest path, then alphabetical.
func (r *SearchRepository) resolveTitle(ctx context.Context, title, from string) (string, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT path FROM documents WHERE title = ? COLLATE NOCASE", title)
	if err != nil {
		return "", err
	}
	defer rows.Close()

	var paths []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return "", err
		}
		paths = append(paths, p)
	}
	if err := rows.Err(); err != nil || len(paths) == 0 {
		return "", err
	}

	fromDir := filepath.Dir(from)
	sort.Slice(paths, func(i, j int) bool {
		si, sj := filepath.Dir(paths[i]) == fromDir, filepath.Dir(paths[j]) == fromDir
		if si != sj {
			return si
		}
		if len(paths[i]) != len(paths[j]) {
			return len(paths[i]) < len(paths[j])
		}
		return paths[i] < paths[j]
	})
	return paths[0], nil
}

// resolveLinks returns the outgoing wikilink targets of a document that resolve to an indexed document.
func (r *SearchRepository) resolveLinks(ctx context.Context, docID, from string) ([]entities.ResolvedLink, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT DISTINCT target_document_id FROM links WHERE source_document_id = ? ORDER BY target_document_id", docID)
	if err != nil {
		return nil, err
	}
	var targets []string
	for rows.Next() {
		var target string
		if err := rows.Scan(&target); err != nil {
			rows.Close()
			return nil, err
		}
		targets = append(targets, target)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	links := []entities.ResolvedLink{}
	for _, target := range targets {
		path, err := r.resolveTitle(ctx, target, from)
		if err != nil {
			return nil, err
		}
		if path != "" {
			links = append(links, entities.ResolvedLink{Target: target, Path: path})
		}
	}
	return links, nil
}

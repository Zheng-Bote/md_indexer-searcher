package repository

import (
	"context"
	"database/sql"
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
	if err != nil { return nil, err }
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
		if err == sql.ErrNoRows { return nil, nil }
		return nil, err
	}
	return &doc, nil
}

func (r *SearchRepository) GetBacklinks(ctx context.Context, targetDocID string) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT d.title 
		FROM links l 
		JOIN documents d ON l.source_document_id = d.id 
		WHERE l.target_document_id = ?`, targetDocID)
	if err != nil { return nil, err }
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

package repository

import (
	"context"
	"database/sql"
	"strings"

	"github.com/md-indexer/internal/entities"
	_ "modernc.org/sqlite"
)

type SQLiteRepository struct {
	db *sql.DB
}

func NewSQLiteRepository(dsn string) (*SQLiteRepository, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if err := migrate(db); err != nil {
		return nil, err
	}
	return &SQLiteRepository{db: db}, nil
}

func (r *SQLiteRepository) Close() error {
	return r.db.Close()
}

func (r *SQLiteRepository) GetDB() *sql.DB {
	return r.db
}

func migrate(db *sql.DB) error {
	query := `
	CREATE TABLE IF NOT EXISTS documents (id TEXT PRIMARY KEY, path TEXT UNIQUE, title TEXT, hash TEXT, modified_at DATETIME, indexed_at DATETIME);
	CREATE TABLE IF NOT EXISTS sections (id TEXT PRIMARY KEY, document_id TEXT, heading TEXT, level INTEGER, content TEXT, FOREIGN KEY(document_id) REFERENCES documents(id));
	CREATE TABLE IF NOT EXISTS links (source_document_id TEXT, target_document_id TEXT, FOREIGN KEY(source_document_id) REFERENCES documents(id));
	CREATE TABLE IF NOT EXISTS tags (id TEXT PRIMARY KEY, name TEXT UNIQUE);
	CREATE TABLE IF NOT EXISTS document_tags (document_id TEXT, tag_id TEXT, PRIMARY KEY(document_id, tag_id));
	CREATE TABLE IF NOT EXISTS metadata (document_id TEXT, key TEXT, value TEXT, FOREIGN KEY(document_id) REFERENCES documents(id));
	
	CREATE TABLE IF NOT EXISTS entity_types (id TEXT PRIMARY KEY, name TEXT UNIQUE);
	CREATE TABLE IF NOT EXISTS entities (id TEXT PRIMARY KEY, type_id TEXT, name TEXT, FOREIGN KEY(type_id) REFERENCES entity_types(id));
	CREATE TABLE IF NOT EXISTS entity_properties (entity_id TEXT, property_key TEXT, property_value TEXT, FOREIGN KEY(entity_id) REFERENCES entities(id));
	CREATE TABLE IF NOT EXISTS entity_relations (source_entity_id TEXT, target_entity_id TEXT, relation_type TEXT, FOREIGN KEY(source_entity_id) REFERENCES entities(id), FOREIGN KEY(target_entity_id) REFERENCES entities(id));

	CREATE VIRTUAL TABLE IF NOT EXISTS fts_documents USING fts5(id UNINDEXED, title, content, tags);`
	
	_, err := db.Exec(query)
	return err
}

func (r *SQLiteRepository) UpsertDocument(ctx context.Context, doc *entities.Document) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil { return err }
	defer tx.Rollback()

	// 1. Wipe Document data
	tx.ExecContext(ctx, "DELETE FROM documents WHERE path = ?", doc.Path)
	tx.ExecContext(ctx, "DELETE FROM sections WHERE document_id = ?", doc.ID)
	tx.ExecContext(ctx, "DELETE FROM links WHERE source_document_id = ?", doc.ID)
	tx.ExecContext(ctx, "DELETE FROM fts_documents WHERE id = ?", doc.ID)

	// 2. Wipe Entity data (linked by name/ID for this document)
	// We use doc.EntityName as the Entity ID for simplicity in linking.
	entID := doc.EntityName
	if entID == "" { entID = doc.Title }
	
	tx.ExecContext(ctx, "DELETE FROM entities WHERE id = ?", entID)
	tx.ExecContext(ctx, "DELETE FROM entity_properties WHERE entity_id = ?", entID)
	tx.ExecContext(ctx, "DELETE FROM entity_relations WHERE source_entity_id = ?", entID)

	// 3. Insert Document data
	tx.ExecContext(ctx, "INSERT INTO documents (id, path, title, hash, modified_at, indexed_at) VALUES (?, ?, ?, ?, ?, ?)", doc.ID, doc.Path, doc.Title, doc.Hash, doc.ModifiedAt, doc.IndexedAt)
	tx.ExecContext(ctx, "INSERT INTO fts_documents (id, title, content, tags) VALUES (?, ?, ?, ?)", doc.ID, doc.Title, doc.Content, strings.Join(doc.Tags, " "))

	for _, link := range doc.Links {
		tx.ExecContext(ctx, "INSERT INTO links (source_document_id, target_document_id) VALUES (?, ?)", link.SourceDocID, link.TargetDocID)
	}

	for _, sec := range doc.Sections {
		tx.ExecContext(ctx, "INSERT INTO sections (id, document_id, heading, level, content) VALUES (?, ?, ?, ?, ?)", sec.ID, doc.ID, sec.Heading, sec.Level, sec.Content)
	}

	// 4. Insert Entity data if IsEntity
	if doc.IsEntity {
		tx.ExecContext(ctx, "INSERT OR IGNORE INTO entity_types (id, name) VALUES (?, ?)", doc.EntityType, doc.EntityType)
		tx.ExecContext(ctx, "INSERT INTO entities (id, type_id, name) VALUES (?, ?, ?)", entID, doc.EntityType, doc.EntityName)
		
		for k, v := range doc.EntityProperties {
			tx.ExecContext(ctx, "INSERT INTO entity_properties (entity_id, property_key, property_value) VALUES (?, ?, ?)", entID, k, v)
		}
		
		for _, rel := range doc.EntityRelations {
			tx.ExecContext(ctx, "INSERT INTO entity_relations (source_entity_id, target_entity_id, relation_type) VALUES (?, ?, ?)", rel.SourceID, rel.TargetID, rel.Type)
		}
	}

	return tx.Commit()
}

func (r *SQLiteRepository) RemoveDocument(ctx context.Context, path string) error {
	_, err := r.db.ExecContext(ctx, "DELETE FROM documents WHERE path = ?", path)
	return err
}

func (r *SQLiteRepository) GetDocumentByPath(ctx context.Context, path string) (*entities.Document, error) {
	row := r.db.QueryRowContext(ctx, "SELECT id, path, hash FROM documents WHERE path = ?", path)
	doc := &entities.Document{}
	err := row.Scan(&doc.ID, &doc.Path, &doc.Hash)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return doc, nil
}

func (r *SQLiteRepository) Search(ctx context.Context, query string) ([]entities.SearchResult, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, title, snippet(fts_documents, 2, '<b>', '</b>', '...', 64) as snippet, rank 
		FROM fts_documents 
		WHERE fts_documents MATCH ? 
		ORDER BY rank LIMIT 20`, query)
	if err != nil { return nil, err }
	defer rows.Close()

	var results []entities.SearchResult
	for rows.Next() {
		var res entities.SearchResult
		if err := rows.Scan(&res.DocumentID, &res.Title, &res.Snippet, &res.Rank); err != nil { return nil, err }
		results = append(results, res)
	}
	return results, nil
}

func (r *SQLiteRepository) GetAllDocuments(ctx context.Context) ([]entities.Document, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT id, path, title FROM documents")
	if err != nil { return nil, err }
	defer rows.Close()

	var results []entities.Document
	for rows.Next() {
		var doc entities.Document
		if err := rows.Scan(&doc.ID, &doc.Path, &doc.Title); err != nil { return nil, err }
		results = append(results, doc)
	}
	return results, nil
}

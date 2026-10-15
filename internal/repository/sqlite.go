// SPDX-License-Identifier: Apache-2.0

package repository

import (
	"context"
	"database/sql"
	"sort"
	"strings"
	"time"

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

	CREATE VIRTUAL TABLE IF NOT EXISTS fts_documents USING fts5(id UNINDEXED, title, content, tags);
	-- Section embeddings for vector retrieval; content_hash detects changed sections.
	CREATE TABLE IF NOT EXISTS section_embeddings (section_id TEXT PRIMARY KEY, embedding BLOB NOT NULL, dimensions INTEGER NOT NULL, model TEXT NOT NULL, content_hash TEXT NOT NULL, created_at DATETIME NOT NULL);
	-- Wikilink targets and entities resolve to documents by title.
	CREATE INDEX IF NOT EXISTS idx_documents_title ON documents(title);
	CREATE VIRTUAL TABLE IF NOT EXISTS fts_sections USING fts5(section_id UNINDEXED, document_id UNINDEXED, title, heading, tags, content, metadata);

	-- Backfill section FTS for databases indexed before fts_sections existed (unchanged files are never re-indexed).
	INSERT INTO fts_sections (section_id, document_id, title, heading, tags, content, metadata)
	SELECT s.id, s.document_id, d.title, s.heading,
		COALESCE((SELECT group_concat(dt.tag_id, ' ') FROM document_tags dt WHERE dt.document_id = d.id), ''),
		s.content,
		COALESCE((SELECT group_concat(p.property_value, ' ') FROM entity_properties p WHERE p.entity_id = d.title), '')
	FROM sections s
	JOIN documents d ON d.id = s.document_id
	WHERE s.content != '' AND NOT EXISTS (SELECT 1 FROM fts_sections);`
	
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
	tx.ExecContext(ctx, "DELETE FROM fts_sections WHERE document_id = ?", doc.ID)

	// 2. Wipe Entity properties and outgoing relations for this document
	entID := doc.EntityName
	if entID == "" { entID = doc.Title }
	
	tx.ExecContext(ctx, "DELETE FROM entity_properties WHERE entity_id = ?", entID)
	tx.ExecContext(ctx, "DELETE FROM entity_relations WHERE source_entity_id = ?", entID)

	// 3. Insert Document data
	tx.ExecContext(ctx, "INSERT INTO documents (id, path, title, hash, modified_at, indexed_at) VALUES (?, ?, ?, ?, ?, ?)", doc.ID, doc.Path, doc.Title, doc.Hash, doc.ModifiedAt, doc.IndexedAt)
	tx.ExecContext(ctx, "INSERT INTO fts_documents (id, title, content, tags) VALUES (?, ?, ?, ?)", doc.ID, doc.Title, doc.Content, strings.Join(doc.Tags, " "))

	// Persist document tags
	tx.ExecContext(ctx, "DELETE FROM document_tags WHERE document_id = ?", doc.ID)
	for _, tag := range doc.Tags {
		tx.ExecContext(ctx, "INSERT OR IGNORE INTO tags (id, name) VALUES (?, ?)", tag, tag)
		tx.ExecContext(ctx, "INSERT OR IGNORE INTO document_tags (document_id, tag_id) VALUES (?, ?)", doc.ID, tag)
	}

	for _, link := range doc.Links {
		tx.ExecContext(ctx, "INSERT INTO links (source_document_id, target_document_id) VALUES (?, ?)", link.SourceDocID, link.TargetDocID)
	}

	tags := strings.Join(doc.Tags, " ")
	metadata := propertyValues(doc.EntityProperties)
	for _, sec := range doc.Sections {
		tx.ExecContext(ctx, "INSERT INTO sections (id, document_id, heading, level, content) VALUES (?, ?, ?, ?, ?)", sec.ID, doc.ID, sec.Heading, sec.Level, sec.Content)
		if sec.Content != "" {
			tx.ExecContext(ctx, "INSERT INTO fts_sections (section_id, document_id, title, heading, tags, content, metadata) VALUES (?, ?, ?, ?, ?, ?, ?)", sec.ID, doc.ID, doc.Title, sec.Heading, tags, sec.Content, metadata)
		}
	}

	// 4. Insert Entity data if IsEntity
	if doc.IsEntity {
		tx.ExecContext(ctx, "INSERT OR IGNORE INTO entity_types (id, name) VALUES ('unresolved', 'unresolved')")
		tx.ExecContext(ctx, "INSERT OR IGNORE INTO entity_types (id, name) VALUES (?, ?)", doc.EntityType, doc.EntityType)
		tx.ExecContext(ctx, "INSERT INTO entities (id, type_id, name) VALUES (?, ?, ?) ON CONFLICT(id) DO UPDATE SET type_id = excluded.type_id, name = excluded.name", entID, doc.EntityType, doc.EntityName)
		
		for k, v := range doc.EntityProperties {
			tx.ExecContext(ctx, "INSERT INTO entity_properties (entity_id, property_key, property_value) VALUES (?, ?, ?)", entID, k, v)
		}
		
		seenRel := make(map[string]bool)
		for _, rel := range doc.EntityRelations {
			if rel.SourceID == "" || rel.TargetID == "" {
				continue
			}
			relKey := rel.SourceID + "->" + rel.TargetID + ":" + rel.Type
			if seenRel[relKey] {
				continue
			}
			seenRel[relKey] = true

			// Ensure target entity stub exists to satisfy relational integrity
			tx.ExecContext(ctx, "INSERT OR IGNORE INTO entities (id, type_id, name) VALUES (?, 'unresolved', ?)", rel.TargetID, rel.TargetID)

			tx.ExecContext(ctx, "INSERT INTO entity_relations (source_entity_id, target_entity_id, relation_type) VALUES (?, ?, ?)", rel.SourceID, rel.TargetID, rel.Type)
		}
	} else {
		tx.ExecContext(ctx, "DELETE FROM entities WHERE id = ?", entID)
	}

	return tx.Commit()
}

// propertyValues joins frontmatter property values in key order for full-text indexing.
func propertyValues(props map[string]string) string {
	keys := make([]string, 0, len(props))
	for k := range props {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	values := make([]string, 0, len(keys))
	for _, k := range keys {
		values = append(values, props[k])
	}
	return strings.Join(values, " ")
}

// EmbeddableSection is a non-empty section of an indexed document.
type EmbeddableSection struct {
	ID      string
	Title   string
	Heading string
	Content string
}

// EmbeddableSections returns all non-empty sections of indexed documents.
func (r *SQLiteRepository) EmbeddableSections(ctx context.Context) ([]EmbeddableSection, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT s.id, d.title, s.heading, s.content
		FROM sections s
		JOIN documents d ON d.id = s.document_id
		WHERE s.content != ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []EmbeddableSection
	for rows.Next() {
		var sec EmbeddableSection
		if err := rows.Scan(&sec.ID, &sec.Title, &sec.Heading, &sec.Content); err != nil {
			return nil, err
		}
		out = append(out, sec)
	}
	return out, rows.Err()
}

// EmbeddingHashes maps section IDs to the content hash of their stored embedding for a model.
func (r *SQLiteRepository) EmbeddingHashes(ctx context.Context, model string) (map[string]string, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT section_id, content_hash FROM section_embeddings WHERE model = ?", model)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	hashes := make(map[string]string)
	for rows.Next() {
		var id, hash string
		if err := rows.Scan(&id, &hash); err != nil {
			return nil, err
		}
		hashes[id] = hash
	}
	return hashes, rows.Err()
}

// SaveEmbedding stores or replaces the embedding of a section.
func (r *SQLiteRepository) SaveEmbedding(ctx context.Context, sectionID string, vector []byte, dimensions int, model, contentHash string) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO section_embeddings (section_id, embedding, dimensions, model, content_hash, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(section_id) DO UPDATE SET embedding = excluded.embedding, dimensions = excluded.dimensions,
			model = excluded.model, content_hash = excluded.content_hash, created_at = excluded.created_at`,
		sectionID, vector, dimensions, model, contentHash, time.Now().UTC())
	return err
}

// DeleteOrphanEmbeddings removes embeddings whose section is gone, empty or belongs to a removed document.
func (r *SQLiteRepository) DeleteOrphanEmbeddings(ctx context.Context) (int64, error) {
	res, err := r.db.ExecContext(ctx, `
		DELETE FROM section_embeddings
		WHERE section_id NOT IN (
			SELECT s.id FROM sections s JOIN documents d ON d.id = s.document_id WHERE s.content != ''
		)`)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
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

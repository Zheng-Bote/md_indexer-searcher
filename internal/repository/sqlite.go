// SPDX-License-Identifier: Apache-2.0

package repository

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/md-indexer/internal/entities"
	_ "modernc.org/sqlite"
)

type SQLiteRepository struct {
	db *sql.DB
}

// filePragmas lets md-indexer (writer) and md-api/md-search (readers) share one database file concurrently.
const filePragmas = "_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"

func NewSQLiteRepository(dsn string) (*SQLiteRepository, error) {
	// Only plain file paths get the pragmas; in-memory and explicitly parameterized DSNs are used as given.
	if dsn != ":memory:" && !strings.HasPrefix(dsn, "file:") && !strings.Contains(dsn, "?") {
		dsn += "?" + filePragmas
	}
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
	CREATE TABLE IF NOT EXISTS documents (id TEXT PRIMARY KEY, path TEXT UNIQUE, title TEXT, hash TEXT, modified_at DATETIME, indexed_at DATETIME, markdown TEXT);
	CREATE TABLE IF NOT EXISTS sections (id TEXT PRIMARY KEY, document_id TEXT, heading TEXT, level INTEGER, content TEXT, FOREIGN KEY(document_id) REFERENCES documents(id));
	CREATE TABLE IF NOT EXISTS links (source_document_id TEXT, target_document_id TEXT, FOREIGN KEY(source_document_id) REFERENCES documents(id));
	CREATE TABLE IF NOT EXISTS tags (id TEXT PRIMARY KEY, name TEXT UNIQUE);
	CREATE TABLE IF NOT EXISTS document_tags (document_id TEXT, tag_id TEXT, PRIMARY KEY(document_id, tag_id));
	CREATE TABLE IF NOT EXISTS metadata (document_id TEXT, key TEXT, value TEXT, FOREIGN KEY(document_id) REFERENCES documents(id));
	
	CREATE TABLE IF NOT EXISTS entity_types (id TEXT PRIMARY KEY, name TEXT UNIQUE);
	CREATE TABLE IF NOT EXISTS entities (id TEXT PRIMARY KEY, type_id TEXT, name TEXT, document_id TEXT, FOREIGN KEY(type_id) REFERENCES entity_types(id));
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

	if _, err := db.Exec(query); err != nil {
		return err
	}
	// Databases built before documents.markdown get the column; the indexer re-parses rows where it is NULL.
	_, _ = db.Exec("ALTER TABLE documents ADD COLUMN markdown TEXT")
	// Ensure existing databases built before entities.document_id was added get the column and backfill.
	_, _ = db.Exec("ALTER TABLE entities ADD COLUMN document_id TEXT")
	_, _ = db.Exec("UPDATE entities SET document_id = (SELECT d.id FROM documents d WHERE d.title = entities.id COLLATE NOCASE LIMIT 1) WHERE (document_id IS NULL OR document_id = '') AND type_id != 'unresolved'")
	return nil
}

func (r *SQLiteRepository) UpsertDocument(ctx context.Context, doc *entities.Document) error {
	var execErr error
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	exec := txExec(ctx, tx, &execErr)

	// 1. Wipe previous Document data (both by previous ID at path and new doc.ID)
	var prevID, prevTitle string
	_ = tx.QueryRowContext(ctx, "SELECT id, title FROM documents WHERE path = ?", doc.Path).Scan(&prevID, &prevTitle)
	for _, id := range []string{prevID, doc.ID} {
		if id == "" {
			continue
		}
		exec("DELETE FROM sections WHERE document_id = ?", id)
		exec("DELETE FROM links WHERE source_document_id = ?", id)
		exec("DELETE FROM fts_documents WHERE id = ?", id)
		exec("DELETE FROM fts_sections WHERE document_id = ?", id)
		exec("DELETE FROM document_tags WHERE document_id = ?", id)
		exec("DELETE FROM metadata WHERE document_id = ?", id)
	}
	exec("DELETE FROM documents WHERE path = ?", doc.Path)

	// 2. Wipe Entity properties and outgoing relations for this document
	entID := doc.EntityName
	if entID == "" {
		entID = doc.Title
	}
	if prevTitle != "" && prevTitle != entID {
		exec("DELETE FROM entity_properties WHERE entity_id = ?", prevTitle)
		exec("DELETE FROM entity_relations WHERE source_entity_id = ?", prevTitle)
	}
	exec("DELETE FROM entity_properties WHERE entity_id = ?", entID)
	exec("DELETE FROM entity_relations WHERE source_entity_id = ?", entID)

	// 3. Insert Document data
	exec("INSERT INTO documents (id, path, title, hash, modified_at, indexed_at, markdown) VALUES (?, ?, ?, ?, ?, ?, ?)", doc.ID, doc.Path, doc.Title, doc.Hash, doc.ModifiedAt, doc.IndexedAt, doc.Markdown)
	exec("INSERT INTO fts_documents (id, title, content, tags) VALUES (?, ?, ?, ?)", doc.ID, doc.Title, doc.Content, strings.Join(doc.Tags, " "))

	// Persist document metadata
	for k, v := range doc.Metadata {
		switch val := v.(type) {
		case string:
			exec("INSERT INTO metadata (document_id, key, value) VALUES (?, ?, ?)", doc.ID, k, val)
		case int, int64, float64, bool:
			exec("INSERT INTO metadata (document_id, key, value) VALUES (?, ?, ?)", doc.ID, k, fmt.Sprintf("%v", val))
		}
	}

	// Persist document tags
	for _, tag := range doc.Tags {
		exec("INSERT OR IGNORE INTO tags (id, name) VALUES (?, ?)", tag, tag)
		exec("INSERT OR IGNORE INTO document_tags (document_id, tag_id) VALUES (?, ?)", doc.ID, tag)
	}

	for _, link := range doc.Links {
		exec("INSERT INTO links (source_document_id, target_document_id) VALUES (?, ?)", link.SourceDocID, link.TargetDocID)
	}

	tags := strings.Join(doc.Tags, " ")
	metadata := propertyValues(doc.EntityProperties)
	for _, sec := range doc.Sections {
		exec("INSERT INTO sections (id, document_id, heading, level, content) VALUES (?, ?, ?, ?, ?)", sec.ID, doc.ID, sec.Heading, sec.Level, sec.Content)
		if sec.Content != "" {
			exec("INSERT INTO fts_sections (section_id, document_id, title, heading, tags, content, metadata) VALUES (?, ?, ?, ?, ?, ?, ?)", sec.ID, doc.ID, doc.Title, sec.Heading, tags, sec.Content, metadata)
		}
	}

	// 4. Insert Entity data if IsEntity
	if doc.IsEntity {
		exec("INSERT OR IGNORE INTO entity_types (id, name) VALUES ('unresolved', 'unresolved')")
		exec("INSERT OR IGNORE INTO entity_types (id, name) VALUES (?, ?)", doc.EntityType, doc.EntityType)
		exec("INSERT INTO entities (id, type_id, name, document_id) VALUES (?, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET type_id = excluded.type_id, name = excluded.name, document_id = excluded.document_id", entID, doc.EntityType, doc.EntityName, doc.ID)

		for k, v := range doc.EntityProperties {
			exec("INSERT INTO entity_properties (entity_id, property_key, property_value) VALUES (?, ?, ?)", entID, k, v)
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
			exec("INSERT OR IGNORE INTO entities (id, type_id, name, document_id) VALUES (?, 'unresolved', ?, '')", rel.TargetID, rel.TargetID)

			exec("INSERT INTO entity_relations (source_entity_id, target_entity_id, relation_type) VALUES (?, ?, ?)", rel.SourceID, rel.TargetID, rel.Type)
		}
	} else {
		exec("DELETE FROM entities WHERE id = ? OR document_id = ?", entID, doc.ID)
	}

	if execErr != nil {
		return execErr
	}
	return tx.Commit()
}

// txExec returns an executor that records the first failing statement in *errp and skips the
// remaining ones, so the caller can roll back instead of committing a partial document.
func txExec(ctx context.Context, tx *sql.Tx, errp *error) func(query string, args ...any) {
	return func(query string, args ...any) {
		if *errp != nil {
			return
		}
		if _, err := tx.ExecContext(ctx, query, args...); err != nil {
			*errp = fmt.Errorf("exec %q: %w", query, err)
		}
	}
}

// RekeyEmbeddings moves the section embeddings of a document from oldDocID to newDocID
// (section IDs are "<docID>-<suffix>"), so a document whose ID scheme changed keeps its vectors.
func (r *SQLiteRepository) RekeyEmbeddings(ctx context.Context, oldDocID, newDocID string) (int64, error) {
	prefix := oldDocID + "-"
	res, err := r.db.ExecContext(ctx,
		"UPDATE OR IGNORE section_embeddings SET section_id = ? || substr(section_id, ?) WHERE substr(section_id, 1, ?) = ?",
		newDocID, len(oldDocID)+1, len(prefix), prefix)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
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
	var execErr error
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	exec := txExec(ctx, tx, &execErr)

	var docID, title string
	if err := tx.QueryRowContext(ctx, "SELECT id, title FROM documents WHERE path = ?", path).Scan(&docID, &title); err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		return err
	}

	exec("DELETE FROM section_embeddings WHERE section_id IN (SELECT id FROM sections WHERE document_id = ?)", docID)
	exec("DELETE FROM sections WHERE document_id = ?", docID)
	exec("DELETE FROM links WHERE source_document_id = ?", docID)
	exec("DELETE FROM document_tags WHERE document_id = ?", docID)
	exec("DELETE FROM metadata WHERE document_id = ?", docID)
	exec("DELETE FROM fts_documents WHERE id = ?", docID)
	exec("DELETE FROM fts_sections WHERE document_id = ?", docID)
	exec("DELETE FROM documents WHERE id = ?", docID)
	if title != "" {
		exec("DELETE FROM entity_properties WHERE entity_id = ?", title)
		exec("DELETE FROM entity_relations WHERE source_entity_id = ?", title)
		exec("DELETE FROM entities WHERE id = ?", title)
	}
	if execErr != nil {
		return execErr
	}
	return tx.Commit()
}

func (r *SQLiteRepository) GetDocumentByPath(ctx context.Context, path string) (*entities.Document, error) {
	row := r.db.QueryRowContext(ctx, "SELECT id, path, hash, markdown IS NOT NULL FROM documents WHERE path = ?", path)
	doc := &entities.Document{}
	err := row.Scan(&doc.ID, &doc.Path, &doc.Hash, &doc.HasMarkdown)
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
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []entities.SearchResult
	for rows.Next() {
		var res entities.SearchResult
		if err := rows.Scan(&res.DocumentID, &res.Title, &res.Snippet, &res.Rank); err != nil {
			return nil, err
		}
		results = append(results, res)
	}
	return results, nil
}

func (r *SQLiteRepository) GetAllDocuments(ctx context.Context) ([]entities.Document, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT id, path, title FROM documents")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []entities.Document
	for rows.Next() {
		var doc entities.Document
		if err := rows.Scan(&doc.ID, &doc.Path, &doc.Title); err != nil {
			return nil, err
		}
		results = append(results, doc)
	}
	return results, nil
}

// ListDocumentPaths returns all document file paths currently stored in the database.
func (r *SQLiteRepository) ListDocumentPaths(ctx context.Context) ([]string, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT path FROM documents")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var paths []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		paths = append(paths, p)
	}
	return paths, rows.Err()
}

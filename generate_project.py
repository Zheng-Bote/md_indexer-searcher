import os

files = {
    "cmd/indexer/main.go": """package main

import (
\t"context"
\t"flag"
\t"net/http"
\t"os"
\t"os/signal"
\t"syscall"
\t"time"

\t"github.com/md-indexer/internal/api"
\t"github.com/md-indexer/internal/config"
\t"github.com/md-indexer/internal/repository"
\t"github.com/md-indexer/internal/service"
\t"github.com/md-indexer/internal/watcher"
\t"github.com/rs/zerolog"
\t"github.com/rs/zerolog/log"
)

func main() {
\tlog.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})

\tdir := flag.String("dir", ".", "Directory to index")
\tdbPath := flag.String("db", "index.db", "Path to SQLite database")
\tport := flag.String("port", "8080", "HTTP server port")
\tflag.Parse()

\tcfg := config.Config{
\t\tDirectory: *dir,
\t\tDBPath:    *dbPath,
\t\tPort:      *port,
\t}

\trepo, err := repository.NewSQLiteRepository(cfg.DBPath)
\tif err != nil {
\t\tlog.Fatal().Err(err).Msg("Failed to initialize repository")
\t}
\tdefer repo.Close()

\tindexerService := service.NewIndexerService(repo, cfg.Directory)

\tlog.Info().Msg("Starting full re-index...")
\tif err := indexerService.IndexAll(context.Background()); err != nil {
\t\tlog.Fatal().Err(err).Msg("Failed to index directory")
\t}

\tfsWatcher, err := watcher.NewWatcher(cfg.Directory, indexerService)
\tif err != nil {
\t\tlog.Fatal().Err(err).Msg("Failed to initialize watcher")
\t}
\tdefer fsWatcher.Close()
\tgo fsWatcher.Start()

\tserver := api.NewServer(cfg, repo)
\thttpServer := &http.Server{
\t\tAddr:    ":" + cfg.Port,
\t\tHandler: server.Handler(),
\t}

\tgo func() {
\t\tlog.Info().Str("port", cfg.Port).Msg("Starting HTTP server")
\t\tif err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
\t\t\tlog.Fatal().Err(err).Msg("HTTP server failed")
\t\t}
\t}()

\tquit := make(chan os.Signal, 1)
\tsignal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
\t<-quit
\tlog.Info().Msg("Shutting down gracefully...")

\tctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
\tdefer cancel()
\tif err := httpServer.Shutdown(ctx); err != nil {
\t\tlog.Fatal().Err(err).Msg("Server shutdown failed")
\t}
}
""",
    "internal/config/config.go": """package config

type Config struct {
\tDirectory string
\tDBPath    string
\tPort      string
}
""",
    "internal/entities/models.go": """package entities

import "time"

type Document struct {
\tID         string
\tPath       string
\tTitle      string
\tHash       string
\tModifiedAt time.Time
\tIndexedAt  time.Time
\tSections   []Section
\tLinks      []Link
\tTags       []string
\tMetadata   map[string]interface{}
}

type Section struct {
\tID         string
\tDocumentID string
\tHeading    string
\tLevel      int
\tContent    string
}

type Link struct {
\tSourceDocID string
\tTargetDocID string
}

type SearchResult struct {
\tDocumentID string  `json:"document_id"`
\tPath       string  `json:"path"`
\tTitle      string  `json:"title"`
\tSnippet    string  `json:"snippet"`
\tRank       float64 `json:"rank"`
}
""",
    "internal/parser/markdown.go": """package parser

import (
\t"bytes"
\t"crypto/sha256"
\t"encoding/hex"
\t"os"
\t"path/filepath"
\t"strings"
\t"time"

\t"github.com/md-indexer/internal/entities"
\t"github.com/yuin/goldmark"
\t"github.com/yuin/goldmark-meta"
\t"github.com/yuin/goldmark/ast"
\t"github.com/yuin/goldmark/text"
)

type Parser struct {
\tmd goldmark.Markdown
}

func NewParser() *Parser {
\treturn &Parser{
\t\tmd: goldmark.New(
\t\t\tgoldmark.WithExtensions(meta.Meta),
\t\t),
\t}
}

func (p *Parser) ParseFile(path string) (*entities.Document, error) {
\tcontent, err := os.ReadFile(path)
\tif err != nil {
\t\treturn nil, err
\t}

\tstat, err := os.Stat(path)
\tif err != nil {
\t\treturn nil, err
\t}

\thashBytes := sha256.Sum256(content)
\thashStr := hex.EncodeToString(hashBytes[:])

\tdoc := &entities.Document{
\t\tID:         hashStr, // Simple ID generation, could be UUID or path hash
\t\tPath:       path,
\t\tTitle:      filepath.Base(path),
\t\tHash:       hashStr,
\t\tModifiedAt: stat.ModTime(),
\t\tIndexedAt:  time.Now(),
\t\tMetadata:   make(map[string]interface{}),
\t}

\tp.extractAST(doc, content)
\treturn doc, nil
}

func (p *Parser) extractAST(doc *entities.Document, content []byte) {
\tctx := parserContext()
\tnode := p.md.Parser().Parse(text.NewReader(content), goldmark.WithParserContext(ctx))

\tmetaData := meta.Get(ctx)
\tif metaData != nil {
\t\tfor k, v := range metaData {
\t\t\tdoc.Metadata[k] = v
\t\t}
\t}

\t// Traverse AST for sections, links, and tags
\tast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
\t\tif !entering {
\t\t\treturn ast.WalkContinue, nil
\t\t}

\t\t// Simplistic extraction logic for sections, can be expanded
\t\tif heading, ok := n.(*ast.Heading); ok {
\t\t\tdoc.Sections = append(doc.Sections, entities.Section{
\t\t\t\tID:      doc.ID + "-" + string(heading.Text(content)), // Simplify
\t\t\t\tHeading: string(heading.Text(content)),
\t\t\t\tLevel:   heading.Level,
\t\t\t})
\t\t}

\t\tif textNode, ok := n.(*ast.Text); ok {
\t\t\tval := string(textNode.Segment.Value(content))
\t\t\tif strings.HasPrefix(val, "[[") && strings.HasSuffix(val, "]]") {
\t\t\t\ttarget := strings.TrimSuffix(strings.TrimPrefix(val, "[["), "]]")
\t\t\t\tdoc.Links = append(doc.Links, entities.Link{
\t\t\t\t\tSourceDocID: doc.ID,
\t\t\t\t\tTargetDocID: target, // Will need resolution
\t\t\t\t})
\t\t\t}
\t\t}
\t\treturn ast.WalkContinue, nil
\t})
}

func parserContext() goldmark.parser.Context {
\treturn goldmark.parser.NewContext()
}
""",
    "internal/repository/sqlite.go": """package repository

import (
\t"context"
\t"database/sql"
\t"fmt"
\t"strings"

\t"github.com/md-indexer/internal/entities"
\t_ "modernc.org/sqlite"
)

type SQLiteRepository struct {
\tdb *sql.DB
}

func NewSQLiteRepository(dsn string) (*SQLiteRepository, error) {
\tdb, err := sql.Open("sqlite", dsn)
\tif err != nil {
\t\treturn nil, err
\t}

\tif err := migrate(db); err != nil {
\t\treturn nil, err
\t}

\treturn &SQLiteRepository{db: db}, nil
}

func (r *SQLiteRepository) Close() error {
\treturn r.db.Close()
}

func migrate(db *sql.DB) error {
\tquery := `
\tCREATE TABLE IF NOT EXISTS documents (
\t\tid TEXT PRIMARY KEY,
\t\tpath TEXT UNIQUE,
\t\ttitle TEXT,
\t\thash TEXT,
\t\tmodified_at DATETIME,
\t\tindexed_at DATETIME
\t);

\tCREATE TABLE IF NOT EXISTS sections (
\t\tid TEXT PRIMARY KEY,
\t\tdocument_id TEXT,
\t\theading TEXT,
\t\tlevel INTEGER,
\t\tcontent TEXT,
\t\tFOREIGN KEY(document_id) REFERENCES documents(id)
\t);

\tCREATE TABLE IF NOT EXISTS links (
\t\tsource_document_id TEXT,
\t\ttarget_document_id TEXT,
\t\tFOREIGN KEY(source_document_id) REFERENCES documents(id)
\t);

\tCREATE TABLE IF NOT EXISTS tags (
\t\tid TEXT PRIMARY KEY,
\t\tname TEXT UNIQUE
\t);

\tCREATE TABLE IF NOT EXISTS document_tags (
\t\tdocument_id TEXT,
\t\ttag_id TEXT,
\t\tPRIMARY KEY(document_id, tag_id)
\t);

\tCREATE TABLE IF NOT EXISTS metadata (
\t\tdocument_id TEXT,
\t\tkey TEXT,
\t\tvalue TEXT,
\t\tFOREIGN KEY(document_id) REFERENCES documents(id)
\t);

\tCREATE VIRTUAL TABLE IF NOT EXISTS fts_documents USING fts5(
\t\tid UNINDEXED,
\t\ttitle,
\t\tcontent,
\t\ttags
\t);
\t`
\t_, err := db.Exec(query)
\treturn err
}

func (r *SQLiteRepository) UpsertDocument(ctx context.Context, doc *entities.Document) error {
\ttx, err := r.db.BeginTx(ctx, nil)
\tif err != nil {
\t\treturn err
\t}
\tdefer tx.Rollback()

\t// Delete existing
\t_, _ = tx.ExecContext(ctx, "DELETE FROM documents WHERE path = ?", doc.Path)
\t_, _ = tx.ExecContext(ctx, "DELETE FROM sections WHERE document_id = ?", doc.ID)
\t_, _ = tx.ExecContext(ctx, "DELETE FROM links WHERE source_document_id = ?", doc.ID)
\t_, _ = tx.ExecContext(ctx, "DELETE FROM fts_documents WHERE id = ?", doc.ID)

\t// Insert document
\t_, err = tx.ExecContext(ctx, 
\t\t"INSERT INTO documents (id, path, title, hash, modified_at, indexed_at) VALUES (?, ?, ?, ?, ?, ?)",
\t\tdoc.ID, doc.Path, doc.Title, doc.Hash, doc.ModifiedAt, doc.IndexedAt)
\tif err != nil {
\t\treturn err
\t}

\t// FTS
\t_, err = tx.ExecContext(ctx,
\t\t"INSERT INTO fts_documents (id, title, content, tags) VALUES (?, ?, ?, ?)",
\t\tdoc.ID, doc.Title, "", strings.Join(doc.Tags, " "))
\tif err != nil {
\t\treturn err
\t}

\treturn tx.Commit()
}

func (r *SQLiteRepository) RemoveDocument(ctx context.Context, path string) error {
\t_, err := r.db.ExecContext(ctx, "DELETE FROM documents WHERE path = ?", path)
\treturn err
}

func (r *SQLiteRepository) GetDocumentByPath(ctx context.Context, path string) (*entities.Document, error) {
\trow := r.db.QueryRowContext(ctx, "SELECT id, path, hash FROM documents WHERE path = ?", path)
\tdoc := &entities.Document{}
\terr := row.Scan(&doc.ID, &doc.Path, &doc.Hash)
\tif err != nil {
\t\tif err == sql.ErrNoRows {
\t\t\treturn nil, nil
\t\t}
\t\treturn nil, err
\t}
\treturn doc, nil
}

func (r *SQLiteRepository) Search(ctx context.Context, query string) ([]entities.SearchResult, error) {
\t// BM25 ranking
\trows, err := r.db.QueryContext(ctx, `
\t\tSELECT id, title, snippet(fts_documents, 2, '<b>', '</b>', '...', 64) as snippet, rank 
\t\tFROM fts_documents 
\t\tWHERE fts_documents MATCH ? 
\t\tORDER BY rank 
\t\tLIMIT 20`, query)
\tif err != nil {
\t\treturn nil, err
\t}
\tdefer rows.Close()

\tvar results []entities.SearchResult
\tfor rows.Next() {
\t\tvar res entities.SearchResult
\t\tif err := rows.Scan(&res.DocumentID, &res.Title, &res.Snippet, &res.Rank); err != nil {
\t\t\treturn nil, err
\t\t}
\t\tresults = append(results, res)
\t}
\treturn results, nil
}

func (r *SQLiteRepository) GetAllDocuments(ctx context.Context) ([]entities.Document, error) {
\trows, err := r.db.QueryContext(ctx, "SELECT id, path, title FROM documents")
\tif err != nil {
\t\treturn nil, err
\t}
\tdefer rows.Close()

\tvar results []entities.Document
\tfor rows.Next() {
\t\tvar doc entities.Document
\t\tif err := rows.Scan(&doc.ID, &doc.Path, &doc.Title); err != nil {
\t\t\treturn nil, err
\t\t}
\t\tresults = append(results, doc)
\t}
\treturn results, nil
}
""",
    "internal/service/indexer.go": """package service

import (
\t"context"
\t"os"
\t"path/filepath"
\t"strings"

\t"github.com/md-indexer/internal/parser"
\t"github.com/md-indexer/internal/repository"
\t"github.com/rs/zerolog/log"
)

type IndexerService struct {
\trepo   *repository.SQLiteRepository
\tdir    string
\tparser *parser.Parser
}

func NewIndexerService(repo *repository.SQLiteRepository, dir string) *IndexerService {
\treturn &IndexerService{
\t\trepo:   repo,
\t\tdir:    dir,
\t\tparser: parser.NewParser(),
\t}
}

func (s *IndexerService) IndexAll(ctx context.Context) error {
\treturn filepath.Walk(s.dir, func(path string, info os.FileInfo, err error) error {
\t\tif err != nil {
\t\t\treturn err
\t\t}
\t\tif !info.IsDir() && strings.HasSuffix(strings.ToLower(path), ".md") {
\t\t\treturn s.IndexFile(ctx, path)
\t\t}
\t\treturn nil
\t})
}

func (s *IndexerService) IndexFile(ctx context.Context, path string) error {
\texistingDoc, err := s.repo.GetDocumentByPath(ctx, path)
\tif err != nil {
\t\treturn err
\t}

\tdoc, err := s.parser.ParseFile(path)
\tif err != nil {
\t\tlog.Error().Err(err).Str("path", path).Msg("Failed to parse file")
\t\treturn err
\t}

\t// Skip if unchanged
\tif existingDoc != nil && existingDoc.Hash == doc.Hash {
\t\tlog.Debug().Str("path", path).Msg("File unchanged, skipping")
\t\treturn nil
\t}

\tlog.Info().Str("path", path).Msg("Indexing file")
\treturn s.repo.UpsertDocument(ctx, doc)
}

func (s *IndexerService) RemoveFile(ctx context.Context, path string) error {
\tlog.Info().Str("path", path).Msg("Removing file from index")
\treturn s.repo.RemoveDocument(ctx, path)
}
""",
    "internal/watcher/watcher.go": """package watcher

import (
\t"context"
\t"path/filepath"
\t"strings"

\t"github.com/fsnotify/fsnotify"
\t"github.com/md-indexer/internal/service"
\t"github.com/rs/zerolog/log"
)

type Watcher struct {
\tfsWatcher *fsnotify.Watcher
\tindexer   *service.IndexerService
\tdir       string
}

func NewWatcher(dir string, indexer *service.IndexerService) (*Watcher, error) {
\tfsw, err := fsnotify.NewWatcher()
\tif err != nil {
\t\treturn nil, err
\t}

\t// Add all subdirectories
\terr = filepath.Walk(dir, func(path string, info interface{ IsDir() bool }, err error) error {
\t\tif info != nil && info.IsDir() {
\t\t\treturn fsw.Add(path)
\t\t}
\t\treturn nil
\t})
\tif err != nil {
\t\treturn nil, err
\t}

\treturn &Watcher{
\t\tfsWatcher: fsw,
\t\tindexer:   indexer,
\t\tdir:       dir,
\t}, nil
}

func (w *Watcher) Start() {
\tfor {
\t\tselect {
\t\tcase event, ok := <-w.fsWatcher.Events:
\t\t\tif !ok {
\t\t\t\treturn
\t\t\t}
\t\t\tif !strings.HasSuffix(strings.ToLower(event.Name), ".md") {
\t\t\t\tcontinue
\t\t\t}

\t\t\tctx := context.Background()
\t\t\tlog.Info().Str("event", event.Op.String()).Str("file", event.Name).Msg("File system event")

\t\t\tif event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
\t\t\t\tw.indexer.IndexFile(ctx, event.Name)
\t\t\t} else if event.Has(fsnotify.Remove) || event.Has(fsnotify.Rename) {
\t\t\t\tw.indexer.RemoveFile(ctx, event.Name)
\t\t\t}
\t\tcase err, ok := <-w.fsWatcher.Errors:
\t\t\tif !ok {
\t\t\t\treturn
\t\t\t}
\t\t\tlog.Error().Err(err).Msg("Watcher error")
\t\t}
\t}
}

func (w *Watcher) Close() error {
\treturn w.fsWatcher.Close()
}
""",
    "internal/api/server.go": """package api

import (
\t"encoding/json"
\t"net/http"

\t"github.com/go-chi/chi/v5"
\t"github.com/go-chi/chi/v5/middleware"
\t"github.com/md-indexer/internal/config"
\t"github.com/md-indexer/internal/repository"
)

type Server struct {
\tcfg  config.Config
\trepo *repository.SQLiteRepository
}

func NewServer(cfg config.Config, repo *repository.SQLiteRepository) *Server {
\treturn &Server{
\t\tcfg:  cfg,
\t\trepo: repo,
\t}
}

func (s *Server) Handler() http.Handler {
\tr := chi.NewRouter()
\tr.Use(middleware.Logger)
\tr.Use(middleware.Recoverer)

\tr.Get("/health", s.healthCheck)
\tr.Get("/documents", s.getDocuments)
\tr.Get("/search", s.search)
\t// Additional routes like /documents/{id}, /graph/document/{id}, etc.
\treturn r
}

func (s *Server) healthCheck(w http.ResponseWriter, r *http.Request) {
\tw.WriteHeader(http.StatusOK)
\tw.Write([]byte(`{"status":"ok"}`))
}

func (s *Server) getDocuments(w http.ResponseWriter, r *http.Request) {
\tdocs, err := s.repo.GetAllDocuments(r.Context())
\tif err != nil {
\t\thttp.Error(w, err.Error(), http.StatusInternalServerError)
\t\treturn
\t}

\tw.Header().Set("Content-Type", "application/json")
\tjson.NewEncoder(w).Encode(docs)
}

func (s *Server) search(w http.ResponseWriter, r *http.Request) {
\tquery := r.URL.Query().Get("q")
\tif query == "" {
\t\thttp.Error(w, "missing query parameter 'q'", http.StatusBadRequest)
\t\treturn
\t}

\tresults, err := s.repo.Search(r.Context(), query)
\tif err != nil {
\t\thttp.Error(w, err.Error(), http.StatusInternalServerError)
\t\treturn
\t}

\tw.Header().Set("Content-Type", "application/json")
\tjson.NewEncoder(w).Encode(results)
}
""",
    "internal/parser/markdown_test.go": """package parser

import (
\t"os"
\t"testing"
)

func TestParseFile(t *testing.T) {
\t// Create temp markdown file
\tcontent := `---
type: component
owner: robert
status: active
---
# Architecture
This is a test document with a link to [[Encryption]] and a tag #security.
`
\tf, _ := os.CreateTemp("", "*.md")
\tdefer os.Remove(f.Name())
\tf.WriteString(content)
\tf.Close()

\tp := NewParser()
\tdoc, err := p.ParseFile(f.Name())
\tif err != nil {
\t\tt.Fatalf("ParseFile failed: %v", err)
\t}

\tif doc.Metadata["type"] != "component" {
\t\tt.Errorf("Expected type metadata 'component', got %v", doc.Metadata["type"])
\t}
}
"""
}

for path, content in files.items():
    os.makedirs(os.path.dirname(path), exist_ok=True)
    with open(path, "w") as f:
        f.write(content)

print("Files generated successfully.")

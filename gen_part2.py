import os

files = {
    "internal/repository/sqlite.go": """package repository

import (
\t"context"
\t"database/sql"
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

func (r *SQLiteRepository) GetDB() *sql.DB {
\treturn r.db
}

func migrate(db *sql.DB) error {
\tquery := `
\tCREATE TABLE IF NOT EXISTS documents (id TEXT PRIMARY KEY, path TEXT UNIQUE, title TEXT, hash TEXT, modified_at DATETIME, indexed_at DATETIME);
\tCREATE TABLE IF NOT EXISTS sections (id TEXT PRIMARY KEY, document_id TEXT, heading TEXT, level INTEGER, content TEXT, FOREIGN KEY(document_id) REFERENCES documents(id));
\tCREATE TABLE IF NOT EXISTS links (source_document_id TEXT, target_document_id TEXT, FOREIGN KEY(source_document_id) REFERENCES documents(id));
\tCREATE TABLE IF NOT EXISTS tags (id TEXT PRIMARY KEY, name TEXT UNIQUE);
\tCREATE TABLE IF NOT EXISTS document_tags (document_id TEXT, tag_id TEXT, PRIMARY KEY(document_id, tag_id));
\tCREATE TABLE IF NOT EXISTS metadata (document_id TEXT, key TEXT, value TEXT, FOREIGN KEY(document_id) REFERENCES documents(id));
\t
\tCREATE TABLE IF NOT EXISTS entity_types (id TEXT PRIMARY KEY, name TEXT UNIQUE);
\tCREATE TABLE IF NOT EXISTS entities (id TEXT PRIMARY KEY, type_id TEXT, name TEXT, FOREIGN KEY(type_id) REFERENCES entity_types(id));
\tCREATE TABLE IF NOT EXISTS entity_properties (entity_id TEXT, property_key TEXT, property_value TEXT, FOREIGN KEY(entity_id) REFERENCES entities(id));
\tCREATE TABLE IF NOT EXISTS entity_relations (source_entity_id TEXT, target_entity_id TEXT, relation_type TEXT, FOREIGN KEY(source_entity_id) REFERENCES entities(id), FOREIGN KEY(target_entity_id) REFERENCES entities(id));

\tCREATE VIRTUAL TABLE IF NOT EXISTS fts_documents USING fts5(id UNINDEXED, title, content, tags);`
\t
\t_, err := db.Exec(query)
\treturn err
}

func (r *SQLiteRepository) UpsertDocument(ctx context.Context, doc *entities.Document) error {
\ttx, err := r.db.BeginTx(ctx, nil)
\tif err != nil { return err }
\tdefer tx.Rollback()

\ttx.ExecContext(ctx, "DELETE FROM documents WHERE path = ?", doc.Path)
\ttx.ExecContext(ctx, "DELETE FROM sections WHERE document_id = ?", doc.ID)
\ttx.ExecContext(ctx, "DELETE FROM links WHERE source_document_id = ?", doc.ID)
\ttx.ExecContext(ctx, "DELETE FROM fts_documents WHERE id = ?", doc.ID)

\ttx.ExecContext(ctx, "INSERT INTO documents (id, path, title, hash, modified_at, indexed_at) VALUES (?, ?, ?, ?, ?, ?)", doc.ID, doc.Path, doc.Title, doc.Hash, doc.ModifiedAt, doc.IndexedAt)
\ttx.ExecContext(ctx, "INSERT INTO fts_documents (id, title, content, tags) VALUES (?, ?, ?, ?)", doc.ID, doc.Title, "", strings.Join(doc.Tags, " "))

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
\trows, err := r.db.QueryContext(ctx, `
\t\tSELECT id, title, snippet(fts_documents, 2, '<b>', '</b>', '...', 64) as snippet, rank 
\t\tFROM fts_documents 
\t\tWHERE fts_documents MATCH ? 
\t\tORDER BY rank LIMIT 20`, query)
\tif err != nil { return nil, err }
\tdefer rows.Close()

\tvar results []entities.SearchResult
\tfor rows.Next() {
\t\tvar res entities.SearchResult
\t\tif err := rows.Scan(&res.DocumentID, &res.Title, &res.Snippet, &res.Rank); err != nil { return nil, err }
\t\tresults = append(results, res)
\t}
\treturn results, nil
}

func (r *SQLiteRepository) GetAllDocuments(ctx context.Context) ([]entities.Document, error) {
\trows, err := r.db.QueryContext(ctx, "SELECT id, path, title FROM documents")
\tif err != nil { return nil, err }
\tdefer rows.Close()

\tvar results []entities.Document
\tfor rows.Next() {
\t\tvar doc entities.Document
\t\tif err := rows.Scan(&doc.ID, &doc.Path, &doc.Title); err != nil { return nil, err }
\t\tresults = append(results, doc)
\t}
\treturn results, nil
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

type Entity struct {
\tID         string            `json:"id"`
\tType       string            `json:"type"`
\tName       string            `json:"name"`
\tProperties map[string]string `json:"properties"`
}

type EntityRelation struct {
\tSourceID string `json:"source_id"`
\tTargetID string `json:"target_id"`
\tType     string `json:"type"`
}

type GraphNode struct {
\tID       string `json:"id"`
\tName     string `json:"name"`
\tType     string `json:"type"`
}

type GraphEdge struct {
\tSource   string `json:"source"`
\tTarget   string `json:"target"`
\tRelation string `json:"relation"`
}
""",
    "internal/repository/search_repo.go": """package repository

import (
\t"context"
\t"database/sql"
\t"github.com/md-indexer/internal/entities"
)

type SearchRepository struct {
\tdb *sql.DB
}

func NewSearchRepository(repo *SQLiteRepository) *SearchRepository {
\treturn &SearchRepository{db: repo.GetDB()}
}

func (r *SearchRepository) FindEntityByName(ctx context.Context, name string) (*entities.Entity, error) {
\tvar ent entities.Entity
\tvar typeID string
\terr := r.db.QueryRowContext(ctx, "SELECT id, type_id, name FROM entities WHERE name = ?", name).Scan(&ent.ID, &typeID, &ent.Name)
\tif err != nil {
\t\tif err == sql.ErrNoRows {
\t\t\treturn nil, nil
\t\t}
\t\treturn nil, err
\t}
\t
\terr = r.db.QueryRowContext(ctx, "SELECT name FROM entity_types WHERE id = ?", typeID).Scan(&ent.Type)
\tif err != nil && err != sql.ErrNoRows {
\t\treturn nil, err
\t}

\trows, err := r.db.QueryContext(ctx, "SELECT property_key, property_value FROM entity_properties WHERE entity_id = ?", ent.ID)
\tif err != nil { return nil, err }
\tdefer rows.Close()

\tent.Properties = make(map[string]string)
\tfor rows.Next() {
\t\tvar k, v string
\t\tif err := rows.Scan(&k, &v); err == nil {
\t\t\tent.Properties[k] = v
\t\t}
\t}
\treturn &ent, nil
}

func (r *SearchRepository) FindDocumentByTitle(ctx context.Context, title string) (*entities.Document, error) {
\trow := r.db.QueryRowContext(ctx, "SELECT id, path, title FROM documents WHERE title = ?", title)
\tvar doc entities.Document
\terr := row.Scan(&doc.ID, &doc.Path, &doc.Title)
\tif err != nil {
\t\tif err == sql.ErrNoRows { return nil, nil }
\t\treturn nil, err
\t}
\treturn &doc, nil
}

func (r *SearchRepository) GetBacklinks(ctx context.Context, targetDocID string) ([]string, error) {
\trows, err := r.db.QueryContext(ctx, `
\t\tSELECT d.title 
\t\tFROM links l 
\t\tJOIN documents d ON l.source_document_id = d.id 
\t\tWHERE l.target_document_id = ?`, targetDocID)
\tif err != nil { return nil, err }
\tdefer rows.Close()
\t
\tvar links []string
\tfor rows.Next() {
\t\tvar title string
\t\tif err := rows.Scan(&title); err == nil {
\t\t\tlinks = append(links, title)
\t\t}
\t}
\treturn links, nil
}
""",
    "internal/search/engine.go": """package search

import (
\t"context"
\t"github.com/md-indexer/internal/entities"
\t"github.com/md-indexer/internal/repository"
)

type SearchEngine interface {
\tSearch(ctx context.Context, query string) ([]entities.SearchResult, error)
\tBacklinks(ctx context.Context, name string) ([]string, error)
\tRelations(ctx context.Context, name string) ([]entities.EntityRelation, error)
\tImpact(ctx context.Context, name string) ([]string, error)
\tGraph(ctx context.Context, name string) (interface{}, error)
\tFindEntity(ctx context.Context, name string) (*entities.Entity, error)
\tFindDocument(ctx context.Context, name string) (*entities.Document, error)
}

type engineImpl struct {
\tsRepo *repository.SearchRepository
\tqRepo *repository.SQLiteRepository
}

func NewSearchEngine(qRepo *repository.SQLiteRepository) SearchEngine {
\treturn &engineImpl{
\t\tqRepo: qRepo,
\t\tsRepo: repository.NewSearchRepository(qRepo),
\t}
}

func (e *engineImpl) Search(ctx context.Context, query string) ([]entities.SearchResult, error) {
\treturn e.qRepo.Search(ctx, query)
}

func (e *engineImpl) FindEntity(ctx context.Context, name string) (*entities.Entity, error) {
\treturn e.sRepo.FindEntityByName(ctx, name)
}

func (e *engineImpl) FindDocument(ctx context.Context, name string) (*entities.Document, error) {
\treturn e.sRepo.FindDocumentByTitle(ctx, name)
}

func (e *engineImpl) Backlinks(ctx context.Context, name string) ([]string, error) {
\tdoc, err := e.FindDocument(ctx, name)
\tif err != nil || doc == nil { return []string{}, err }
\treturn e.sRepo.GetBacklinks(ctx, doc.ID)
}

func (e *engineImpl) Relations(ctx context.Context, name string) ([]entities.EntityRelation, error) {
\treturn []entities.EntityRelation{}, nil
}

func (e *engineImpl) Impact(ctx context.Context, name string) ([]string, error) {
\treturn []string{}, nil
}

func (e *engineImpl) Graph(ctx context.Context, name string) (interface{}, error) {
\treturn nil, nil
}
""",
    "internal/graph/engine.go": """package graph

import (
\t"context"
\t"github.com/md-indexer/internal/entities"
\t"github.com/md-indexer/internal/repository"
)

type GraphEngine interface {
\tDirectRelations(ctx context.Context, entityID string) ([]entities.EntityRelation, error)
\tReverseRelations(ctx context.Context, entityID string) ([]entities.EntityRelation, error)
\tDependencyTree(ctx context.Context, entityID string) (interface{}, error)
\tImpactAnalysis(ctx context.Context, entityID string) (interface{}, error)
\tShortestPath(ctx context.Context, sourceID, targetID string) ([]string, error)
}

type graphImpl struct {
\trepo *repository.SQLiteRepository
}

func NewGraphEngine(repo *repository.SQLiteRepository) GraphEngine {
\treturn &graphImpl{repo: repo}
}

func (g *graphImpl) DirectRelations(ctx context.Context, entityID string) ([]entities.EntityRelation, error) {
\treturn nil, nil
}

func (g *graphImpl) ReverseRelations(ctx context.Context, entityID string) ([]entities.EntityRelation, error) {
\treturn nil, nil
}

func (g *graphImpl) DependencyTree(ctx context.Context, entityID string) (interface{}, error) {
\treturn nil, nil
}

func (g *graphImpl) ImpactAnalysis(ctx context.Context, entityID string) (interface{}, error) {
\treturn nil, nil
}

func (g *graphImpl) ShortestPath(ctx context.Context, sourceID, targetID string) ([]string, error) {
\treturn nil, nil
}
""",
    "cmd/md-search/main.go": """package main

import (
\t"context"
\t"encoding/json"
\t"fmt"
\t"os"

\t"github.com/md-indexer/internal/repository"
\t"github.com/md-indexer/internal/search"
\t"github.com/spf13/cobra"
)

var dbPath string
var jsonOutput bool

func main() {
\trootCmd := &cobra.Command{
\t\tUse:   "md-search",
\t\tShort: "CLI Knowledge Explorer",
\t}

\trootCmd.PersistentFlags().StringVar(&dbPath, "db", "index.db", "Path to SQLite database")
\trootCmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "Output as JSON")

\tsearchCmd := &cobra.Command{
\t\tUse:   "search [query]",
\t\tShort: "Full-text search",
\t\tArgs:  cobra.ExactArgs(1),
\t\tRun: func(cmd *cobra.Command, args []string) {
\t\t\trepo, _ := repository.NewSQLiteRepository(dbPath)
\t\t\tse := search.NewSearchEngine(repo)
\t\t\tres, _ := se.Search(context.Background(), args[0])
\t\t\t
\t\t\tif jsonOutput {
\t\t\t\tenc := json.NewEncoder(os.Stdout)
\t\t\t\tenc.SetIndent("", "  ")
\t\t\t\tenc.Encode(res)
\t\t\t} else {
\t\t\t\tfmt.Println("Result\tScore\tTitle")
\t\t\t\tfmt.Println("--------------------------------")
\t\t\t\tfor i, r := range res {
\t\t\t\t\tfmt.Printf("%d\t%.2f\t%s\\n", i+1, r.Rank, r.Title)
\t\t\t\t}
\t\t\t}
\t\t},
\t}

\tdocumentCmd := &cobra.Command{
\t\tUse:   "document [title]",
\t\tShort: "Show document details",
\t\tArgs:  cobra.ExactArgs(1),
\t\tRun: func(cmd *cobra.Command, args []string) {
\t\t\trepo, _ := repository.NewSQLiteRepository(dbPath)
\t\t\tse := search.NewSearchEngine(repo)
\t\t\tdoc, _ := se.FindDocument(context.Background(), args[0])
\t\t\tif doc != nil {
\t\t\t\tfmt.Printf("Title: %s\\nPath: %s\\n", doc.Title, doc.Path)
\t\t\t} else {
\t\t\t\tfmt.Println("Document not found")
\t\t\t}
\t\t},
\t}

\trootCmd.AddCommand(searchCmd, documentCmd)
\t
\tif err := rootCmd.Execute(); err != nil {
\t\tos.Exit(1)
\t}
}
""",
    "cmd/md-api/main.go": """package main

import (
\t"context"
\t"flag"
\t"net/http"
\t"os"
\t"os/signal"
\t"syscall"
\t"time"

\t"github.com/md-indexer/internal/api/rest"
\t"github.com/md-indexer/internal/repository"
\t"github.com/md-indexer/internal/search"
\t"github.com/rs/zerolog"
\t"github.com/rs/zerolog/log"
)

func main() {
\tlog.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})
\tdbPath := flag.String("db", "index.db", "Path to SQLite database")
\tport := flag.String("port", "8081", "HTTP server port")
\tflag.Parse()

\trepo, err := repository.NewSQLiteRepository(*dbPath)
\tif err != nil {
\t\tlog.Fatal().Err(err).Msg("Failed to initialize repository")
\t}
\tdefer repo.Close()

\tsearchEngine := search.NewSearchEngine(repo)
\t
\tserver := rest.NewServer(searchEngine)
\thttpServer := &http.Server{
\t\tAddr:    ":" + *port,
\t\tHandler: server.Handler(),
\t}

\tgo func() {
\t\tlog.Info().Str("port", *port).Msg("Starting API HTTP server")
\t\tif err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
\t\t\tlog.Fatal().Err(err).Msg("HTTP server failed")
\t\t}
\t}()

\tquit := make(chan os.Signal, 1)
\tsignal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
\t<-quit
\tlog.Info().Msg("Shutting down API gracefully...")

\tctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
\tdefer cancel()
\thttpServer.Shutdown(ctx)
}
""",
    "internal/api/rest/server.go": """package rest

import (
\t"encoding/json"
\t"net/http"

\t"github.com/go-chi/chi/v5"
\t"github.com/go-chi/chi/v5/middleware"
\t"github.com/md-indexer/internal/search"
)

type Server struct {
\tsearchEngine search.SearchEngine
}

func NewServer(se search.SearchEngine) *Server {
\treturn &Server{searchEngine: se}
}

func respondJSON(w http.ResponseWriter, data interface{}, err error) {
\tw.Header().Set("Content-Type", "application/json")
\tif err != nil {
\t\tw.WriteHeader(http.StatusInternalServerError)
\t\tjson.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": err.Error()})
\t\treturn
\t}
\tjson.NewEncoder(w).Encode(map[string]interface{}{"success": true, "data": data})
}

func (s *Server) Handler() http.Handler {
\tr := chi.NewRouter()
\tr.Use(middleware.RequestID)
\tr.Use(middleware.Logger)
\tr.Use(middleware.Recoverer)
\tr.Use(middleware.SetHeader("Access-Control-Allow-Origin", "*"))

\tr.Get("/health", func(w http.ResponseWriter, r *http.Request) {
\t\tw.Header().Set("Content-Type", "application/json")
\t\tw.Write([]byte(`{"status":"ok"}`))
\t})

\tr.Get("/api/search", func(w http.ResponseWriter, r *http.Request) {
\t\tq := r.URL.Query().Get("q")
\t\tres, err := s.searchEngine.Search(r.Context(), q)
\t\trespondJSON(w, map[string]interface{}{"results": res}, err)
\t})

\tr.Get("/api/documents/{name}", func(w http.ResponseWriter, r *http.Request) {
\t\tname := chi.URLParam(r, "name")
\t\tres, err := s.searchEngine.FindDocument(r.Context(), name)
\t\trespondJSON(w, res, err)
\t})

\tr.Get("/api/entities/{name}", func(w http.ResponseWriter, r *http.Request) {
\t\tname := chi.URLParam(r, "name")
\t\tres, err := s.searchEngine.FindEntity(r.Context(), name)
\t\trespondJSON(w, res, err)
\t})

\tr.Get("/api/backlinks/{name}", func(w http.ResponseWriter, r *http.Request) {
\t\tname := chi.URLParam(r, "name")
\t\tres, err := s.searchEngine.Backlinks(r.Context(), name)
\t\trespondJSON(w, res, err)
\t})

\treturn r
}
""",
    "Dockerfile": """FROM golang:1.21-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build md-indexer, md-search, and md-api
RUN go build -o /app/md-indexer ./cmd/indexer
RUN go build -o /app/md-search ./cmd/md-search
RUN go build -o /app/md-api ./cmd/md-api

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/md-indexer .
COPY --from=builder /app/md-search .
COPY --from=builder /app/md-api .

EXPOSE 8080 8081
CMD ["./md-api"]
""",
    "Makefile": """
build:
\tgo build -o bin/md-indexer ./cmd/indexer

search:
\tgo build -o bin/md-search ./cmd/md-search

api:
\tgo build -o bin/md-api ./cmd/md-api

api-run: api
\t./bin/md-api -port 8081

test:
\tgo test -v ./...
""",
    "openapi.yaml": """openapi: 3.0.0
info:
  title: MD Indexer API
  version: 1.0.0
paths:
  /health:
    get:
      summary: Health Check
      responses:
        '200':
          description: OK
  /api/search:
    get:
      summary: Full-text Search
      parameters:
        - in: query
          name: q
          schema:
            type: string
      responses:
        '200':
          description: Search Results
"""
}

for path, content in files.items():
    dirname = os.path.dirname(path)
    if dirname:
        os.makedirs(dirname, exist_ok=True)
    with open(path, "w") as f:
        f.write(content)

print("Part 2 files generated successfully.")

// SPDX-License-Identifier: Apache-2.0

package search

import (
	"context"
	"github.com/md-indexer/internal/entities"
	"github.com/md-indexer/internal/graph"
	"github.com/md-indexer/internal/repository"
)

type SearchEngine interface {
	Search(ctx context.Context, query string) ([]entities.SearchResult, error)
	Backlinks(ctx context.Context, name string) ([]string, error)
	Relations(ctx context.Context, name string) ([]entities.EntityRelation, error)
	Impact(ctx context.Context, name string) ([]entities.GraphEdge, error)
	Graph(ctx context.Context, name string) ([]entities.GraphEdge, error)
	FindEntity(ctx context.Context, name string) (*entities.Entity, error)
	FindDocument(ctx context.Context, name string) (*entities.Document, error)
	// Document returns the indexed document at path with its Markdown and resolved wikilinks.
	Document(ctx context.Context, path string) (*entities.DocumentView, error)
	// ResolveDocument resolves a wikilink title as seen from the document at from.
	ResolveDocument(ctx context.Context, title, from string) (*entities.DocumentView, error)
}

type engineImpl struct {
	sRepo *repository.SearchRepository
	qRepo *repository.SQLiteRepository
	graph graph.GraphEngine
}

func NewSearchEngine(qRepo *repository.SQLiteRepository) SearchEngine {
	return &engineImpl{
		qRepo: qRepo,
		sRepo: repository.NewSearchRepository(qRepo),
		graph: graph.NewGraphEngine(qRepo),
	}
}

func (e *engineImpl) Search(ctx context.Context, query string) ([]entities.SearchResult, error) {
	return e.qRepo.Search(ctx, query)
}

func (e *engineImpl) FindEntity(ctx context.Context, name string) (*entities.Entity, error) {
	return e.sRepo.FindEntityByName(ctx, name)
}

func (e *engineImpl) FindDocument(ctx context.Context, name string) (*entities.Document, error) {
	return e.sRepo.FindDocumentByTitle(ctx, name)
}

func (e *engineImpl) Document(ctx context.Context, path string) (*entities.DocumentView, error) {
	return e.sRepo.DocumentByPath(ctx, path)
}

func (e *engineImpl) ResolveDocument(ctx context.Context, title, from string) (*entities.DocumentView, error) {
	return e.sRepo.DocumentByTitle(ctx, title, from)
}

func (e *engineImpl) Backlinks(ctx context.Context, name string) ([]string, error) {
	doc, err := e.FindDocument(ctx, name)
	if err != nil || doc == nil {
		return []string{}, err
	}
	return e.sRepo.GetBacklinks(ctx, doc.ID)
}

func (e *engineImpl) Relations(ctx context.Context, name string) ([]entities.EntityRelation, error) {
	ent, err := e.FindEntity(ctx, name)
	if err != nil || ent == nil {
		return nil, err
	}

	// Return direct outgoing relations
	return e.graph.DirectRelations(ctx, ent.ID)
}

func (e *engineImpl) Impact(ctx context.Context, name string) ([]entities.GraphEdge, error) {
	ent, err := e.FindEntity(ctx, name)
	if err != nil || ent == nil {
		return nil, err
	}

	return e.graph.ImpactAnalysis(ctx, ent.ID)
}

func (e *engineImpl) Graph(ctx context.Context, name string) ([]entities.GraphEdge, error) {
	ent, err := e.FindEntity(ctx, name)
	if err != nil || ent == nil {
		return nil, err
	}

	return e.graph.DependencyTree(ctx, ent.ID)
}

package search

import (
	"context"
	"github.com/md-indexer/internal/entities"
	"github.com/md-indexer/internal/repository"
)

type SearchEngine interface {
	Search(ctx context.Context, query string) ([]entities.SearchResult, error)
	Backlinks(ctx context.Context, name string) ([]string, error)
	Relations(ctx context.Context, name string) ([]entities.EntityRelation, error)
	Impact(ctx context.Context, name string) ([]string, error)
	Graph(ctx context.Context, name string) (interface{}, error)
	FindEntity(ctx context.Context, name string) (*entities.Entity, error)
	FindDocument(ctx context.Context, name string) (*entities.Document, error)
}

type engineImpl struct {
	sRepo *repository.SearchRepository
	qRepo *repository.SQLiteRepository
}

func NewSearchEngine(qRepo *repository.SQLiteRepository) SearchEngine {
	return &engineImpl{
		qRepo: qRepo,
		sRepo: repository.NewSearchRepository(qRepo),
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

func (e *engineImpl) Backlinks(ctx context.Context, name string) ([]string, error) {
	doc, err := e.FindDocument(ctx, name)
	if err != nil || doc == nil { return []string{}, err }
	return e.sRepo.GetBacklinks(ctx, doc.ID)
}

func (e *engineImpl) Relations(ctx context.Context, name string) ([]entities.EntityRelation, error) {
	return []entities.EntityRelation{}, nil
}

func (e *engineImpl) Impact(ctx context.Context, name string) ([]string, error) {
	return []string{}, nil
}

func (e *engineImpl) Graph(ctx context.Context, name string) (interface{}, error) {
	return nil, nil
}

package graph

import (
	"context"
	"github.com/md-indexer/internal/entities"
	"github.com/md-indexer/internal/repository"
)

type GraphEngine interface {
	DirectRelations(ctx context.Context, entityID string) ([]entities.EntityRelation, error)
	ReverseRelations(ctx context.Context, entityID string) ([]entities.EntityRelation, error)
	DependencyTree(ctx context.Context, entityID string) (interface{}, error)
	ImpactAnalysis(ctx context.Context, entityID string) (interface{}, error)
	ShortestPath(ctx context.Context, sourceID, targetID string) ([]string, error)
}

type graphImpl struct {
	repo *repository.SQLiteRepository
}

func NewGraphEngine(repo *repository.SQLiteRepository) GraphEngine {
	return &graphImpl{repo: repo}
}

func (g *graphImpl) DirectRelations(ctx context.Context, entityID string) ([]entities.EntityRelation, error) {
	return nil, nil
}

func (g *graphImpl) ReverseRelations(ctx context.Context, entityID string) ([]entities.EntityRelation, error) {
	return nil, nil
}

func (g *graphImpl) DependencyTree(ctx context.Context, entityID string) (interface{}, error) {
	return nil, nil
}

func (g *graphImpl) ImpactAnalysis(ctx context.Context, entityID string) (interface{}, error) {
	return nil, nil
}

func (g *graphImpl) ShortestPath(ctx context.Context, sourceID, targetID string) ([]string, error) {
	return nil, nil
}

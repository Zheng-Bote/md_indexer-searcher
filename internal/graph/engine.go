package graph

import (
	"context"
	"database/sql"
	"strings"

	"github.com/md-indexer/internal/entities"
	"github.com/md-indexer/internal/repository"
)

type GraphEngine interface {
	DirectRelations(ctx context.Context, entityID string) ([]entities.EntityRelation, error)
	ReverseRelations(ctx context.Context, entityID string) ([]entities.EntityRelation, error)
	DependencyTree(ctx context.Context, entityID string) ([]entities.GraphEdge, error)
	ImpactAnalysis(ctx context.Context, entityID string) ([]entities.GraphEdge, error)
	ShortestPath(ctx context.Context, sourceID, targetID string) ([]string, error)
}

type graphImpl struct {
	repo *repository.SQLiteRepository
}

func NewGraphEngine(repo *repository.SQLiteRepository) GraphEngine {
	return &graphImpl{repo: repo}
}

func (g *graphImpl) DirectRelations(ctx context.Context, entityID string) ([]entities.EntityRelation, error) {
	db := g.repo.GetDB()
	rows, err := db.QueryContext(ctx, "SELECT source_entity_id, target_entity_id, relation_type FROM entity_relations WHERE source_entity_id = ?", entityID)
	if err != nil { return nil, err }
	defer rows.Close()

	var rels []entities.EntityRelation
	for rows.Next() {
		var r entities.EntityRelation
		if err := rows.Scan(&r.SourceID, &r.TargetID, &r.Type); err == nil {
			rels = append(rels, r)
		}
	}
	return rels, nil
}

func (g *graphImpl) ReverseRelations(ctx context.Context, entityID string) ([]entities.EntityRelation, error) {
	db := g.repo.GetDB()
	rows, err := db.QueryContext(ctx, "SELECT source_entity_id, target_entity_id, relation_type FROM entity_relations WHERE target_entity_id = ?", entityID)
	if err != nil { return nil, err }
	defer rows.Close()

	var rels []entities.EntityRelation
	for rows.Next() {
		var r entities.EntityRelation
		if err := rows.Scan(&r.SourceID, &r.TargetID, &r.Type); err == nil {
			rels = append(rels, r)
		}
	}
	return rels, nil
}

func (g *graphImpl) DependencyTree(ctx context.Context, entityID string) ([]entities.GraphEdge, error) {
	db := g.repo.GetDB()
	query := `
		WITH RECURSIVE traverse(source, target, rel, depth) AS (
			SELECT source_entity_id, target_entity_id, relation_type, 1
			FROM entity_relations
			WHERE source_entity_id = ?
			UNION ALL
			SELECT er.source_entity_id, er.target_entity_id, er.relation_type, t.depth + 1
			FROM entity_relations er
			JOIN traverse t ON er.source_entity_id = t.target
			WHERE t.depth < 10
		)
		SELECT source, target, rel FROM traverse;
	`
	rows, err := db.QueryContext(ctx, query, entityID)
	if err != nil { return nil, err }
	defer rows.Close()

	var edges []entities.GraphEdge
	for rows.Next() {
		var e entities.GraphEdge
		if err := rows.Scan(&e.Source, &e.Target, &e.Relation); err == nil {
			edges = append(edges, e)
		}
	}
	return edges, nil
}

func (g *graphImpl) ImpactAnalysis(ctx context.Context, entityID string) ([]entities.GraphEdge, error) {
	db := g.repo.GetDB()
	query := `
		WITH RECURSIVE traverse(source, target, rel, depth) AS (
			SELECT source_entity_id, target_entity_id, relation_type, 1
			FROM entity_relations
			WHERE target_entity_id = ?
			UNION ALL
			SELECT er.source_entity_id, er.target_entity_id, er.relation_type, t.depth + 1
			FROM entity_relations er
			JOIN traverse t ON er.target_entity_id = t.source
			WHERE t.depth < 10
		)
		SELECT source, target, rel FROM traverse;
	`
	rows, err := db.QueryContext(ctx, query, entityID)
	if err != nil { return nil, err }
	defer rows.Close()

	var edges []entities.GraphEdge
	for rows.Next() {
		var e entities.GraphEdge
		if err := rows.Scan(&e.Source, &e.Target, &e.Relation); err == nil {
			edges = append(edges, e)
		}
	}
	return edges, nil
}

func (g *graphImpl) ShortestPath(ctx context.Context, sourceID, targetID string) ([]string, error) {
	db := g.repo.GetDB()
	query := `
		WITH RECURSIVE
		  bfs(node, path, depth) AS (
		    SELECT ?, ?, 0
		    UNION ALL
		    SELECT er.target_entity_id, bfs.path || ',' || er.target_entity_id, bfs.depth + 1
		    FROM entity_relations er
		    JOIN bfs ON er.source_entity_id = bfs.node
		    WHERE bfs.node != ? AND bfs.depth < 10
		  )
		SELECT path FROM bfs WHERE node = ? ORDER BY depth ASC LIMIT 1;
	`
	var pathStr string
	err := db.QueryRowContext(ctx, query, sourceID, sourceID, targetID, targetID).Scan(&pathStr)
	if err != nil {
		if err == sql.ErrNoRows { return nil, nil }
		return nil, err
	}
	return strings.Split(pathStr, ","), nil
}

// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

package hybrid

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/md-indexer/internal/ai/model"
)

// Relation weights: how strongly a graph edge makes the neighbour relevant.
// Relation types are the upper-cased frontmatter list keys (uses: [...] -> USES).
const (
	WeightUses      = 1.0
	WeightDependsOn = 0.9
	WeightADR       = 0.8
	WeightContains  = 0.5
	WeightLinksTo   = 0.4
	WeightRelation  = 0.3
	WeightSharedTag = 0.3
	WeightReference = 0.2
	// WeightOther applies to custom frontmatter relation lists not listed above.
	WeightOther = 0.3
)

var relationWeights = map[string]float64{
	"USES":       WeightUses,
	"DEPENDS_ON": WeightDependsOn,
	"ADR":        WeightADR,
	"CONTAINS":   WeightContains,
	"LINKS_TO":   WeightLinksTo,
	"RELATION":   WeightRelation,
	"REFERENCES": WeightReference,
	"REFERENCE":  WeightReference,
}

func relationWeight(relation string) float64 {
	if w, ok := relationWeights[relation]; ok {
		return w
	}
	return WeightOther
}

const (
	// beamWidth bounds the nodes expanded per hop so dense neighbourhoods cannot explode the walk.
	beamWidth = 50
	// maxTagDocuments drops tags shared by more documents; such tags are too generic to relate notes.
	maxTagDocuments = 10
	// maxGraphSectionsPerDocument limits how many sections of each reached document become candidates.
	maxGraphSectionsPerDocument = 2
)

// GraphResult holds the sections of reached documents and the reasoning chain that leads to each document.
type GraphResult struct {
	Candidates []Candidate
	Chains     map[string]model.ReasoningChain // keyed by document ID
}

// GraphRetriever expands seed documents to their weighted graph neighbourhood.
type GraphRetriever interface {
	Expand(ctx context.Context, seedDocIDs []string, depth, limit int) (*GraphResult, error)
}

// SQLiteGraph walks wikilinks, entity relations (both directions) and shared tags.
type SQLiteGraph struct {
	db *sql.DB
}

// NewSQLiteGraph creates a graph retriever.
func NewSQLiteGraph(db *sql.DB) *SQLiteGraph {
	return &SQLiteGraph{db: db}
}

// edge is a traversable connection; source/target keep the semantic direction of the relation.
type edge struct {
	source, target string
	relation       string
	weight         float64
}

type graphNode struct {
	score  float64
	parent string
	via    edge
}

// Expand runs a beam-limited weighted BFS from the seeds. A node's GraphScore is the best product of edge
// weights over all paths of at most depth hops; its reasoning chain is that best path.
func (g *SQLiteGraph) Expand(ctx context.Context, seedDocIDs []string, depth, limit int) (*GraphResult, error) {
	res := &GraphResult{Chains: make(map[string]model.ReasoningChain)}
	if len(seedDocIDs) == 0 || depth < 1 {
		return res, nil
	}

	nodes := make(map[string]*graphNode)
	seeds := make(map[string]bool)
	for _, id := range seedDocIDs {
		nodes[id] = &graphNode{score: 1}
		seeds[id] = true
	}

	frontier := seedDocIDs
	for hop := 0; hop < depth && len(frontier) > 0; hop++ {
		edges, err := g.neighbours(ctx, frontier)
		if err != nil {
			return nil, err
		}
		inFrontier := make(map[string]bool, len(frontier))
		for _, id := range frontier {
			inFrontier[id] = true
		}
		next := make(map[string]bool)
		for _, e := range edges {
			// Edges are undirected for traversal; the stored direction is kept for the chain.
			for _, step := range [][2]string{{e.source, e.target}, {e.target, e.source}} {
				from, to := step[0], step[1]
				if !inFrontier[from] || seeds[to] {
					continue
				}
				score := nodes[from].score * e.weight
				if n, ok := nodes[to]; !ok || score > n.score {
					nodes[to] = &graphNode{score: score, parent: from, via: e}
					next[to] = true
				}
			}
		}
		frontier = topNodes(next, nodes, beamWidth)
	}

	reached := make([]string, 0, len(nodes))
	for id := range nodes {
		if !seeds[id] {
			reached = append(reached, id)
		}
	}
	sort.Slice(reached, func(i, j int) bool {
		if nodes[reached[i]].score != nodes[reached[j]].score {
			return nodes[reached[i]].score > nodes[reached[j]].score
		}
		return reached[i] < reached[j]
	})
	if len(reached) > limit {
		reached = reached[:limit]
	}
	if len(reached) == 0 {
		return res, nil
	}

	cands, err := g.sections(ctx, reached, nodes)
	if err != nil {
		return nil, err
	}
	res.Candidates = cands

	titles, err := g.titles(ctx, nodes)
	if err != nil {
		return nil, err
	}
	for _, id := range reached {
		res.Chains[id] = chainTo(id, nodes, titles)
	}
	return res, nil
}

// topNodes returns up to n of the given node IDs with the highest scores.
func topNodes(ids map[string]bool, nodes map[string]*graphNode, n int) []string {
	out := make([]string, 0, len(ids))
	for id := range ids {
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool {
		if nodes[out[i]].score != nodes[out[j]].score {
			return nodes[out[i]].score > nodes[out[j]].score
		}
		return out[i] < out[j]
	})
	if len(out) > n {
		out = out[:n]
	}
	return out
}

// chainTo walks parent pointers back to the seed and returns the steps in seed-to-node order.
func chainTo(id string, nodes map[string]*graphNode, titles map[string]string) model.ReasoningChain {
	var steps []model.ReasoningStep
	for n := nodes[id]; n != nil && n.parent != ""; n = nodes[n.parent] {
		steps = append(steps, model.ReasoningStep{
			Source:   titles[n.via.source],
			Relation: n.via.relation,
			Target:   titles[n.via.target],
		})
	}
	for i, j := 0, len(steps)-1; i < j; i, j = i+1, j-1 {
		steps[i], steps[j] = steps[j], steps[i]
	}
	return model.ReasoningChain{Steps: steps}
}

func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

func idArgs(ids []string, times int) []interface{} {
	args := make([]interface{}, 0, len(ids)*times)
	for t := 0; t < times; t++ {
		for _, id := range ids {
			args = append(args, id)
		}
	}
	return args
}

// neighbours returns all edges touching the given documents.
// links.target_document_id and entity IDs hold names; they resolve to documents via the title.
func (g *SQLiteGraph) neighbours(ctx context.Context, ids []string) ([]edge, error) {
	ph := placeholders(len(ids))
	var edges []edge

	linkRows, err := g.db.QueryContext(ctx, fmt.Sprintf(`
		SELECT l.source_document_id, d.id
		FROM links l
		JOIN documents d ON d.title = l.target_document_id
		JOIN documents s ON s.id = l.source_document_id
		WHERE l.source_document_id IN (%[1]s) OR d.id IN (%[1]s)`, ph), idArgs(ids, 2)...)
	if err != nil {
		return nil, fmt.Errorf("graph links: %w", err)
	}
	for linkRows.Next() {
		e := edge{relation: "LINKS_TO", weight: WeightLinksTo}
		if err := linkRows.Scan(&e.source, &e.target); err != nil {
			linkRows.Close()
			return nil, fmt.Errorf("graph links scan: %w", err)
		}
		edges = append(edges, e)
	}
	linkRows.Close()

	// TAGS are handled as shared-tag edges below; ATTACHMENTS point to files, not notes.
	relRows, err := g.db.QueryContext(ctx, fmt.Sprintf(`
		SELECT ds.id, dt.id, r.relation_type
		FROM entity_relations r
		JOIN documents ds ON ds.title = r.source_entity_id
		JOIN documents dt ON dt.title = r.target_entity_id
		WHERE r.relation_type NOT IN ('TAGS', 'ATTACHMENTS') AND (ds.id IN (%[1]s) OR dt.id IN (%[1]s))`, ph), idArgs(ids, 2)...)
	if err != nil {
		return nil, fmt.Errorf("graph relations: %w", err)
	}
	for relRows.Next() {
		var e edge
		if err := relRows.Scan(&e.source, &e.target, &e.relation); err != nil {
			relRows.Close()
			return nil, fmt.Errorf("graph relations scan: %w", err)
		}
		e.weight = relationWeight(e.relation)
		edges = append(edges, e)
	}
	relRows.Close()

	tagRows, err := g.db.QueryContext(ctx, fmt.Sprintf(`
		SELECT a.document_id, b.document_id, a.tag_id
		FROM document_tags a
		JOIN document_tags b ON b.tag_id = a.tag_id AND b.document_id != a.document_id
		JOIN documents d ON d.id = b.document_id
		WHERE a.document_id IN (%s)
			AND a.tag_id IN (SELECT tag_id FROM document_tags GROUP BY tag_id HAVING count(*) <= ?)`, ph),
		append(idArgs(ids, 1), maxTagDocuments)...)
	if err != nil {
		return nil, fmt.Errorf("graph tags: %w", err)
	}
	for tagRows.Next() {
		var e edge
		var tag string
		if err := tagRows.Scan(&e.source, &e.target, &tag); err != nil {
			tagRows.Close()
			return nil, fmt.Errorf("graph tags scan: %w", err)
		}
		if isNumeric(tag) {
			continue
		}
		e.relation = "SHARES_TAG #" + tag
		e.weight = WeightSharedTag
		edges = append(edges, e)
	}
	tagRows.Close()
	return edges, nil
}

// isNumeric reports tags like "1" that stem from headings or code, not from tagging.
func isNumeric(tag string) bool {
	for _, r := range tag {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

// sections returns up to maxGraphSectionsPerDocument non-empty sections per document, in document order.
func (g *SQLiteGraph) sections(ctx context.Context, docIDs []string, nodes map[string]*graphNode) ([]Candidate, error) {
	rows, err := g.db.QueryContext(ctx, fmt.Sprintf(`
		SELECT s.id, s.document_id, d.path, d.title, s.heading, s.content
		FROM sections s
		JOIN documents d ON d.id = s.document_id
		WHERE s.document_id IN (%s) AND s.content != ''
		ORDER BY s.rowid`, placeholders(len(docIDs))), idArgs(docIDs, 1)...)
	if err != nil {
		return nil, fmt.Errorf("graph sections: %w", err)
	}
	defer rows.Close()

	byDoc := make(map[string][]Candidate)
	for rows.Next() {
		c := Candidate{SourceType: SourceGraph}
		if err := rows.Scan(&c.SectionID, &c.DocumentID, &c.Path, &c.Title, &c.Heading, &c.Content); err != nil {
			return nil, fmt.Errorf("graph sections scan: %w", err)
		}
		if len(byDoc[c.DocumentID]) < maxGraphSectionsPerDocument {
			c.GraphScore = nodes[c.DocumentID].score
			byDoc[c.DocumentID] = append(byDoc[c.DocumentID], c)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("graph sections: %w", err)
	}

	var out []Candidate
	for _, id := range docIDs {
		out = append(out, byDoc[id]...)
	}
	return out, nil
}

// titles resolves document IDs of all visited nodes and their chain edges to titles.
func (g *SQLiteGraph) titles(ctx context.Context, nodes map[string]*graphNode) (map[string]string, error) {
	ids := make([]string, 0, len(nodes))
	for id := range nodes {
		ids = append(ids, id)
	}
	rows, err := g.db.QueryContext(ctx, fmt.Sprintf("SELECT id, title FROM documents WHERE id IN (%s)", placeholders(len(ids))), idArgs(ids, 1)...)
	if err != nil {
		return nil, fmt.Errorf("graph titles: %w", err)
	}
	defer rows.Close()

	titles := make(map[string]string, len(ids))
	for rows.Next() {
		var id, title string
		if err := rows.Scan(&id, &title); err != nil {
			return nil, fmt.Errorf("graph titles scan: %w", err)
		}
		titles[id] = title
	}
	return titles, rows.Err()
}

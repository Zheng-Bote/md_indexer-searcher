// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

// Package hybrid implements the planned, multi-source retriever with rank fusion for md-ai.
package hybrid

import (
	"context"
	"database/sql"
	"regexp"

	"github.com/md-indexer/internal/ai/model"
)

const (
	// maxGraphSeeds limits how many top documents start the graph expansion.
	maxGraphSeeds = 5
	// maxChains limits the reasoning chains passed to the context builder.
	maxChains = 5
)

// Retriever plans retrieval per query, collects candidates from all planned sources and fuses them.
type Retriever struct {
	planner    *Planner
	fts        FTSRetriever
	entities   EntityRetriever
	graph      GraphRetriever
	vectors    VectorRetriever
	weights    Weights
	topK       int
	topKVector int
}

// NewRetriever wires a Retriever from its components. vectors may be nil to disable vector retrieval.
// topK limits the FTS and graph candidate lists, topKVector the vector list.
func NewRetriever(planner *Planner, fts FTSRetriever, entities EntityRetriever, graph GraphRetriever, vectors VectorRetriever, weights Weights, topK, topKVector int) *Retriever {
	if topK < 1 {
		topK = 25
	}
	if topKVector < 1 {
		topKVector = 25
	}
	return &Retriever{planner: planner, fts: fts, entities: entities, graph: graph, vectors: vectors, weights: weights, topK: topK, topKVector: topKVector}
}

// NewSQLiteRetriever creates a Retriever backed by knowledge.db; vectors may be nil.
func NewSQLiteRetriever(db *sql.DB, maxGraphDepth int, vectors VectorRetriever, weights Weights, topK, topKVector int) *Retriever {
	return NewRetriever(NewPlanner(maxGraphDepth), NewSQLiteFTS(db), NewSQLiteEntities(db), NewSQLiteGraph(db), vectors, weights, topK, topKVector)
}

// Retrieve returns fused evidence for the query in the md-ai retrieval model.
func (r *Retriever) Retrieve(ctx context.Context, query string, intent model.Intent) (*model.RetrievalResult, error) {
	res := &model.RetrievalResult{
		Query:     query,
		Intent:    intent,
		Snippets:  make([]model.DocumentSnippet, 0),
		Entities:  make([]model.EntityMatch, 0),
		Relations: make([]model.EntityRelationMatch, 0),
	}
	terms := Terms(query)
	if len(terms) == 0 {
		return res, nil
	}

	plan := r.planner.Plan(query, intent)
	lists := make(map[string][]Candidate)

	if plan.UseFTS {
		cands, err := r.fts.Search(ctx, terms, r.topK)
		if err != nil {
			return nil, err
		}
		lists[SourceFTS] = cands
	}

	if plan.UseVectors && r.vectors != nil {
		cands, err := r.vectors.Search(ctx, query, r.topKVector)
		if err != nil {
			return nil, err
		}
		lists[SourceVector] = cands
	}

	if plan.UseEntities {
		er, err := r.entities.Find(ctx, query)
		if err != nil {
			return nil, err
		}
		lists[SourceEntity] = er.Candidates
		res.Entities = append(res.Entities, er.Entities...)
		res.Relations = append(res.Relations, er.Relations...)
	}

	var chains map[string]model.ReasoningChain
	if plan.UseGraph && plan.GraphDepth > 0 {
		seeds := topDocuments(maxGraphSeeds, lists[SourceEntity], lists[SourceFTS], lists[SourceVector])
		gr, err := r.graph.Expand(ctx, seeds, plan.GraphDepth, r.topK)
		if err != nil {
			return nil, err
		}
		lists[SourceGraph] = gr.Candidates
		chains = gr.Chains
	}

	fused := Fuse(lists, r.weights, plan.MaxResults)
	res.Chains = selectChains(fused, chains, maxChains)
	for _, c := range fused {
		res.Snippets = append(res.Snippets, model.DocumentSnippet{
			DocumentPath: c.Path,
			Title:        c.Title,
			SectionTitle: c.Heading,
			Content:      c.Content,
			Score:        c.FinalScore,
			Strategy:     strategy(c),
		})
	}
	return res, nil
}

// selectChains returns the reasoning chains of fused documents in fused order, one per document.
// A chain that is a prefix of another selected chain adds no information and is dropped.
func selectChains(fused []Candidate, chains map[string]model.ReasoningChain, n int) []model.ReasoningChain {
	var all []model.ReasoningChain
	seen := make(map[string]bool)
	for _, c := range fused {
		chain, ok := chains[c.DocumentID]
		if !ok || len(chain.Steps) == 0 || seen[c.DocumentID] {
			continue
		}
		seen[c.DocumentID] = true
		all = append(all, chain)
	}

	var out []model.ReasoningChain
	for i, c := range all {
		if len(out) >= n {
			break
		}
		redundant := false
		for j, other := range all {
			if i != j && isPrefix(c.Steps, other.Steps) && (len(c.Steps) < len(other.Steps) || j < i) {
				redundant = true
				break
			}
		}
		if !redundant {
			out = append(out, c)
		}
	}
	return out
}

func isPrefix(prefix, steps []model.ReasoningStep) bool {
	if len(prefix) > len(steps) {
		return false
	}
	for i := range prefix {
		if prefix[i] != steps[i] {
			return false
		}
	}
	return true
}

// topDocuments returns up to n distinct document IDs in list order.
func topDocuments(n int, lists ...[]Candidate) []string {
	seen := make(map[string]bool)
	var ids []string
	for _, list := range lists {
		for _, c := range list {
			if len(ids) >= n {
				return ids
			}
			if c.DocumentID != "" && !seen[c.DocumentID] {
				seen[c.DocumentID] = true
				ids = append(ids, c.DocumentID)
			}
		}
	}
	return ids
}

// adrPattern matches "ADR" as a word (ADR-003, docs/adr/...), not inside words like "Badradio".
var adrPattern = regexp.MustCompile(`(?i)(^|[^\pL\pN])adr([^\pL]|$)`)

// strategy keeps the classic retriever's "adr" marker so the context builder can prioritise decision records.
func strategy(c Candidate) string {
	if adrPattern.MatchString(c.Path) || adrPattern.MatchString(c.Title) {
		return "adr"
	}
	return c.SourceType
}

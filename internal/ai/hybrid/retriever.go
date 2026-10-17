// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

// Package hybrid implements the planned, multi-source retriever with rank fusion for md-ai.
package hybrid

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"

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
	plan := r.planner.Plan(query, intent)
	res := &model.RetrievalResult{
		Query:  query,
		Intent: intent,
		Plan: &model.RetrievalPlanInfo{
			QueryType:   string(plan.QueryType),
			UseFTS:      plan.UseFTS,
			UseEntities: plan.UseEntities,
			UseGraph:    plan.UseGraph,
			UseVectors:  plan.UseVectors,
			GraphDepth:  plan.GraphDepth,
			MaxResults:  plan.MaxResults,
		},
		Snippets:  make([]model.DocumentSnippet, 0),
		Entities:  make([]model.EntityMatch, 0),
		Relations: make([]model.EntityRelationMatch, 0),
	}
	terms := Terms(query)
	if len(terms) == 0 {
		return res, nil
	}

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
		res.Gaps = append(res.Gaps, er.Gaps...)
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

	fused := Fuse(lists, r.weights.ForPlan(plan), plan.MaxResults)
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
	if plan.QueryType == QueryArchitecture {
		res.Architecture = buildArchitectureContext(fused, res.Entities, res.Relations, res.Chains)
	}
	return res, nil
}

// buildArchitectureContext groups discovered entities, documents, ADRs and dependencies into a structured ArchitectureContext.
func buildArchitectureContext(fused []Candidate, entities []model.EntityMatch, relations []model.EntityRelationMatch, chains []model.ReasoningChain) *model.ArchitectureContext {
	arch := &model.ArchitectureContext{}
	seenSvc := make(map[string]bool)
	seenComp := make(map[string]bool)
	seenPipe := make(map[string]bool)
	seenADR := make(map[string]bool)
	seenEnt := make(map[string]bool)
	seenDep := make(map[string]bool)

	classify := func(name, typ, path string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		lt := strings.ToLower(typ)
		ln := strings.ToLower(name)
		lp := strings.ToLower(path)

		switch {
		case adrPattern.MatchString(name) || adrPattern.MatchString(path) || lt == "adr":
			if !seenADR[name] {
				seenADR[name] = true
				arch.ADRs = append(arch.ADRs, name)
			}
		case lt == "pipeline" || strings.Contains(ln, "pipeline") || strings.Contains(lp, "/pipeline"):
			if !seenPipe[name] {
				seenPipe[name] = true
				arch.Pipelines = append(arch.Pipelines, name)
			}
		case lt == "service" || strings.HasSuffix(lt, "-service") || strings.Contains(ln, "service") || strings.Contains(lp, "/service"):
			if !seenSvc[name] {
				seenSvc[name] = true
				arch.Services = append(arch.Services, name)
			}
		case lt == "component" || strings.Contains(ln, "component") || strings.Contains(ln, "engine") || strings.Contains(ln, "delivery") || strings.Contains(ln, "gateway") || strings.Contains(lp, "/component"):
			if !seenComp[name] {
				seenComp[name] = true
				arch.Components = append(arch.Components, name)
			}
		default:
			if !seenEnt[name] {
				seenEnt[name] = true
				arch.Entities = append(arch.Entities, name)
			}
		}
	}

	for _, e := range entities {
		classify(e.Name, e.Type, e.DocumentURI)
	}
	for _, c := range fused {
		classify(c.Title, "", c.Path)
	}
	for _, r := range relations {
		key := fmt.Sprintf("%s->%s:%s", r.SourceEntity, r.TargetEntity, r.RelationType)
		if !seenDep[key] {
			seenDep[key] = true
			arch.Dependencies = append(arch.Dependencies, r)
		}
	}
	for _, ch := range chains {
		for _, st := range ch.Steps {
			classify(st.Source, "", "")
			classify(st.Target, "", "")
			key := fmt.Sprintf("%s->%s:%s", st.Source, st.Target, st.Relation)
			if !seenDep[key] {
				seenDep[key] = true
				arch.Dependencies = append(arch.Dependencies, model.EntityRelationMatch{
					SourceEntity: st.Source,
					RelationType: st.Relation,
					TargetEntity: st.Target,
				})
			}
		}
	}

	if len(arch.Services) == 0 && len(arch.Components) == 0 && len(arch.Pipelines) == 0 && len(arch.ADRs) == 0 && len(arch.Entities) == 0 && len(arch.Dependencies) == 0 {
		return nil
	}
	return arch
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

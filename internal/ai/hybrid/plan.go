// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

package hybrid

import (
	"regexp"

	"github.com/md-indexer/internal/ai/model"
)

// QueryType selects the retrieval strategy mix for a question.
type QueryType string

const (
	QueryFact         QueryType = "fact"
	QueryEntity       QueryType = "entity"
	QueryImpact       QueryType = "impact"
	QueryArchitecture QueryType = "architecture"
	QueryExploration  QueryType = "exploration"
)

// RetrievalPlan describes which retrieval sources run for a query and how far they reach.
type RetrievalPlan struct {
	QueryType QueryType

	UseFTS      bool
	UseEntities bool
	UseGraph    bool
	UseVectors  bool

	GraphDepth int

	MaxResults int
}

var architecturePattern = regexp.MustCompile(`(?i)(\bhow (does|do|is)\b.*\bwork|\bwie (funktionier|arbeitet|läuft)|\barchite[ck]tur|\bdata ?flow\b|\bdatenfluss\b)`)

// Planner turns a classified query into a RetrievalPlan.
type Planner struct {
	maxGraphDepth int
}

// NewPlanner creates a Planner; graph depth of every plan is capped at maxGraphDepth.
func NewPlanner(maxGraphDepth int) *Planner {
	if maxGraphDepth < 1 {
		maxGraphDepth = 1
	}
	return &Planner{maxGraphDepth: maxGraphDepth}
}

// Plan maps the query and its intent to a retrieval plan.
// FTS runs for every type because it seeds entity and graph retrieval.
func (p *Planner) Plan(query string, intent model.Intent) RetrievalPlan {
	var plan RetrievalPlan
	switch p.queryType(query, intent) {
	case QueryFact:
		plan = RetrievalPlan{QueryType: QueryFact, UseFTS: true, UseVectors: true, MaxResults: 8}
	case QueryEntity:
		plan = RetrievalPlan{QueryType: QueryEntity, UseFTS: true, UseEntities: true, UseVectors: true, MaxResults: 10}
	case QueryImpact:
		plan = RetrievalPlan{QueryType: QueryImpact, UseFTS: true, UseEntities: true, UseGraph: true, GraphDepth: 3, MaxResults: 15}
	case QueryArchitecture:
		plan = RetrievalPlan{QueryType: QueryArchitecture, UseFTS: true, UseEntities: true, UseGraph: true, UseVectors: true, GraphDepth: 4, MaxResults: 15}
	default:
		plan = RetrievalPlan{QueryType: QueryExploration, UseFTS: true, UseEntities: true, UseGraph: true, UseVectors: true, GraphDepth: 1, MaxResults: 12}
	}
	if plan.GraphDepth > p.maxGraphDepth {
		plan.GraphDepth = p.maxGraphDepth
	}
	return plan
}

func (p *Planner) queryType(query string, intent model.Intent) QueryType {
	if architecturePattern.MatchString(query) {
		return QueryArchitecture
	}
	switch intent {
	case model.IntentADR:
		return QueryFact
	case model.IntentEntity:
		return QueryEntity
	case model.IntentImpact, model.IntentRelationship:
		return QueryImpact
	default:
		return QueryExploration
	}
}

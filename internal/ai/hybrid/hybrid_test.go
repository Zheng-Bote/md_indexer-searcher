// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

package hybrid

import (
	"reflect"
	"testing"

	"github.com/md-indexer/internal/ai/model"
)

func TestTerms(t *testing.T) {
	tests := []struct {
		query string
		want  []string
	}{
		{"Wie konfiguriere ich Ollama?", []string{"konfiguriere", "ollama"}},
		{"How does the ingestion pipeline work?", []string{"ingestion", "pipeline"}},
		{"Was ist ADR-003?", []string{"adr", "003"}},
		{"Verschlüsselung und Verschlüsselung", []string{"verschlüsselung"}},
		{"wie ich die", nil},
	}
	for _, tt := range tests {
		if got := Terms(tt.query); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("Terms(%q) = %v, want %v", tt.query, got, tt.want)
		}
	}
}

func TestFTSMatch_QuotesTerms(t *testing.T) {
	got := ftsMatch([]string{"near", "not", "aes"})
	want := `"near" OR "not" OR "aes"`
	if got != want {
		t.Errorf("ftsMatch() = %q, want %q", got, want)
	}
}

func TestPlanner_Plan(t *testing.T) {
	p := NewPlanner(3)
	tests := []struct {
		query       string
		intent      model.Intent
		wantType    QueryType
		wantGraph   bool
		wantDepth   int
		wantVectors bool
	}{
		{"Was ist ADR-003?", model.IntentADR, QueryFact, false, 0, true},
		{"Was ist der Encryption Service?", model.IntentEntity, QueryEntity, false, 0, true},
		{"Welche Komponenten verwenden Encryption?", model.IntentImpact, QueryImpact, true, 3, false},
		// Architecture depth 4 is capped by maxGraphDepth 3.
		{"Wie funktioniert die Ingestion Pipeline?", model.IntentSearch, QueryArchitecture, true, 3, true},
		{"How does the indexer work?", model.IntentSearch, QueryArchitecture, true, 3, true},
		{"raspberry pi sensor", model.IntentSearch, QueryExploration, true, 1, true},
	}
	for _, tt := range tests {
		plan := p.Plan(tt.query, tt.intent)
		if plan.QueryType != tt.wantType {
			t.Errorf("%q: type = %s, want %s", tt.query, plan.QueryType, tt.wantType)
		}
		if !plan.UseFTS {
			t.Errorf("%q: FTS must always be enabled", tt.query)
		}
		if plan.UseVectors != tt.wantVectors {
			t.Errorf("%q: vectors = %v, want %v", tt.query, plan.UseVectors, tt.wantVectors)
		}
		if plan.UseGraph != tt.wantGraph || plan.GraphDepth != tt.wantDepth {
			t.Errorf("%q: graph = %v/%d, want %v/%d", tt.query, plan.UseGraph, plan.GraphDepth, tt.wantGraph, tt.wantDepth)
		}
	}
}

func TestFuse(t *testing.T) {
	w := Weights{FTS: 0.35, Graph: 0.20, Entity: 0.10}
	lists := map[string][]Candidate{
		SourceFTS: {
			{SectionID: "a1", DocumentID: "a", FTSScore: 9},
			{SectionID: "b1", DocumentID: "b", FTSScore: 5},
		},
		SourceEntity: {
			{SectionID: "b1", DocumentID: "b", EntityScore: 1},
		},
		SourceGraph: {
			{SectionID: "c1", DocumentID: "c", GraphScore: 1},
		},
	}

	got := Fuse(lists, w, 10)
	if len(got) != 3 {
		t.Fatalf("expected 3 fused candidates, got %d", len(got))
	}
	// b1 is found by FTS (rank 2) and entity (rank 1): 0.35/62 + 0.10/61 > 0.35/61.
	if got[0].SectionID != "b1" {
		t.Errorf("expected b1 first, got %s", got[0].SectionID)
	}
	if got[0].FTSScore != 5 || got[0].EntityScore != 1 {
		t.Errorf("expected merged per-source scores, got fts=%v entity=%v", got[0].FTSScore, got[0].EntityScore)
	}
	if got[0].SourceType != SourceFTS {
		t.Errorf("expected strongest source fts for b1, got %s", got[0].SourceType)
	}
	if got[2].SectionID != "c1" || got[2].SourceType != SourceGraph {
		t.Errorf("expected graph candidate c1 last, got %s/%s", got[2].SectionID, got[2].SourceType)
	}
}

func TestFuse_LimitsSectionsPerDocument(t *testing.T) {
	var fts []Candidate
	for _, id := range []string{"a1", "a2", "a3", "a4", "b1"} {
		fts = append(fts, Candidate{SectionID: id, DocumentID: id[:1]})
	}
	got := Fuse(map[string][]Candidate{SourceFTS: fts}, Weights{FTS: 1}, 10)

	var ids []string
	for _, c := range got {
		ids = append(ids, c.SectionID)
	}
	want := []string{"a1", "a2", "a3", "b1"}
	if !reflect.DeepEqual(ids, want) {
		t.Errorf("expected %v, got %v", want, ids)
	}

	if got := Fuse(map[string][]Candidate{SourceFTS: fts}, Weights{FTS: 1}, 2); len(got) != 2 {
		t.Errorf("expected maxResults 2, got %d", len(got))
	}
}

func TestStrategy_ADRWordOnly(t *testing.T) {
	tests := map[string]string{
		"docs/adr/001-sqlite.md":      "adr",
		"vault/ADR-003 Encryption.md": "adr",
		"vault/ADR003.md":             "adr",
		"vault/Badradio.md":           SourceFTS,
		"vault/Adresse ändern.md":     SourceFTS,
	}
	for path, want := range tests {
		if got := strategy(Candidate{Path: path, SourceType: SourceFTS}); got != want {
			t.Errorf("strategy(%q) = %s, want %s", path, got, want)
		}
	}
}

func TestSelectChains_DropsPrefixes(t *testing.T) {
	ab := model.ReasoningStep{Source: "A", Relation: "USES", Target: "B"}
	bc := model.ReasoningStep{Source: "B", Relation: "USES", Target: "C"}
	ad := model.ReasoningStep{Source: "A", Relation: "CONTAINS", Target: "D"}
	chains := map[string]model.ReasoningChain{
		"b": {Steps: []model.ReasoningStep{ab}},
		"c": {Steps: []model.ReasoningStep{ab, bc}},
		"d": {Steps: []model.ReasoningStep{ad}},
	}
	fused := []Candidate{{DocumentID: "b"}, {DocumentID: "c"}, {DocumentID: "x"}, {DocumentID: "d"}}

	got := selectChains(fused, chains, 5)
	want := []model.ReasoningChain{chains["c"], chains["d"]}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("selectChains() = %+v, want %+v", got, want)
	}
	if got := selectChains(fused, chains, 1); len(got) != 1 {
		t.Errorf("expected limit 1, got %d", len(got))
	}
}

func TestWeights_ForPlan(t *testing.T) {
	base := Weights{FTS: 0.35, Vector: 0.35, Graph: 0.20, Entity: 0.10}
	if got := base.ForPlan(RetrievalPlan{QueryType: QueryFact}); got.Graph != 0.20 {
		t.Errorf("fact plan must keep default graph weight 0.20, got %v", got.Graph)
	}
	for _, qt := range []QueryType{QueryImpact, QueryArchitecture} {
		got := base.ForPlan(RetrievalPlan{QueryType: qt})
		if got.Graph < 0.40 {
			t.Errorf("%s plan must boost graph weight to >= 0.40, got %v", qt, got.Graph)
		}
	}
}

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
		query     string
		intent    model.Intent
		wantType  QueryType
		wantGraph bool
		wantDepth int
	}{
		{"Was ist ADR-003?", model.IntentADR, QueryFact, false, 0},
		{"Was ist der Encryption Service?", model.IntentEntity, QueryEntity, false, 0},
		{"Welche Komponenten verwenden Encryption?", model.IntentImpact, QueryImpact, true, 3},
		// Architecture depth 4 is capped by maxGraphDepth 3.
		{"Wie funktioniert die Ingestion Pipeline?", model.IntentSearch, QueryArchitecture, true, 3},
		{"How does the indexer work?", model.IntentSearch, QueryArchitecture, true, 3},
		{"raspberry pi sensor", model.IntentSearch, QueryExploration, true, 1},
	}
	for _, tt := range tests {
		plan := p.Plan(tt.query, tt.intent)
		if plan.QueryType != tt.wantType {
			t.Errorf("%q: type = %s, want %s", tt.query, plan.QueryType, tt.wantType)
		}
		if !plan.UseFTS {
			t.Errorf("%q: FTS must always be enabled", tt.query)
		}
		if plan.UseVectors {
			t.Errorf("%q: vectors must stay disabled until embeddings exist", tt.query)
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

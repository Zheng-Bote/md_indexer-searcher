// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

package hybrid

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/md-indexer/internal/ai/model"
	"github.com/md-indexer/internal/entities"
	"github.com/md-indexer/internal/repository"
)

type note struct {
	title     string
	content   string
	tags      []string
	relations map[string][]string // relation type -> target titles
}

// setupGraphDB indexes notes the way the parser produces them from template frontmatter
// (uses: [...] -> USES relations, entity ID = title).
func setupGraphDB(t *testing.T, notes []note) *sql.DB {
	t.Helper()
	repo, err := repository.NewSQLiteRepository(filepath.Join(t.TempDir(), "knowledge.db"))
	if err != nil {
		t.Fatalf("failed to init repository: %v", err)
	}
	t.Cleanup(func() { repo.Close() })

	for i, n := range notes {
		id := fmt.Sprintf("doc-%d", i)
		doc := &entities.Document{
			ID:         id,
			Path:       "vault/" + n.title + ".md",
			Title:      n.title,
			Tags:       n.tags,
			Sections:   []entities.Section{{ID: id + "-overview", Heading: "Überblick", Level: 2, Content: n.content}},
			IsEntity:   true,
			EntityType: "Component",
			EntityName: n.title,
		}
		for rel, targets := range n.relations {
			for _, target := range targets {
				doc.EntityRelations = append(doc.EntityRelations, entities.EntityRelation{SourceID: n.title, TargetID: target, Type: rel})
			}
		}
		if err := repo.UpsertDocument(context.Background(), doc); err != nil {
			t.Fatalf("failed to upsert %s: %v", n.title, err)
		}
	}
	return repo.GetDB()
}

var pipelineNotes = []note{
	{title: "Transformation Engine", content: "Transforms collected records into packages.", relations: map[string][]string{"CONTAINS": {"Package"}}},
	{title: "Package", content: "A signed bundle of transformed records.", relations: map[string][]string{"USES": {"Encryption"}}},
	{title: "Encryption", content: "AES-GCM envelope encryption with rotating keys."},
	{title: "REST Delivery", content: "Delivers packages to subscribers over HTTPS.", relations: map[string][]string{"DEPENDS_ON": {"Encryption"}}},
	{title: "Glossary", content: "Terms used across the platform.", relations: map[string][]string{"REFERENCES": {"Encryption"}}},
}

func TestSQLiteGraph_MultiHopChain(t *testing.T) {
	g := NewSQLiteGraph(setupGraphDB(t, pipelineNotes))

	res, err := g.Expand(context.Background(), []string{"doc-0"}, 3, 25)
	if err != nil {
		t.Fatalf("expand failed: %v", err)
	}

	scores := make(map[string]float64)
	for _, c := range res.Candidates {
		scores[c.Title] = c.GraphScore
	}
	// Path products: CONTAINS 0.5 -> USES 1.0 -> DEPENDS_ON 0.9.
	want := map[string]float64{"Package": 0.5, "Encryption": 0.5, "REST Delivery": 0.45}
	for title, w := range want {
		if math.Abs(scores[title]-w) > 1e-9 {
			t.Errorf("%s: GraphScore = %v, want %v", title, scores[title], w)
		}
	}

	got := res.Chains["doc-3"].Steps
	wantChain := []model.ReasoningStep{
		{Source: "Transformation Engine", Relation: "CONTAINS", Target: "Package"},
		{Source: "Package", Relation: "USES", Target: "Encryption"},
		// Traversed against the stored direction; the step keeps the semantic direction.
		{Source: "REST Delivery", Relation: "DEPENDS_ON", Target: "Encryption"},
	}
	if !reflect.DeepEqual(got, wantChain) {
		t.Errorf("chain to REST Delivery = %+v, want %+v", got, wantChain)
	}
}

func TestSQLiteGraph_DepthLimit(t *testing.T) {
	g := NewSQLiteGraph(setupGraphDB(t, pipelineNotes))

	res, err := g.Expand(context.Background(), []string{"doc-0"}, 1, 25)
	if err != nil {
		t.Fatalf("expand failed: %v", err)
	}
	if len(res.Candidates) != 1 || res.Candidates[0].Title != "Package" {
		t.Errorf("depth 1 must only reach Package, got %+v", res.Candidates)
	}
}

func TestSQLiteGraph_RelationWeightsRankNeighbours(t *testing.T) {
	g := NewSQLiteGraph(setupGraphDB(t, pipelineNotes))

	// From Encryption: Package (USES 1.0), REST Delivery (DEPENDS_ON 0.9), Glossary (REFERENCES 0.2).
	res, err := g.Expand(context.Background(), []string{"doc-2"}, 1, 25)
	if err != nil {
		t.Fatalf("expand failed: %v", err)
	}
	var order []string
	for _, c := range res.Candidates {
		order = append(order, c.Title)
	}
	want := []string{"Package", "REST Delivery", "Glossary"}
	if !reflect.DeepEqual(order, want) {
		t.Errorf("neighbour order = %v, want %v", order, want)
	}
}

func TestSQLiteGraph_SharedTags(t *testing.T) {
	notes := []note{
		{title: "MQTT Broker", content: "Mosquitto broker.", tags: []string{"mqtt", "1"}},
		{title: "Sensor Node", content: "ESP32 publishing readings.", tags: []string{"mqtt"}},
		{title: "Numbered", content: "Only shares a numeric tag.", tags: []string{"1"}},
	}
	// "generic" is shared by more than maxTagDocuments notes and must not create edges.
	for i := 0; i <= maxTagDocuments; i++ {
		notes = append(notes, note{title: fmt.Sprintf("Generic %d", i), content: "x", tags: []string{"generic"}})
	}
	notes[0].tags = append(notes[0].tags, "generic")

	res, err := NewSQLiteGraph(setupGraphDB(t, notes)).Expand(context.Background(), []string{"doc-0"}, 1, 25)
	if err != nil {
		t.Fatalf("expand failed: %v", err)
	}
	if len(res.Candidates) != 1 || res.Candidates[0].Title != "Sensor Node" || res.Candidates[0].GraphScore != WeightSharedTag {
		t.Fatalf("expected only Sensor Node via shared tag, got %+v", res.Candidates)
	}
	want := []model.ReasoningStep{{Source: "MQTT Broker", Relation: "SHARES_TAG #mqtt", Target: "Sensor Node"}}
	if !reflect.DeepEqual(res.Chains["doc-1"].Steps, want) {
		t.Errorf("chain = %+v, want %+v", res.Chains["doc-1"].Steps, want)
	}
}

func TestRetriever_ImpactQueryReturnsChains(t *testing.T) {
	r := NewSQLiteRetriever(setupGraphDB(t, pipelineNotes), 3, nil, testWeights, 25, 25)

	res, err := r.Retrieve(context.Background(), "Welche Komponenten verwenden Encryption?", model.IntentImpact)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, s := range res.Snippets {
		if s.Title == "Package" && s.Strategy == SourceGraph {
			found = true
		}
	}
	if !found {
		t.Errorf("expected Package (USES Encryption) via graph, got %+v", res.Snippets)
	}
	if len(res.Chains) == 0 {
		t.Fatalf("expected reasoning chains for graph-reached documents")
	}
	want := model.ReasoningStep{Source: "Package", Relation: "USES", Target: "Encryption"}
	hasStep := false
	for _, c := range res.Chains {
		for _, s := range c.Steps {
			if s == want {
				hasStep = true
			}
		}
	}
	if !hasStep {
		t.Errorf("expected step %+v in chains %+v", want, res.Chains)
	}
}

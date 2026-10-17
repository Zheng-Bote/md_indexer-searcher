// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

package hybrid

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/md-indexer/internal/ai/model"
	"github.com/md-indexer/internal/entities"
	"github.com/md-indexer/internal/repository"
)

var testWeights = Weights{FTS: 0.35, Graph: 0.20, Entity: 0.10}

// setupDB builds knowledge.db through the real indexer migration and write path.
func setupDB(t *testing.T) *sql.DB {
	t.Helper()
	repo, err := repository.NewSQLiteRepository(filepath.Join(t.TempDir(), "knowledge.db"))
	if err != nil {
		t.Fatalf("failed to init repository: %v", err)
	}
	t.Cleanup(func() { repo.Close() })

	docs := []*entities.Document{
		{
			ID:    "doc-enc",
			Path:  "docs/components/encryption-service.md",
			Title: "Encryption Service",
			Tags:  []string{"security"},
			Sections: []entities.Section{
				{ID: "enc-title", Heading: "Encryption Service", Level: 1, Content: ""},
				{ID: "enc-overview", Heading: "Overview", Level: 2, Content: "Encrypts packages with AES-GCM envelope encryption."},
				{ID: "enc-api", Heading: "API", Level: 2, Content: "REST endpoint for key rotation."},
			},
			IsEntity:         true,
			EntityType:       "Component",
			EntityName:       "Encryption Service",
			EntityProperties: map[string]string{"description": "Envelope encryption component"},
			EntityRelations:  []entities.EntityRelation{{SourceID: "Encryption Service", TargetID: "KMS", Type: "uses"}},
		},
		{
			ID:    "doc-delivery",
			Path:  "docs/components/rest-delivery.md",
			Title: "REST Delivery",
			Sections: []entities.Section{
				{ID: "del-flow", Heading: "Flow", Level: 2, Content: "Delivers encrypted packages. See [[Encryption Service]]."},
			},
			Links: []entities.Link{{SourceDocID: "doc-delivery", TargetDocID: "Encryption Service"}},
		},
		{
			ID:    "doc-ingest",
			Path:  "docs/pipelines/ingestion.md",
			Title: "Ingestion Pipeline",
			Sections: []entities.Section{
				{ID: "ing-steps", Heading: "Steps", Level: 2, Content: "Collect, transform, package and hand over to [[REST Delivery]]."},
			},
			Links: []entities.Link{{SourceDocID: "doc-ingest", TargetDocID: "REST Delivery"}},
		},
		{
			ID:    "doc-noise",
			Path:  "notes/howto.md",
			Title: "Wie ich die Dinge mache",
			Sections: []entities.Section{
				{ID: "noise-1", Heading: "Allgemein", Level: 2, Content: "Wie ich die Dinge mache, wenn ich sie mache."},
			},
		},
	}
	for _, d := range docs {
		if err := repo.UpsertDocument(context.Background(), d); err != nil {
			t.Fatalf("failed to upsert %s: %v", d.Path, err)
		}
	}
	return repo.GetDB()
}

func paths(res *model.RetrievalResult) map[string]string {
	out := make(map[string]string)
	for _, s := range res.Snippets {
		out[s.DocumentPath+"#"+s.SectionTitle] = s.Strategy
	}
	return out
}

func TestRetriever_StopwordsDoNotMatch(t *testing.T) {
	r := NewSQLiteRetriever(setupDB(t), 3, nil, testWeights, 25, 25)

	res, err := r.Retrieve(context.Background(), "Wie verschlüssele ich mit der Encryption?", model.IntentSearch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := paths(res)
	if _, ok := got["notes/howto.md#Allgemein"]; ok {
		t.Errorf("stopword-only document must not be retrieved, got %v", got)
	}
	if _, ok := got["docs/components/encryption-service.md#Overview"]; !ok {
		t.Errorf("expected encryption overview, got %v", got)
	}
}

func TestRetriever_EntityQuery(t *testing.T) {
	r := NewSQLiteRetriever(setupDB(t), 3, nil, testWeights, 25, 25)

	res, err := r.Retrieve(context.Background(), "Was ist der Encryption Service?", model.IntentEntity)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Entities) != 1 || res.Entities[0].Name != "Encryption Service" {
		t.Fatalf("expected entity 'Encryption Service', got %+v", res.Entities)
	}
	e := res.Entities[0]
	if e.Type != "Component" || e.DocumentURI != "docs/components/encryption-service.md" || e.Description != "Envelope encryption component" {
		t.Errorf("unexpected entity details: %+v", e)
	}
	if len(res.Relations) != 1 || res.Relations[0].TargetEntity != "KMS" {
		t.Errorf("expected relation to KMS, got %+v", res.Relations)
	}
	if len(res.Snippets) == 0 || res.Snippets[0].DocumentPath != "docs/components/encryption-service.md" {
		t.Errorf("expected encryption service section ranked first, got %+v", res.Snippets)
	}
	for _, s := range res.Snippets {
		if s.Score <= 0 {
			t.Errorf("expected fused score > 0, got %v for %s", s.Score, s.DocumentPath)
		}
	}
	if res.Plan == nil || res.Plan.QueryType != string(QueryEntity) {
		t.Errorf("expected entity plan info in result, got %+v", res.Plan)
	}
	// Encryption Service has an API section and description, but no ADR relation or link -> 1 ADR gap.
	if len(res.Gaps) != 1 || res.Gaps[0].Entity != "Encryption Service" {
		t.Errorf("expected ADR knowledge gap for Encryption Service, got %+v", res.Gaps)
	}
}

func TestRetriever_ArchitectureQueryExpandsGraph(t *testing.T) {
	r := NewSQLiteRetriever(setupDB(t), 3, nil, testWeights, 25, 25)

	res, err := r.Retrieve(context.Background(), "Wie funktioniert die Ingestion Pipeline?", model.IntentSearch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := paths(res)
	if got["docs/pipelines/ingestion.md#Steps"] != SourceFTS {
		t.Errorf("expected ingestion steps via fts, got %v", got)
	}
	// Two hops: Ingestion -> REST Delivery -> Encryption Service.
	if got["docs/components/rest-delivery.md#Flow"] != SourceGraph {
		t.Errorf("expected REST delivery via graph, got %v", got)
	}
	if got["docs/components/encryption-service.md#Overview"] != SourceGraph {
		t.Errorf("expected encryption service via 2-hop graph, got %v", got)
	}
	if res.Architecture == nil {
		t.Fatalf("expected ArchitectureContext on architecture query")
	}
	if len(res.Architecture.Pipelines) == 0 || len(res.Architecture.Services) == 0 || len(res.Architecture.Components) == 0 {
		t.Errorf("expected pipelines, services and components in ArchitectureContext, got %+v", res.Architecture)
	}
}

func TestRetriever_FactQuerySkipsGraph(t *testing.T) {
	r := NewSQLiteRetriever(setupDB(t), 3, nil, testWeights, 25, 25)

	res, err := r.Retrieve(context.Background(), "Was ist die Ingestion Pipeline?", model.IntentADR)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, s := range res.Snippets {
		if s.Strategy == SourceGraph {
			t.Errorf("fact plan must not expand the graph, got %s", s.DocumentPath)
		}
	}
	if len(res.Entities) != 0 {
		t.Errorf("fact plan must not run entity retrieval, got %+v", res.Entities)
	}
}

func TestRetriever_EmptyQuery(t *testing.T) {
	r := NewSQLiteRetriever(setupDB(t), 3, nil, testWeights, 25, 25)
	res, err := r.Retrieve(context.Background(), "wie ich die", model.IntentSearch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Snippets) != 0 {
		t.Errorf("expected no snippets for stopword-only query, got %d", len(res.Snippets))
	}
}

func TestSQLiteFTS_FallbackWithoutSectionIndex(t *testing.T) {
	db := setupDB(t)
	if _, err := db.Exec("DROP TABLE fts_sections"); err != nil {
		t.Fatalf("failed to drop fts_sections: %v", err)
	}
	cands, err := NewSQLiteFTS(db).Search(context.Background(), []string{"encryption"}, 10)
	if err != nil {
		t.Fatalf("fallback search failed: %v", err)
	}
	found := false
	for _, c := range cands {
		if c.SectionID == "enc-overview" && c.FTSScore > 0 {
			found = true
		}
		if c.Content == "" {
			t.Errorf("fallback must skip empty sections, got %s", c.SectionID)
		}
	}
	if !found {
		t.Errorf("expected enc-overview from fts_documents fallback, got %+v", cands)
	}
}

func TestSQLiteFTS_TitleOutranksContent(t *testing.T) {
	cands, err := NewSQLiteFTS(setupDB(t)).Search(context.Background(), []string{"delivery"}, 10)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(cands) < 2 {
		t.Fatalf("expected title and content matches, got %+v", cands)
	}
	// "REST Delivery" title match must outrank the ingestion section that only mentions it in content.
	if cands[0].DocumentID != "doc-delivery" {
		t.Errorf("expected title match first, got %s", cands[0].DocumentID)
	}
}

func TestRetriever_EntityDocumentIDMismatch(t *testing.T) {
	repo, err := repository.NewSQLiteRepository(filepath.Join(t.TempDir(), "knowledge.db"))
	if err != nil {
		t.Fatalf("failed to init repository: %v", err)
	}
	t.Cleanup(func() { repo.Close() })

	// EntityName ("MQTT Broker") differs from document Title ("01 - MQTT Broker Setup").
	doc := &entities.Document{
		ID:    "doc-mqtt",
		Path:  "docs/services/mqtt.md",
		Title: "01 - MQTT Broker Setup",
		Sections: []entities.Section{
			{ID: "mqtt-1", Heading: "Konfiguration", Level: 2, Content: "Port 1883 und TLS-Konfiguration für den Broker."},
		},
		IsEntity:         true,
		EntityType:       "Service",
		EntityName:       "MQTT Broker",
		EntityProperties: map[string]string{"description": "Central message broker"},
	}
	if err := repo.UpsertDocument(context.Background(), doc); err != nil {
		t.Fatalf("failed to upsert doc: %v", err)
	}

	r := NewSQLiteRetriever(repo.GetDB(), 3, nil, testWeights, 25, 25)
	res, err := r.Retrieve(context.Background(), "Wie konfiguriere ich den MQTT Broker?", model.IntentEntity)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Entities) != 1 || res.Entities[0].DocumentURI != "docs/services/mqtt.md" {
		t.Fatalf("expected entity resolved via entities.document_id to docs/services/mqtt.md, got %+v", res.Entities)
	}
	if len(res.Snippets) == 0 || res.Snippets[0].DocumentPath != "docs/services/mqtt.md" {
		t.Errorf("expected section from docs/services/mqtt.md, got %+v", res.Snippets)
	}
}

func TestSQLiteFTS_GermanStemPrefixMatch(t *testing.T) {
	repo, err := repository.NewSQLiteRepository(filepath.Join(t.TempDir(), "knowledge.db"))
	if err != nil {
		t.Fatalf("failed to init repository: %v", err)
	}
	t.Cleanup(func() { repo.Close() })

	doc := &entities.Document{
		ID:    "doc-cfg",
		Path:  "docs/config.md",
		Title: "Systemhandbuch",
		Sections: []entities.Section{
			{ID: "cfg-1", Heading: "Konfiguration", Level: 2, Content: "Die Verschlüsselungsverfahren werden hier beschrieben."},
		},
	}
	if err := repo.UpsertDocument(context.Background(), doc); err != nil {
		t.Fatalf("failed to upsert doc: %v", err)
	}

	// "konfiguriere" stems to "konfigur"* -> matches "Konfiguration";
	// "verschlüsselung" stems to "verschlüssel"* -> matches "Verschlüsselungsverfahren".
	cands, err := NewSQLiteFTS(repo.GetDB()).Search(context.Background(), Terms("Wie konfiguriere ich die Verschlüsselung?"), 10)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(cands) != 1 || cands[0].SectionID != "cfg-1" {
		t.Errorf("expected stem prefix match on cfg-1, got %+v", cands)
	}
}

func TestRetriever_CaseInsensitiveLinksAndRelations(t *testing.T) {
	repo, err := repository.NewSQLiteRepository(filepath.Join(t.TempDir(), "knowledge.db"))
	if err != nil {
		t.Fatalf("failed to init repository: %v", err)
	}
	t.Cleanup(func() { repo.Close() })

	docs := []*entities.Document{
		{
			ID:    "doc-mqtt",
			Path:  "docs/services/mqtt-broker.md",
			Title: "MQTT Broker",
			Sections: []entities.Section{
				{ID: "mqtt-api", Heading: "API", Level: 2, Content: "Pub/Sub endpoint on port 1883."},
			},
			IsEntity:         true,
			EntityType:       "Service",
			EntityName:       "MQTT Broker",
			EntityProperties: map[string]string{"description": "Central MQTT broker"},
		},
		{
			ID:    "doc-fhem",
			Path:  "docs/services/fhem.md",
			Title: "FHEM Server",
			Sections: []entities.Section{
				{ID: "fhem-1", Heading: "Overview", Level: 2, Content: "Home automation server linking to [[mqtt broker]]."},
			},
			// Lowercase link target and lowercase entity relation target must still resolve to "MQTT Broker".
			Links:            []entities.Link{{SourceDocID: "doc-fhem", TargetDocID: "mqtt broker"}},
			IsEntity:         true,
			EntityType:       "Service",
			EntityName:       "FHEM Server",
			EntityProperties: map[string]string{"description": "FHEM automation"},
			EntityRelations:  []entities.EntityRelation{{SourceID: "FHEM Server", TargetID: "mqtt broker", Type: "USES"}},
		},
	}
	for _, d := range docs {
		if err := repo.UpsertDocument(context.Background(), d); err != nil {
			t.Fatalf("failed to upsert %s: %v", d.Path, err)
		}
	}

	g := NewSQLiteGraph(repo.GetDB())
	res, err := g.Expand(context.Background(), []string{"doc-fhem"}, 1, 10)
	if err != nil {
		t.Fatalf("graph expand failed: %v", err)
	}
	if len(res.Candidates) == 0 || res.Candidates[0].DocumentID != "doc-mqtt" {
		t.Fatalf("expected case-insensitive graph expansion to reach doc-mqtt, got %+v", res.Candidates)
	}
}

func TestAuditEntityGaps(t *testing.T) {
	db := setupDB(t)
	gaps, err := AuditEntityGaps(context.Background(), db)
	if err != nil {
		t.Fatalf("AuditEntityGaps failed: %v", err)
	}
	// setupDB has "Encryption Service" (Component) with description and API section, but no ADR linked.
	if len(gaps) != 1 || gaps[0].Entity != "Encryption Service" {
		t.Errorf("expected 1 ADR gap for Encryption Service, got %+v", gaps)
	}
}

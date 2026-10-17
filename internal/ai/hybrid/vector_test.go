// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

package hybrid

import (
	"context"
	"database/sql"
	"testing"

	"github.com/md-indexer/internal/ai/model"
	"github.com/md-indexer/internal/embedding"
)

// keywordEmbedder maps texts to fixed 3-d vectors so similarity is predictable:
// axis 0 = security, axis 1 = delivery, axis 2 = pipeline.
type keywordEmbedder struct {
	calls int
	query []float32
}

func (k *keywordEmbedder) Model() string { return "test-embed" }

func (k *keywordEmbedder) Embed(_ context.Context, texts []string) ([][]float32, error) {
	k.calls++
	out := make([][]float32, len(texts))
	for i := range texts {
		out[i] = k.query
	}
	return out, nil
}

func storeEmbedding(t *testing.T, db *sql.DB, sectionID string, vec []float32) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO section_embeddings (section_id, embedding, dimensions, model, content_hash, created_at)
		VALUES (?, ?, ?, 'test-embed', 'h', CURRENT_TIMESTAMP)`, sectionID, embedding.Encode(vec), len(vec))
	if err != nil {
		t.Fatalf("store embedding: %v", err)
	}
}

func TestSQLiteVectors_NoEmbeddingsSkipsEmbedder(t *testing.T) {
	emb := &keywordEmbedder{query: []float32{1, 0, 0}}
	cands, err := NewSQLiteVectors(setupDB(t), emb, 0.5).Search(context.Background(), "anything", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cands) != 0 || emb.calls != 0 {
		t.Errorf("expected no candidates and no embed call, got %d candidates, %d calls", len(cands), emb.calls)
	}
}

func TestSQLiteVectors_RanksAndFilters(t *testing.T) {
	db := setupDB(t)
	storeEmbedding(t, db, "enc-overview", []float32{1, 0.1, 0}) // security, very similar
	storeEmbedding(t, db, "enc-api", []float32{0.7, 0.7, 0})    // security + delivery, similar
	storeEmbedding(t, db, "ing-steps", []float32{0, 0, 1})      // pipeline, unrelated
	storeEmbedding(t, db, "gone", []float32{1, 0, 0})           // section no longer exists

	emb := &keywordEmbedder{query: []float32{1, 0, 0}}
	cands, err := NewSQLiteVectors(db, emb, 0.5).Search(context.Background(), "Verschlüsselung", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cands) != 2 {
		t.Fatalf("expected 2 candidates above min score, got %+v", cands)
	}
	if cands[0].SectionID != "enc-overview" || cands[1].SectionID != "enc-api" {
		t.Errorf("unexpected order: %s, %s", cands[0].SectionID, cands[1].SectionID)
	}
	if cands[0].VectorScore <= cands[1].VectorScore || cands[0].Path != "docs/components/encryption-service.md" {
		t.Errorf("unexpected candidate details: %+v", cands[0])
	}

	limited, _ := NewSQLiteVectors(db, emb, 0.5).Search(context.Background(), "Verschlüsselung", 1)
	if len(limited) != 1 {
		t.Errorf("expected limit 1, got %d", len(limited))
	}
}

func TestRetriever_VectorFindsWhatFTSMisses(t *testing.T) {
	db := setupDB(t)
	storeEmbedding(t, db, "enc-overview", []float32{1, 0, 0})
	vectors := NewSQLiteVectors(db, &keywordEmbedder{query: []float32{1, 0, 0}}, 0.5)
	weights := Weights{FTS: 0.35, Vector: 0.35, Graph: 0.20, Entity: 0.10}
	r := NewSQLiteRetriever(db, 3, vectors, weights, 25, 25)

	// "Chiffrierung" appears nowhere in the text; only the embedding links it to the encryption section.
	res, err := r.Retrieve(context.Background(), "Was ist Chiffrierung?", model.IntentADR)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Snippets) != 1 || res.Snippets[0].DocumentPath != "docs/components/encryption-service.md" || res.Snippets[0].Strategy != SourceVector {
		t.Errorf("expected encryption overview via vector, got %+v", res.Snippets)
	}
}

func TestSQLiteVectors_InMemoryCacheInvalidation(t *testing.T) {
	db := setupDB(t)
	storeEmbedding(t, db, "enc-overview", []float32{1, 0, 0})

	emb := &keywordEmbedder{query: []float32{1, 0, 0}}
	v := NewSQLiteVectors(db, emb, 0.5)

	cands1, err := v.Search(context.Background(), "Verschlüsselung", 10)
	if err != nil || len(cands1) != 1 {
		t.Fatalf("expected 1 candidate on first search, got %v (err=%v)", cands1, err)
	}
	if len(v.cacheVecs) != 1 {
		t.Fatalf("expected 1 cached entry, got %d", len(v.cacheVecs))
	}

	// Second search with unchanged table reuses the cached slice.
	cands2, err := v.Search(context.Background(), "Verschlüsselung", 10)
	if err != nil || len(cands2) != 1 {
		t.Fatalf("expected 1 candidate on cached search, got %v (err=%v)", cands2, err)
	}

	// Inserting a new embedding changes count(*) and invalidates the cache.
	storeEmbedding(t, db, "enc-api", []float32{0.9, 0.1, 0})
	cands3, err := v.Search(context.Background(), "Verschlüsselung", 10)
	if err != nil || len(cands3) != 2 {
		t.Fatalf("expected 2 candidates after cache invalidation, got %v (err=%v)", cands3, err)
	}
}

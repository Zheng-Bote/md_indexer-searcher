// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

package embedding

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestEncodeDecode(t *testing.T) {
	v := []float32{0.5, -1.25, 3, 0}
	got, err := Decode(Encode(v))
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if !reflect.DeepEqual(got, v) {
		t.Errorf("round trip = %v, want %v", got, v)
	}
	if _, err := Decode([]byte{1, 2, 3}); err == nil {
		t.Errorf("expected error for truncated blob")
	}
}

func TestCosine(t *testing.T) {
	if got := Cosine([]float32{1, 0}, []float32{2, 0}); math.Abs(got-1) > 1e-9 {
		t.Errorf("parallel vectors: got %v, want 1", got)
	}
	if got := Cosine([]float32{1, 0}, []float32{0, 3}); math.Abs(got) > 1e-9 {
		t.Errorf("orthogonal vectors: got %v, want 0", got)
	}
	if got := Cosine([]float32{0, 0}, []float32{1, 1}); got != 0 {
		t.Errorf("zero vector: got %v, want 0", got)
	}
}

func TestPrefixes(t *testing.T) {
	if got := QueryText("nomic-embed-text:latest", "q"); got != "search_query: q" {
		t.Errorf("nomic query prefix: got %q", got)
	}
	if got := DocumentText("nomic-embed-text", "d"); got != "search_document: d" {
		t.Errorf("nomic document prefix: got %q", got)
	}
	if got := QueryText("bge-m3", "q"); got != "q" {
		t.Errorf("bge-m3 must not be prefixed: got %q", got)
	}
}

func TestOllamaEmbedder_Embed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/embed" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		var req embedRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req.Model != "bge-m3" {
			t.Errorf("unexpected request %+v", req)
		}
		json.NewEncoder(w).Encode(embedResponse{Embeddings: [][]float32{{1, 0}, {0, 1}}})
	}))
	defer server.Close()

	e := NewOllamaEmbedder(server.URL, "bge-m3", server.Client())
	got, err := e.Embed(context.Background(), []string{"a", "b"})
	if err != nil {
		t.Fatalf("embed failed: %v", err)
	}
	if len(got) != 2 || got[1][1] != 1 {
		t.Errorf("unexpected embeddings %v", got)
	}

	if _, err := e.Embed(context.Background(), []string{"only-one-returned", "x", "y"}); err == nil {
		t.Errorf("expected error on embedding count mismatch")
	}
}

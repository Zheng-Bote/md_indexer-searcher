// SPDX-License-Identifier: Apache-2.0

package rest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/md-indexer/internal/ai/assistant"
	"github.com/md-indexer/internal/ai/classifier"
	"github.com/md-indexer/internal/ai/config"
	"github.com/md-indexer/internal/ai/contextbuilder"
	"github.com/md-indexer/internal/ai/provider"
	"github.com/md-indexer/internal/ai/retriever"
	"github.com/md-indexer/internal/entities"
	"github.com/md-indexer/internal/repository"
	"github.com/md-indexer/internal/search"
)

func setupTestServer(t *testing.T) (*repository.SQLiteRepository, http.Handler) {
	repo, err := repository.NewSQLiteRepository(":memory:")
	if err != nil {
		t.Fatalf("Failed to initialize in-memory repository: %v", err)
	}

	doc := &entities.Document{
		ID:         "doc-api-1",
		Path:       "/vault/PaymentService.md",
		Title:      "PaymentService",
		Content:    "PaymentService processes credit card transactions and fraud checks.",
		Hash:       "hash-pay",
		ModifiedAt: time.Now(),
		IndexedAt:  time.Now(),
		IsEntity:   true,
		EntityType: "payment-service",
		EntityName: "PaymentService",
		EntityProperties: map[string]string{
			"status": "production",
		},
		EntityRelations: []entities.EntityRelation{
			{SourceID: "PaymentService", TargetID: "BankGateway", Type: "CALLS"},
		},
		Links: []entities.Link{
			{SourceDocID: "doc-api-1", TargetDocID: "doc-api-2"},
		},
	}

	doc2 := &entities.Document{
		ID:         "doc-api-2",
		Path:       "/vault/BankGateway.md",
		Title:      "BankGateway",
		Content:    "BankGateway interacts with upstream banks.",
		Hash:       "hash-bank",
		ModifiedAt: time.Now(),
		IndexedAt:  time.Now(),
		IsEntity:   true,
		EntityType: "gateway",
		EntityName: "BankGateway",
	}

	ctx := context.Background()
	for _, d := range []*entities.Document{doc, doc2} {
		if err := repo.UpsertDocument(ctx, d); err != nil {
			t.Fatalf("Upsert failed: %v", err)
		}
	}

	se := search.NewSearchEngine(repo)
	srv := NewServer(se)
	return repo, srv.Handler()
}

func TestRESTServer_AllEndpoints(t *testing.T) {
	repo, handler := setupTestServer(t)
	defer repo.Close()

	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
		wantKey    string
	}{
		{
			name:       "Health check",
			method:     "GET",
			path:       "/health",
			wantStatus: http.StatusOK,
			wantKey:    "status",
		},
		{
			name:       "FTS Search",
			method:     "GET",
			path:       "/api/search?q=transactions",
			wantStatus: http.StatusOK,
			wantKey:    "success",
		},
		{
			name:       "Get Document by Name",
			method:     "GET",
			path:       "/api/documents/PaymentService",
			wantStatus: http.StatusOK,
			wantKey:    "success",
		},
		{
			name:       "Get Entity by Name",
			method:     "GET",
			path:       "/api/entities/PaymentService",
			wantStatus: http.StatusOK,
			wantKey:    "success",
		},
		{
			name:       "Get Backlinks",
			method:     "GET",
			path:       "/api/backlinks/BankGateway",
			wantStatus: http.StatusOK,
			wantKey:    "success",
		},
		{
			name:       "Get Impact Analysis",
			method:     "GET",
			path:       "/api/impact/BankGateway",
			wantStatus: http.StatusOK,
			wantKey:    "success",
		},
		{
			name:       "Get Dependency Graph",
			method:     "GET",
			path:       "/api/graph/PaymentService",
			wantStatus: http.StatusOK,
			wantKey:    "success",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			if w.Code != tc.wantStatus {
				t.Fatalf("Expected status %d, got %d. Body: %s", tc.wantStatus, w.Code, w.Body.String())
			}

			var bodyMap map[string]interface{}
			if err := json.Unmarshal(w.Body.Bytes(), &bodyMap); err != nil {
				t.Fatalf("Failed to parse JSON response: %v", err)
			}

			if _, ok := bodyMap[tc.wantKey]; !ok {
				t.Errorf("Expected response key '%s', got %+v", tc.wantKey, bodyMap)
			}
		})
	}

	// Test /api/ai/ask without assistant (501 Not Implemented)
	reqAI := httptest.NewRequest("POST", "/api/ai/ask", strings.NewReader(`{"question":"test"}`))
	wAI := httptest.NewRecorder()
	handler.ServeHTTP(wAI, reqAI)
	if wAI.Code != http.StatusNotImplemented {
		t.Errorf("expected 501 when AI assistant not configured, got %d", wAI.Code)
	}

	// Test /api/ai/ask with configured mock assistant
	cfg := config.DefaultConfig()
	clf := classifier.NewRuleClassifier()
	ret := retriever.NewRetriever(repo.GetDB(), 3)
	cb := contextbuilder.NewContextBuilder(1000)
	mockLLM := provider.NewMockProvider("Grounded REST answer")
	asst := assistant.NewAssistant(cfg, clf, ret, cb, mockLLM)

	serverWithAI := NewServer(search.NewSearchEngine(repo)).WithAssistant(asst)
	handlerWithAI := serverWithAI.Handler()

	// Invalid payload (400 Bad Request)
	reqBad := httptest.NewRequest("POST", "/api/ai/ask", strings.NewReader(`{}`))
	wBad := httptest.NewRecorder()
	handlerWithAI.ServeHTTP(wBad, reqBad)
	if wBad.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty question, got %d", wBad.Code)
	}

	// Valid payload (200 OK)
	reqGood := httptest.NewRequest("POST", "/api/ai/ask", strings.NewReader(`{"question":"What is PaymentService?"}`))
	wGood := httptest.NewRecorder()
	handlerWithAI.ServeHTTP(wGood, reqGood)
	if wGood.Code != http.StatusOK {
		t.Errorf("expected 200 for valid question, got %d: %s", wGood.Code, wGood.Body.String())
	}
}

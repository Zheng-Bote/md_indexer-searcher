// SPDX-License-Identifier: Apache-2.0

package rest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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
}

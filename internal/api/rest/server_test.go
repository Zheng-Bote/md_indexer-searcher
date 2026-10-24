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
	"github.com/md-indexer/internal/ai/model"
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

func TestRESTServer_AIModels(t *testing.T) {
	repo, err := repository.NewSQLiteRepository(":memory:")
	if err != nil {
		t.Fatalf("Failed to initialize in-memory repository: %v", err)
	}
	defer repo.Close()
	se := search.NewSearchEngine(repo)
	active := ActiveModels{Model: "qwen3:14b", EmbeddingModel: "bge-m3"}

	unreachable := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	unreachableURL := unreachable.URL
	unreachable.Close()

	mock := provider.NewMockProvider("")
	mock.Models = []model.ModelInfo{{Name: "qwen3:14b", Size: 1, Family: "qwen3"}}

	unreachableOllama := provider.NewOllamaProvider(unreachableURL, &http.Client{Timeout: time.Second})
	registry := provider.NewRegistry(unreachableOllama, []config.ProviderConfig{
		{ID: config.ProviderClaude, DisplayName: "Claude", Protocol: config.ProtocolClaude, APIKey: "k", Models: []string{"claude-opus-5-5"}},
	}, time.Minute, nil)

	tests := []struct {
		name           string
		server         *Server
		wantStatus     int
		wantModel      string
		wantLocalError bool
	}{
		{"AI disabled", NewServer(se), http.StatusNotImplemented, "", false},
		{"Ollama unreachable", NewServer(se).WithModelLister(unreachableOllama, active), http.StatusBadGateway, "", false},
		{"Models listed", NewServer(se).WithModelLister(mock, active), http.StatusOK, "qwen3:14b", false},
		{"Cloud models without Ollama", NewServer(se).WithModelLister(registry, active), http.StatusOK, "claude-opus-5-5", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			tt.server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/ai/models", nil))
			if rec.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d: %s", tt.wantStatus, rec.Code, rec.Body.String())
			}
			if tt.wantStatus != http.StatusOK {
				return
			}
			var body struct {
				Success bool `json:"success"`
				Data    struct {
					Models     []model.ModelInfo `json:"models"`
					Active     ActiveModels      `json:"active"`
					LocalError string            `json:"local_error"`
				} `json:"data"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("invalid JSON: %v", err)
			}
			if !body.Success || len(body.Data.Models) != 1 || body.Data.Models[0].Name != tt.wantModel {
				t.Errorf("unexpected models payload: %s", rec.Body.String())
			}
			if body.Data.Active != active {
				t.Errorf("expected active %+v, got %+v", active, body.Data.Active)
			}
			if (body.Data.LocalError != "") != tt.wantLocalError {
				t.Errorf("expected local_error set=%v, got %q", tt.wantLocalError, body.Data.LocalError)
			}
		})
	}
}

func TestRESTServer_Document(t *testing.T) {
	repo, handler := setupTestServer(t)
	defer repo.Close()
	note := &entities.Document{
		ID: "doc-api-3", Path: "/vault/Notes/Overview.md", Title: "Overview",
		Markdown: "# Overview\n\nUses [[bankgateway]].",
		Links:    []entities.Link{{SourceDocID: "doc-api-3", TargetDocID: "bankgateway"}},
	}
	if err := repo.UpsertDocument(context.Background(), note); err != nil {
		t.Fatalf("Upsert failed: %v", err)
	}

	tests := []struct {
		name       string
		path       string
		wantStatus int
		wantPath   string
	}{
		{"by path", "/api/document?path=%2Fvault%2FNotes%2FOverview.md", http.StatusOK, "/vault/Notes/Overview.md"},
		{"by title", "/api/document?title=overview&from=%2Fvault%2FX.md", http.StatusOK, "/vault/Notes/Overview.md"},
		{"unknown path", "/api/document?path=%2Fetc%2Fpasswd", http.StatusNotFound, ""},
		{"unknown title", "/api/document?title=Kubernetes", http.StatusNotFound, ""},
		{"missing parameters", "/api/document", http.StatusBadRequest, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tc.path, nil))
			if rec.Code != tc.wantStatus {
				t.Fatalf("status %d, want %d. Body: %s", rec.Code, tc.wantStatus, rec.Body.String())
			}
			var body struct {
				Success bool                  `json:"success"`
				Data    entities.DocumentView `json:"data"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("invalid JSON: %v", err)
			}
			if body.Success != (tc.wantStatus == http.StatusOK) || body.Data.Path != tc.wantPath {
				t.Fatalf("unexpected body %+v", body)
			}
			if tc.wantStatus == http.StatusOK {
				want := entities.ResolvedLink{Target: "bankgateway", Path: "/vault/BankGateway.md"}
				if body.Data.Markdown != note.Markdown || len(body.Data.Links) != 1 || body.Data.Links[0] != want {
					t.Errorf("unexpected document %+v", body.Data)
				}
			}
		})
	}
}

func TestRESTServer_SendsNoCORSHeaders(t *testing.T) {
	repo, handler := setupTestServer(t)
	defer repo.Close()

	for _, path := range []string{"/health", "/api/search?q=transactions", "/api/document", "/api/ai/ask"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Origin", "https://evil.example")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		for name := range rec.Header() {
			if strings.HasPrefix(name, "Access-Control-") {
				t.Errorf("%s: unexpected CORS header %s: %s", path, name, rec.Header().Get(name))
			}
		}
	}
}

type restResolver map[string]provider.LLMProvider

func (r restResolver) Resolve(ref model.ModelRef) (provider.LLMProvider, string, error) {
	if p, ok := r[ref.Provider+"/"+ref.Name]; ok {
		return p, model.LocalityCloud, nil
	}
	return nil, "", &provider.ErrUnknownModel{Ref: ref}
}

func TestRESTServer_AskWithModel(t *testing.T) {
	repo, _ := setupTestServer(t)
	defer repo.Close()
	cfg := config.DefaultConfig()
	limited := &provider.MockProvider{ResponseFunc: func(ctx context.Context, req *model.GenerateRequest) (*model.GenerateResponse, error) {
		return nil, &provider.Error{Provider: "openrouter", Kind: provider.ErrKindRateLimit, Status: 429, Detail: "slow down"}
	}}
	asst := assistant.NewAssistant(cfg, classifier.NewRuleClassifier(), retriever.NewRetriever(repo.GetDB(), 3), contextbuilder.NewContextBuilder(1000), provider.NewMockProvider("lokal")).
		WithModels(restResolver{
			"claude/claude-opus-5-5": provider.NewMockProvider("cloud answer"),
			"openrouter/x:free":      limited,
		}, contextbuilder.NewContextBuilder(4000))
	handler := NewServer(search.NewSearchEngine(repo)).WithAssistant(asst).Handler()

	tests := []struct {
		name, body, wantKind string
		wantStatus           int
	}{
		{"cloud model", `{"question":"Was ist PaymentService?","model":{"provider":"claude","name":"claude-opus-5-5"}}`, "", http.StatusOK},
		{"unknown model", `{"question":"q","model":{"provider":"groq","name":"llama"}}`, "", http.StatusBadRequest},
		{"rate limit", `{"question":"q","model":{"provider":"openrouter","name":"x:free"}}`, provider.ErrKindRateLimit, http.StatusBadGateway},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/ai/ask", strings.NewReader(tc.body)))
			if rec.Code != tc.wantStatus {
				t.Fatalf("status %d, want %d: %s", rec.Code, tc.wantStatus, rec.Body.String())
			}
			var body struct {
				Success   bool         `json:"success"`
				Data      model.Answer `json:"data"`
				ErrorKind string       `json:"error_kind"`
			}
			json.Unmarshal(rec.Body.Bytes(), &body)
			if body.ErrorKind != tc.wantKind {
				t.Errorf("error_kind %q, want %q", body.ErrorKind, tc.wantKind)
			}
			if tc.wantStatus == http.StatusOK && (body.Data.Answer != "cloud answer" || body.Data.Locality != model.LocalityCloud) {
				t.Errorf("unexpected answer %+v", body.Data)
			}
		})
	}
}

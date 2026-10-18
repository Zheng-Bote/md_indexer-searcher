// SPDX-License-Identifier: Apache-2.0

package rest

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/md-indexer/internal/ai/assistant"
	"github.com/md-indexer/internal/ai/provider"
	"github.com/md-indexer/internal/entities"
	"github.com/md-indexer/internal/search"
)

type Server struct {
	searchEngine search.SearchEngine
	assistant    *assistant.Assistant
	modelLister  provider.ModelLister
	activeModels ActiveModels
}

// ActiveModels names the generation and embedding models the server is configured with.
type ActiveModels struct {
	Model          string `json:"model"`
	EmbeddingModel string `json:"embedding_model"`
}

func NewServer(se search.SearchEngine) *Server {
	return &Server{searchEngine: se}
}

// WithAssistant attaches an AI assistant orchestrator to the REST server.
func (s *Server) WithAssistant(a *assistant.Assistant) *Server {
	s.assistant = a
	return s
}

// WithModelLister enables /api/ai/models using the given backend and reports the active models.
func (s *Server) WithModelLister(l provider.ModelLister, active ActiveModels) *Server {
	s.modelLister = l
	s.activeModels = active
	return s
}

func respondJSON(w http.ResponseWriter, data interface{}, err error) {
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "data": data})
}

func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})

	r.Get("/api/search", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		res, err := s.searchEngine.Search(r.Context(), q)
		respondJSON(w, map[string]interface{}{"results": res}, err)
	})

	r.Get("/api/documents/{name}", func(w http.ResponseWriter, r *http.Request) {
		name := chi.URLParam(r, "name")
		res, err := s.searchEngine.FindDocument(r.Context(), name)
		respondJSON(w, res, err)
	})

	// Serves raw Markdown from the index only; paths that are not indexed documents yield 404.
	r.Get("/api/document", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		var doc *entities.DocumentView
		var err error
		switch {
		case q.Get("path") != "":
			doc, err = s.searchEngine.Document(r.Context(), q.Get("path"))
		case q.Get("title") != "":
			doc, err = s.searchEngine.ResolveDocument(r.Context(), q.Get("title"), q.Get("from"))
		default:
			http.Error(w, `{"success":false,"error":"path or title required"}`, http.StatusBadRequest)
			return
		}
		if err == nil && doc == nil {
			http.Error(w, `{"success":false,"error":"document not found"}`, http.StatusNotFound)
			return
		}
		respondJSON(w, doc, err)
	})

	r.Get("/api/entities/{name}", func(w http.ResponseWriter, r *http.Request) {
		name := chi.URLParam(r, "name")
		res, err := s.searchEngine.FindEntity(r.Context(), name)
		respondJSON(w, res, err)
	})

	r.Get("/api/backlinks/{name}", func(w http.ResponseWriter, r *http.Request) {
		name := chi.URLParam(r, "name")
		res, err := s.searchEngine.Backlinks(r.Context(), name)
		respondJSON(w, res, err)
	})

	r.Get("/api/impact/{name}", func(w http.ResponseWriter, r *http.Request) {
		name := chi.URLParam(r, "name")
		res, err := s.searchEngine.Impact(r.Context(), name)
		respondJSON(w, res, err)
	})

	r.Get("/api/graph/{name}", func(w http.ResponseWriter, r *http.Request) {
		name := chi.URLParam(r, "name")
		res, err := s.searchEngine.Graph(r.Context(), name)
		respondJSON(w, res, err)
	})

	r.Post("/api/ai/ask", func(w http.ResponseWriter, r *http.Request) {
		if s.assistant == nil {
			http.Error(w, `{"success":false,"error":"AI assistant not configured"}`, http.StatusNotImplemented)
			return
		}
		var req struct {
			Question string `json:"question"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Question == "" {
			http.Error(w, `{"success":false,"error":"invalid question payload"}`, http.StatusBadRequest)
			return
		}
		ans, err := s.assistant.Ask(r.Context(), req.Question, nil)
		respondJSON(w, ans, err)
	})

	r.Get("/api/ai/models", func(w http.ResponseWriter, r *http.Request) {
		if s.modelLister == nil {
			http.Error(w, `{"success":false,"error":"AI assistant not configured"}`, http.StatusNotImplemented)
			return
		}
		models, err := s.modelLister.ListModels(r.Context())
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": err.Error()})
			return
		}
		respondJSON(w, map[string]interface{}{"models": models, "active": s.activeModels}, nil)
	})

	return r
}

package rest

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/md-indexer/internal/search"
)

type Server struct {
	searchEngine search.SearchEngine
}

func NewServer(se search.SearchEngine) *Server {
	return &Server{searchEngine: se}
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
	r.Use(middleware.SetHeader("Access-Control-Allow-Origin", "*"))

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

	return r
}

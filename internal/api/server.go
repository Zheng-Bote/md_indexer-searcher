package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/md-indexer/internal/config"
	"github.com/md-indexer/internal/repository"
)

type Server struct {
	cfg  config.Config
	repo *repository.SQLiteRepository
}

func NewServer(cfg config.Config, repo *repository.SQLiteRepository) *Server {
	return &Server{
		cfg:  cfg,
		repo: repo,
	}
}

func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/health", s.healthCheck)
	r.Get("/documents", s.getDocuments)
	r.Get("/search", s.search)
	// Additional routes like /documents/{id}, /graph/document/{id}, etc.
	return r
}

func (s *Server) healthCheck(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}

func (s *Server) getDocuments(w http.ResponseWriter, r *http.Request) {
	docs, err := s.repo.GetAllDocuments(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(docs)
}

func (s *Server) search(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		http.Error(w, "missing query parameter 'q'", http.StatusBadRequest)
		return
	}

	results, err := s.repo.Search(r.Context(), query)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

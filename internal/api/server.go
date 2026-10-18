// SPDX-License-Identifier: Apache-2.0

package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// Server is md-indexer's HTTP server. It only reports readiness: /health answers once the initial
// index is built. Queries go through md-api, which uses the search engine layer.
type Server struct{}

func NewServer() *Server {
	return &Server{}
}

func (s *Server) Handler() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Get("/health", s.healthCheck)
	return r
}

func (s *Server) healthCheck(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}

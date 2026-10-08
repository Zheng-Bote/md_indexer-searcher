package main

import (
	"context"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/md-indexer/internal/api/rest"
	"github.com/md-indexer/internal/repository"
	"github.com/md-indexer/internal/search"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})
	dbPath := flag.String("db", "index.db", "Path to SQLite database")
	port := flag.String("port", "8081", "HTTP server port")
	flag.Parse()

	repo, err := repository.NewSQLiteRepository(*dbPath)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize repository")
	}
	defer repo.Close()

	searchEngine := search.NewSearchEngine(repo)
	
	server := rest.NewServer(searchEngine)
	httpServer := &http.Server{
		Addr:    ":" + *port,
		Handler: server.Handler(),
	}

	go func() {
		log.Info().Str("port", *port).Msg("Starting API HTTP server")
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("HTTP server failed")
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Info().Msg("Shutting down API gracefully...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	httpServer.Shutdown(ctx)
}

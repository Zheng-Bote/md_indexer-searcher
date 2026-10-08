package main

import (
	"context"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/md-indexer/internal/api"
	"github.com/md-indexer/internal/config"
	"github.com/md-indexer/internal/repository"
	"github.com/md-indexer/internal/service"
	"github.com/md-indexer/internal/watcher"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})

	dir := flag.String("dir", ".", "Directory to index")
	dbPath := flag.String("db", "index.db", "Path to SQLite database")
	port := flag.String("port", "8080", "HTTP server port")
	flag.Parse()

	cfg := config.Config{
		Directory: *dir,
		DBPath:    *dbPath,
		Port:      *port,
	}

	repo, err := repository.NewSQLiteRepository(cfg.DBPath)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize repository")
	}
	defer repo.Close()

	indexerService := service.NewIndexerService(repo, cfg.Directory)

	log.Info().Msg("Starting full re-index...")
	if err := indexerService.IndexAll(context.Background()); err != nil {
		log.Fatal().Err(err).Msg("Failed to index directory")
	}

	fsWatcher, err := watcher.NewWatcher(cfg.Directory, indexerService)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize watcher")
	}
	defer fsWatcher.Close()
	go fsWatcher.Start()

	server := api.NewServer(cfg, repo)
	httpServer := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: server.Handler(),
	}

	go func() {
		log.Info().Str("port", cfg.Port).Msg("Starting HTTP server")
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("HTTP server failed")
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Info().Msg("Shutting down gracefully...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Fatal().Err(err).Msg("Server shutdown failed")
	}
}

// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/md-indexer/internal/ai/assistant"
	"github.com/md-indexer/internal/ai/classifier"
	"github.com/md-indexer/internal/ai/config"
	"github.com/md-indexer/internal/ai/contextbuilder"
	"github.com/md-indexer/internal/ai/provider"
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
	aiEnabled := flag.Bool("ai", false, "Enable AI assistant endpoint (/api/ai/ask)")
	ollamaHost := flag.String("ollama-host", "http://localhost:11434", "Ollama API host URL")
	modelName := flag.String("model", "gemma3:1b", "LLM model name for AI endpoint")
	flag.Parse()

	repo, err := repository.NewSQLiteRepository(*dbPath)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize repository")
	}
	defer repo.Close()

	searchEngine := search.NewSearchEngine(repo)
	
	server := rest.NewServer(searchEngine)

	if *aiEnabled || os.Getenv("MD_API_ENABLE_AI") == "true" {
		aiCfg := config.DefaultConfig()
		aiCfg.DBPath = *dbPath
		aiCfg.OllamaHost = *ollamaHost
		aiCfg.Model = *modelName
		clf := classifier.NewRuleClassifier()
		ret := assistant.NewRetriever(repo.GetDB(), aiCfg)
		cb := contextbuilder.NewContextBuilder(aiCfg.MaxContextTokens)
		prov := provider.NewOllamaProvider(aiCfg.OllamaHost, &http.Client{Timeout: aiCfg.Timeout()})
		asst := assistant.NewAssistant(aiCfg, clf, ret, cb, prov)
		server.WithAssistant(asst)
		log.Info().Str("model", *modelName).Msg("AI Assistant endpoint /api/ai/ask enabled")
	}

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

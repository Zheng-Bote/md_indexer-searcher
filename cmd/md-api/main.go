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
	"github.com/md-indexer/internal/netaddr"
	"github.com/md-indexer/internal/repository"
	"github.com/md-indexer/internal/search"
	"github.com/md-indexer/internal/vaultdb"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})
	aiCfg := config.DefaultConfig()
	_ = aiCfg.LoadDefaultFiles()
	aiCfg.LoadFromEnv()

	dbPath := flag.String("db", "", "Path to SQLite database (default: index database of -vault)")
	vault := flag.String("vault", ".", "Vault whose index database is used when -db is not set")
	port := flag.String("port", "8081", "HTTP server port")
	host := flag.String("host", "127.0.0.1", "HTTP bind address; use 0.0.0.0 to listen on all interfaces (no authentication)")
	aiEnabled := flag.Bool("ai", false, "Enable AI assistant endpoint (/api/ai/ask)")
	aiConfigFile := flag.String("ai-config", "", "Optional path to md-ai YAML configuration file")
	ollamaHost := flag.String("ollama-host", aiCfg.OllamaHost, "Ollama API host URL")
	modelName := flag.String("model", aiCfg.Model, "LLM model name for AI endpoint")
	retrieverMode := flag.String("retriever", aiCfg.Retriever, "AI retriever implementation: hybrid or classic")
	enableVectors := flag.Bool("vectors", aiCfg.EnableVectors, "Enable section embeddings for AI retrieval")
	embeddingModel := flag.String("embedding-model", aiCfg.EmbeddingModel, "Ollama embedding model for AI retrieval")
	maxContextTokens := flag.Int("max-context-tokens", aiCfg.MaxContextTokens, "Max context token budget for AI endpoint")
	timeoutSec := flag.Int("timeout", aiCfg.TimeoutSeconds, "LLM request timeout in seconds")
	envFile := flag.String("env-file", aiCfg.EnvFile, "Path to md_indexer's .env with cloud provider keys (default: MD_AI_ENV_FILE or ~/.config/md-indexer/.env)")
	flag.Parse()

	if *dbPath == "" {
		path, err := resolveDB(*vault)
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to resolve database")
		}
		*dbPath = path
	}

	repo, err := repository.NewSQLiteRepository(*dbPath)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize repository")
	}
	defer repo.Close()

	searchEngine := search.NewSearchEngine(repo)

	server := rest.NewServer(searchEngine)

	if *aiEnabled || os.Getenv("MD_API_ENABLE_AI") == "true" {
		if *aiConfigFile != "" {
			if err := aiCfg.LoadFromFile(*aiConfigFile); err != nil {
				log.Fatal().Err(err).Str("config", *aiConfigFile).Msg("Failed to load AI config file")
			}
		}
		aiCfg.DBPath = *dbPath
		aiCfg.OllamaHost = *ollamaHost
		aiCfg.Model = *modelName
		aiCfg.Retriever = *retrieverMode
		aiCfg.EnableVectors = *enableVectors
		aiCfg.EmbeddingModel = *embeddingModel
		aiCfg.MaxContextTokens = *maxContextTokens
		aiCfg.TimeoutSeconds = *timeoutSec
		aiCfg.EnvFile = *envFile
		if err := aiCfg.Validate(); err != nil {
			log.Fatal().Err(err).Msg("Invalid AI configuration")
		}

		clf := classifier.NewRuleClassifier()
		ret := assistant.NewRetriever(repo.GetDB(), aiCfg)
		cb := contextbuilder.NewContextBuilder(aiCfg.MaxContextTokens)
		prov := provider.NewOllamaProvider(aiCfg.OllamaHost, &http.Client{Timeout: aiCfg.Timeout()})
		registry, err := newRegistry(aiCfg, prov)
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to load cloud provider configuration")
		}
		asst := assistant.NewAssistant(aiCfg, clf, ret, cb, prov).
			WithModels(registry, contextbuilder.NewContextBuilder(aiCfg.CloudMaxContextTokens))
		active := rest.ActiveModels{Model: aiCfg.Model}
		if aiCfg.EnableVectors {
			active.EmbeddingModel = aiCfg.EmbeddingModel
		}
		server.WithAssistant(asst).WithModelLister(registry, active)
		log.Info().Str("model", aiCfg.Model).Str("retriever", aiCfg.Retriever).Msg("AI Assistant endpoint /api/ai/ask enabled")
	}

	addr, exposed := netaddr.Listen(*host, *port)
	if exposed {
		log.Warn().Str("addr", addr).Msg("Listening on a non-loopback address: reachable from the network without authentication")
	}
	httpServer := &http.Server{
		Addr:    addr,
		Handler: server.Handler(),
	}

	go func() {
		log.Info().Str("addr", addr).Msg("Starting API HTTP server")
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

// resolveDB finds the existing index database of a vault without creating one.
func resolveDB(vault string) (string, error) {
	dataDir, err := vaultdb.UserDataDir()
	if err != nil {
		return "", err
	}
	return vaultdb.ForReader(vault, dataDir)
}

// newRegistry loads the .env file and registers the enabled cloud providers next to the local Ollama daemon.
// Only provider names and model counts are logged, never keys.
func newRegistry(cfg *config.Config, local *provider.OllamaProvider) (*provider.Registry, error) {
	envFile, err := config.LoadEnvFile(cfg.EnvFile)
	if err != nil {
		return nil, err
	}
	if envFile.Insecure {
		log.Warn().Str("file", envFile.Path).Msg("The .env file is readable by other users; restrict it with chmod 600")
	}
	registry := provider.NewRegistry(local, envFile.Providers(), cfg.CloudTimeout(), cfg.EffortFor)
	for id, models := range registry.Providers() {
		log.Info().Str("provider", id).Int("models", models).Msg("Cloud provider enabled")
	}
	return registry, nil
}

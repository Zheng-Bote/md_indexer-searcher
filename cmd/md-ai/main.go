// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/md-indexer/internal/ai/assistant"
	"github.com/md-indexer/internal/ai/classifier"
	"github.com/md-indexer/internal/ai/config"
	"github.com/md-indexer/internal/ai/contextbuilder"
	"github.com/md-indexer/internal/ai/conversation"
	"github.com/md-indexer/internal/ai/hybrid"
	"github.com/md-indexer/internal/ai/model"
	"github.com/md-indexer/internal/ai/provider"
	"github.com/md-indexer/internal/vaultdb"
	_ "modernc.org/sqlite"
)

var (
	cfgFile      string
	providerID   string
	vaultDir     string
	retrieveOnly bool
	streamOutput bool
	cfg          = config.DefaultConfig()
	rootCmd      = &cobra.Command{
		Use:   "md-ai",
		Short: "Grounded local AI assistant over Markdown knowledge bases",
		Long: `md-ai is a local, grounded AI retrieval-augmented generation (RAG) assistant
that queries Markdown notes indexed by md-indexer (SQLite) using local LLMs (Ollama).`,
	}
)

func init() {
	_ = cfg.LoadDefaultFiles()
	cfg.LoadFromEnv()

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "Path to YAML configuration file (defaults to ~/.config/md-indexer/ai.yaml and ./md-ai.yaml)")
	rootCmd.PersistentFlags().StringVar(&cfg.DBPath, "db", cfg.DBPath, "Path to SQLite knowledge database (default: index database of --vault)")
	rootCmd.PersistentFlags().StringVar(&vaultDir, "vault", ".", "Vault whose index database is used when no database path is configured")
	rootCmd.PersistentFlags().StringVar(&cfg.OllamaHost, "ollama-host", cfg.OllamaHost, "Ollama API host URL")
	rootCmd.PersistentFlags().StringVarP(&cfg.Model, "model", "m", cfg.Model, "LLM model name")
	rootCmd.PersistentFlags().StringVar(&providerID, "provider", config.ProviderOllama, "Provider of --model: ollama, ollama-cloud, openrouter, nvidia, claude or gemini (cloud providers need a key in the .env)")
	rootCmd.PersistentFlags().StringVar(&cfg.EnvFile, "env-file", cfg.EnvFile, "Path to md_indexer's .env with cloud provider keys (default: MD_AI_ENV_FILE or ~/.config/md-indexer/.env)")
	rootCmd.PersistentFlags().Float64Var(&cfg.Temperature, "temperature", cfg.Temperature, "Generation temperature")
	rootCmd.PersistentFlags().IntVar(&cfg.MaxTokens, "max-tokens", cfg.MaxTokens, "Max generated tokens")
	rootCmd.PersistentFlags().IntVar(&cfg.MaxContextTokens, "max-context-tokens", cfg.MaxContextTokens, "Max context token budget")
	rootCmd.PersistentFlags().IntVar(&cfg.GraphDepth, "graph-depth", cfg.GraphDepth, "BFS graph traversal depth")
	rootCmd.PersistentFlags().StringVar(&cfg.Retriever, "retriever", cfg.Retriever, "Retrieval implementation: hybrid or classic")
	rootCmd.PersistentFlags().BoolVar(&cfg.EnableVectors, "vectors", cfg.EnableVectors, "Use section embeddings for retrieval (hybrid retriever)")
	rootCmd.PersistentFlags().StringVar(&cfg.EmbeddingModel, "embedding-model", cfg.EmbeddingModel, "Ollama embedding model; must match the indexer's -embed-model")
	rootCmd.PersistentFlags().IntVar(&cfg.TimeoutSeconds, "timeout", cfg.TimeoutSeconds, "LLM request timeout in seconds")
	rootCmd.PersistentFlags().BoolVar(&cfg.JSONOutput, "json", cfg.JSONOutput, "Output response in JSON format")
	rootCmd.PersistentFlags().BoolVar(&streamOutput, "stream", false, "Stream generated tokens incrementally in text mode")

	askCmd.Flags().BoolVar(&retrieveOnly, "retrieve-only", false, "Execute query planning, retrieval, and context budgeting without calling the LLM")

	rootCmd.AddCommand(askCmd)
	rootCmd.AddCommand(chatCmd)
	rootCmd.AddCommand(gapsCmd)
	rootCmd.AddCommand(versionCmd)
}

func initAssistant() (*assistant.Assistant, *sql.DB, error) {
	if cfgFile != "" {
		if err := cfg.LoadFromFile(cfgFile); err != nil {
			return nil, nil, fmt.Errorf("failed to load config %s: %w", cfgFile, err)
		}
	}
	if cfg.DBPath == "" {
		dataDir, err := vaultdb.UserDataDir()
		if err != nil {
			return nil, nil, err
		}
		if cfg.DBPath, err = vaultdb.ForReader(vaultDir, dataDir); err != nil {
			return nil, nil, err
		}
	}
	if err := cfg.Validate(); err != nil {
		return nil, nil, fmt.Errorf("invalid configuration: %w", err)
	}

	// Connect strictly read-only to SQLite
	dsn := fmt.Sprintf("%s?mode=ro", cfg.DBPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open database: %w", err)
	}

	clf := classifier.NewRuleClassifier()
	ret := assistant.NewRetriever(db, cfg)
	cb := contextbuilder.NewContextBuilder(cfg.MaxContextTokens)
	prov := provider.NewOllamaProvider(cfg.OllamaHost, &http.Client{Timeout: cfg.Timeout()})

	asst := assistant.NewAssistant(cfg, clf, ret, cb, prov)
	envFile, err := config.LoadEnvFile(cfg.EnvFile)
	if err != nil {
		return nil, nil, err
	}
	if envFile.Insecure {
		fmt.Fprintf(os.Stderr, "Warning: %s is readable by other users; restrict it with chmod 600\n", envFile.Path)
	}
	registry := provider.NewRegistry(prov, envFile.Providers(), cfg.CloudTimeout(), cfg.EffortFor)
	asst.WithModels(registry, contextbuilder.NewContextBuilder(cfg.CloudMaxContextTokens))
	ref := model.ModelRef{Provider: providerID, Name: cfg.Model}
	_, locality, err := registry.Resolve(ref)
	if err != nil {
		return nil, nil, err
	}
	if locality == model.LocalityCloud {
		fmt.Fprintf(os.Stderr, "Note: %s (%s) is a cloud model; excerpts of your notes are sent to that provider.\n", cfg.Model, providerID)
	}
	asst.WithDefaultModel(ref)
	return asst, db, nil
}

var askCmd = &cobra.Command{
	Use:   "ask <question>",
	Short: "Ask a grounded single-shot question against the knowledge base",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		question := strings.Join(args, " ")
		asst, db, err := initAssistant()
		if err != nil {
			return err
		}
		defer db.Close()

		ctx := cmd.Context()
		if ctx == nil {
			ctx = context.Background()
		}

		if retrieveOnly {
			ans, err := asst.RetrieveOnly(ctx, question)
			if err != nil {
				return fmt.Errorf("error retrieving context: %w", err)
			}
			if cfg.JSONOutput {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(ans)
			}
			printFormattedAnswer(ans)
			return nil
		}

		if streamOutput && !cfg.JSONOutput {
			ans, err := runStreamingAsk(ctx, asst, question, nil)
			if err != nil {
				return fmt.Errorf("error generating answer: %w", err)
			}
			printSourcesAndGaps(ans)
			return nil
		}

		ans, err := asst.Ask(ctx, question, nil)
		if err != nil {
			return fmt.Errorf("error generating answer: %w", err)
		}

		if cfg.JSONOutput {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(ans)
		}

		printFormattedAnswer(ans)
		return nil
	},
}

var chatCmd = &cobra.Command{
	Use:   "chat",
	Short: "Start an interactive conversation session with your knowledge base",
	RunE: func(cmd *cobra.Command, args []string) error {
		asst, db, err := initAssistant()
		if err != nil {
			return err
		}
		defer db.Close()

		session := conversation.NewSession()
		reader := bufio.NewReader(os.Stdin)

		fmt.Printf("=== md-ai Interactive Chat ===\n")
		fmt.Printf("Model: %s | Database: %s\n", cfg.Model, cfg.DBPath)
		fmt.Println("Type 'exit', 'quit', or press Ctrl+C to stop.")
		fmt.Println()

		ctx := cmd.Context()
		if ctx == nil {
			ctx = context.Background()
		}

		for {
			fmt.Print("You > ")
			line, err := reader.ReadString('\n')
			if err != nil {
				break
			}
			query := strings.TrimSpace(line)
			if query == "" {
				continue
			}
			if query == "exit" || query == "quit" {
				fmt.Println("Goodbye!")
				break
			}

			if streamOutput && !cfg.JSONOutput {
				fmt.Println()
				ans, err := runStreamingAsk(ctx, asst, query, session)
				if err != nil {
					fmt.Printf("[Error] %v\n\n", err)
					continue
				}
				printSourcesAndGaps(ans)
				fmt.Println()
				continue
			}

			ans, err := asst.Ask(ctx, query, session)
			if err != nil {
				fmt.Printf("[Error] %v\n\n", err)
				continue
			}

			fmt.Println()
			printFormattedAnswer(ans)
			fmt.Println()
		}
		return nil
	},
}

var gapsCmd = &cobra.Command{
	Use:   "gaps",
	Short: "Audit the indexed knowledge base for entity documentation gaps",
	RunE: func(cmd *cobra.Command, args []string) error {
		_, db, err := initAssistant()
		if err != nil {
			return err
		}
		defer db.Close()

		ctx := cmd.Context()
		if ctx == nil {
			ctx = context.Background()
		}

		gapsList, err := hybrid.AuditEntityGaps(ctx, db)
		if err != nil {
			return fmt.Errorf("failed to audit entity gaps: %w", err)
		}

		if cfg.JSONOutput {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(gapsList)
		}

		if len(gapsList) == 0 {
			fmt.Println("No entity documentation gaps detected.")
			return nil
		}
		fmt.Printf("Detected %d entity documentation gap(s):\n", len(gapsList))
		for _, g := range gapsList {
			fmt.Printf("- [%s] %s\n", g.Entity, g.Reason)
		}
		return nil
	},
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print md-ai version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("md-ai v0.5.0 (grounded RAG assistant)")
	},
}

func runStreamingAsk(ctx context.Context, asst *assistant.Assistant, question string, session *conversation.Session) (*model.Answer, error) {
	ans, err := asst.AskStream(
		ctx,
		question,
		session,
		func(plan *model.RetrievalPlanInfo, blocks int) {
			if plan != nil {
				fmt.Printf("[Plan: %s | Context blocks: %d]\n\n", plan.QueryType, blocks)
			}
		},
		func(token string) {
			fmt.Print(token)
		},
	)
	if err != nil {
		return nil, err
	}
	fmt.Println()
	return ans, nil
}

func printFormattedAnswer(ans *model.Answer) {
	fmt.Printf("%s\n", ans.Answer)
	printSourcesAndGaps(ans)
}

func printSourcesAndGaps(ans *model.Answer) {
	if len(ans.Sources) > 0 {
		fmt.Printf("\nSources:\n")
		for _, s := range ans.Sources {
			sec := ""
			if s.Section != "" {
				sec = "#" + s.Section
			}
			fmt.Printf("- %s%s\n", s.Path, sec)
			if s.Snippet != "" {
				fmt.Printf("  %s\n", s.Snippet)
			}
		}
	}

	if len(ans.Gaps) > 0 {
		fmt.Printf("\nKnowledge Gaps:\n")
		for _, g := range ans.Gaps {
			if g.Entity != "" && g.Entity != "general" {
				fmt.Printf("- [%s] %s\n", g.Entity, g.Reason)
			} else {
				fmt.Printf("- %s\n", g.Reason)
			}
		}
	}
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

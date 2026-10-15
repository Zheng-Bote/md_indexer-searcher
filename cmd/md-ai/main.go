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
	"github.com/md-indexer/internal/ai/model"
	"github.com/md-indexer/internal/ai/provider"
	"github.com/md-indexer/internal/ai/retriever"
	_ "modernc.org/sqlite"
)

var (
	cfg     = config.DefaultConfig()
	rootCmd = &cobra.Command{
		Use:   "md-ai",
		Short: "Grounded local AI assistant over Markdown knowledge bases",
		Long: `md-ai is a local, grounded AI retrieval-augmented generation (RAG) assistant
that queries Markdown notes indexed in SQLite knowledge.db using local LLMs (Ollama).`,
	}
)

func init() {
	cfg.LoadFromEnv()

	rootCmd.PersistentFlags().StringVar(&cfg.DBPath, "db", cfg.DBPath, "Path to SQLite knowledge database")
	rootCmd.PersistentFlags().StringVar(&cfg.OllamaHost, "ollama-host", cfg.OllamaHost, "Ollama API host URL")
	rootCmd.PersistentFlags().StringVarP(&cfg.Model, "model", "m", cfg.Model, "LLM model name (default: gemma3:1b)")
	rootCmd.PersistentFlags().Float64Var(&cfg.Temperature, "temperature", cfg.Temperature, "Generation temperature")
	rootCmd.PersistentFlags().IntVar(&cfg.MaxTokens, "max-tokens", cfg.MaxTokens, "Max generated tokens")
	rootCmd.PersistentFlags().IntVar(&cfg.MaxContextTokens, "max-context-tokens", cfg.MaxContextTokens, "Max context token budget")
	rootCmd.PersistentFlags().IntVar(&cfg.GraphDepth, "graph-depth", cfg.GraphDepth, "BFS graph traversal depth")
	rootCmd.PersistentFlags().IntVar(&cfg.TimeoutSeconds, "timeout", cfg.TimeoutSeconds, "LLM request timeout in seconds")
	rootCmd.PersistentFlags().BoolVar(&cfg.JSONOutput, "json", cfg.JSONOutput, "Output response in JSON format")

	rootCmd.AddCommand(askCmd)
	rootCmd.AddCommand(chatCmd)
	rootCmd.AddCommand(versionCmd)
}

func initAssistant() (*assistant.Assistant, *sql.DB, error) {
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
	ret := retriever.NewRetriever(db, cfg.GraphDepth)
	cb := contextbuilder.NewContextBuilder(cfg.MaxContextTokens)
	prov := provider.NewOllamaProvider(cfg.OllamaHost, &http.Client{Timeout: cfg.Timeout()})

	asst := assistant.NewAssistant(cfg, clf, ret, cb, prov)
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

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print md-ai version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("md-ai v0.1.0 (local grounded RAG assistant)")
	},
}

func printFormattedAnswer(ans *model.Answer) {
	fmt.Printf("%s\n", ans.Answer)

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
			fmt.Printf("- %s\n", g)
		}
	}
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

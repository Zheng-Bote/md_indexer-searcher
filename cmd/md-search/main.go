// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/md-indexer/internal/repository"
	"github.com/md-indexer/internal/search"
	"github.com/md-indexer/internal/vaultdb"
	"github.com/spf13/cobra"
)

var dbPath string
var vaultDir string
var jsonOutput bool

func main() {
	rootCmd := &cobra.Command{
		Use:   "md-search",
		Short: "CLI Knowledge Explorer",
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			if dbPath != "" {
				return nil
			}
			dataDir, err := vaultdb.UserDataDir()
			if err != nil {
				return err
			}
			if dbPath, err = vaultdb.ForReader(vaultDir, dataDir); err != nil {
				// A missing index is not a usage error; print only the message.
				cmd.SilenceUsage = true
			}
			return err
		},
	}

	rootCmd.PersistentFlags().StringVar(&dbPath, "db", "", "Path to SQLite database (default: index database of --vault)")
	rootCmd.PersistentFlags().StringVar(&vaultDir, "vault", ".", "Vault whose index database is used when --db is not set")
	rootCmd.PersistentFlags().BoolVar(&jsonOutput, "json", false, "Output as JSON")

	searchCmd := &cobra.Command{
		Use:   "search [query]",
		Short: "Full-text search",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			repo, _ := repository.NewSQLiteRepository(dbPath)
			se := search.NewSearchEngine(repo)
			res, _ := se.Search(context.Background(), args[0])

			if jsonOutput {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				enc.Encode(res)
			} else {
				fmt.Println("Result\tScore\tTitle")
				fmt.Println("--------------------------------")
				for i, r := range res {
					fmt.Printf("%d\t%.2f\t%s\n", i+1, r.Rank, r.Title)
				}
			}
		},
	}

	documentCmd := &cobra.Command{
		Use:   "document [title]",
		Short: "Show document details",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			repo, _ := repository.NewSQLiteRepository(dbPath)
			se := search.NewSearchEngine(repo)
			doc, _ := se.FindDocument(context.Background(), args[0])
			if doc != nil {
				fmt.Printf("Title: %s\nPath: %s\n", doc.Title, doc.Path)
			} else {
				fmt.Println("Document not found")
			}
		},
	}

	impactCmd := &cobra.Command{
		Use:   "impact [entity]",
		Short: "Impact analysis for an entity",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			repo, _ := repository.NewSQLiteRepository(dbPath)
			se := search.NewSearchEngine(repo)
			res, _ := se.Impact(context.Background(), args[0])

			if jsonOutput {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				enc.Encode(res)
			} else {
				fmt.Println("Source\tTarget\tRelation")
				for _, r := range res {
					fmt.Printf("%s\t%s\t%s\n", r.Source, r.Target, r.Relation)
				}
			}
		},
	}

	graphCmd := &cobra.Command{
		Use:   "graph [entity]",
		Short: "Dependency tree for an entity",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			repo, _ := repository.NewSQLiteRepository(dbPath)
			se := search.NewSearchEngine(repo)
			res, _ := se.Graph(context.Background(), args[0])

			if jsonOutput {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				enc.Encode(res)
			} else {
				fmt.Println("Source\tTarget\tRelation")
				for _, r := range res {
					fmt.Printf("%s\t%s\t%s\n", r.Source, r.Target, r.Relation)
				}
			}
		},
	}

	rootCmd.AddCommand(searchCmd, documentCmd, impactCmd, graphCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

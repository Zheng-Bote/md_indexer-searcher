package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/md-indexer/internal/repository"
	"github.com/md-indexer/internal/search"
	"github.com/spf13/cobra"
)

var dbPath string
var jsonOutput bool

func main() {
	rootCmd := &cobra.Command{
		Use:   "md-search",
		Short: "CLI Knowledge Explorer",
	}

	rootCmd.PersistentFlags().StringVar(&dbPath, "db", "index.db", "Path to SQLite database")
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
				fmt.Println("Result	Score	Title")
				fmt.Println("--------------------------------")
				for i, r := range res {
					fmt.Printf("%d	%.2f	%s\n", i+1, r.Rank, r.Title)
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

	rootCmd.AddCommand(searchCmd, documentCmd)
	
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

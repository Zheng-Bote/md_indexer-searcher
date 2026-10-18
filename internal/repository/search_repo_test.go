// SPDX-License-Identifier: Apache-2.0

package repository

import (
	"context"
	"testing"

	"github.com/md-indexer/internal/entities"
)

func TestSearchRepository_DocumentLookupAndLinkResolution(t *testing.T) {
	repo, err := NewSQLiteRepository(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteRepository: %v", err)
	}
	defer repo.Close()
	ctx := context.Background()

	docs := []*entities.Document{
		{ID: "d-docker", Path: "/vault/Docker/README.md", Title: "README", Markdown: "# Docker"},
		{ID: "d-mqtt", Path: "/vault/MQTT/README.md", Title: "README", Markdown: "# MQTT"},
		{ID: "d-a", Path: "/vault/A/README.md", Title: "README", Markdown: "# A"},
		{
			ID: "d-broker", Path: "/vault/MQTT/Broker.md", Title: "Broker",
			Markdown: "See [[readme]] and [[Missing]].",
			Links: []entities.Link{
				{SourceDocID: "d-broker", TargetDocID: "readme"},
				{SourceDocID: "d-broker", TargetDocID: "Missing"},
			},
		},
	}
	for _, d := range docs {
		if err := repo.UpsertDocument(ctx, d); err != nil {
			t.Fatalf("UpsertDocument %s: %v", d.Path, err)
		}
	}
	sr := NewSearchRepository(repo)

	t.Run("by path returns markdown and resolved links only", func(t *testing.T) {
		doc, err := sr.DocumentByPath(ctx, "/vault/MQTT/Broker.md")
		if err != nil || doc == nil {
			t.Fatalf("DocumentByPath: %v, %v", doc, err)
		}
		if doc.Markdown != "See [[readme]] and [[Missing]]." {
			t.Errorf("unexpected markdown %q", doc.Markdown)
		}
		want := []entities.ResolvedLink{{Target: "readme", Path: "/vault/MQTT/README.md"}}
		if len(doc.Links) != 1 || doc.Links[0] != want[0] {
			t.Errorf("expected same-folder resolution %v, got %v", want, doc.Links)
		}
	})

	t.Run("unknown path", func(t *testing.T) {
		doc, err := sr.DocumentByPath(ctx, "/etc/passwd")
		if err != nil || doc != nil {
			t.Errorf("expected nil for non-indexed path, got %v, %v", doc, err)
		}
	})

	tests := []struct {
		name, title, from, want string
	}{
		{"same folder wins", "README", "/vault/Docker/Notes.md", "/vault/Docker/README.md"},
		{"shortest path otherwise", "readme", "/vault/Other/Notes.md", "/vault/A/README.md"},
		{"unknown title", "Kubernetes", "/vault/Other/Notes.md", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := sr.DocumentByTitle(ctx, tt.title, tt.from)
			if err != nil {
				t.Fatalf("DocumentByTitle: %v", err)
			}
			got := ""
			if doc != nil {
				got = doc.Path
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

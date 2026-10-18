// SPDX-License-Identifier: Apache-2.0

package entities

import "time"

type Document struct {
	ID               string
	Path             string
	Title            string
	Content          string
	Hash             string
	ModifiedAt       time.Time
	IndexedAt        time.Time
	Sections         []Section
	Links            []Link
	Tags             []string
	Metadata         map[string]interface{}
	IsEntity         bool
	EntityType       string
	EntityName       string
	EntityProperties map[string]string
	EntityRelations  []EntityRelation
	// Markdown is the original file content, served by the document viewer.
	Markdown string `json:"-"`
	// HasMarkdown reports whether the stored index row already contains Markdown (set by GetDocumentByPath).
	HasMarkdown bool `json:"-"`
}
type Section struct {
	ID         string
	DocumentID string
	Heading    string
	Level      int
	Content    string
}
type Link struct {
	SourceDocID string
	TargetDocID string
}
type SearchResult struct {
	DocumentID string  `json:"document_id"`
	Path       string  `json:"path"`
	Title      string  `json:"title"`
	Snippet    string  `json:"snippet"`
	Rank       float64 `json:"rank"`
}

// DocumentView is an indexed document with its original Markdown and resolved outgoing wikilinks.
type DocumentView struct {
	ID       string         `json:"id"`
	Path     string         `json:"path"`
	Title    string         `json:"title"`
	Markdown string         `json:"markdown"`
	Links    []ResolvedLink `json:"links"`
}

// ResolvedLink maps a normalized wikilink target to the document it resolves to in the current vault.
type ResolvedLink struct {
	Target string `json:"target"`
	Path   string `json:"path"`
}

type Entity struct {
	ID         string            `json:"id"`
	Type       string            `json:"type"`
	Name       string            `json:"name"`
	Properties map[string]string `json:"properties"`
}

type EntityRelation struct {
	SourceID string `json:"source_id"`
	TargetID string `json:"target_id"`
	Type     string `json:"type"`
}

type GraphNode struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

type GraphEdge struct {
	Source   string `json:"source"`
	Target   string `json:"target"`
	Relation string `json:"relation"`
}

package entities
import "time"

type Document struct {
	ID         string
	Path       string
	Title      string
	Hash       string
	ModifiedAt time.Time
	IndexedAt  time.Time
	Sections   []Section
	Links      []Link
	Tags       []string
	Metadata   map[string]interface{}
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
	ID       string `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
}

type GraphEdge struct {
	Source   string `json:"source"`
	Target   string `json:"target"`
	Relation string `json:"relation"`
}

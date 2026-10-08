package parser

import (
	"os"
	"testing"
)

func TestParseFile(t *testing.T) {
	// Create temp markdown file
	content := `---
type: component
owner: robert
status: active
---
# Architecture
This is a test document with a link to [[Encryption]] and a tag #security.
`
	f, _ := os.CreateTemp("", "*.md")
	defer os.Remove(f.Name())
	f.WriteString(content)
	f.Close()

	p := NewParser()
	doc, err := p.ParseFile(f.Name())
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}

	if doc.Metadata["type"] != "component" {
		t.Errorf("Expected type metadata 'component', got %v", doc.Metadata["type"])
	}
}

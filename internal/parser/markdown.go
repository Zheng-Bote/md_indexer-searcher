package parser

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/md-indexer/internal/entities"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark-meta"
	"github.com/yuin/goldmark/ast"
	gmparser "github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

type Parser struct {
	md goldmark.Markdown
}

func NewParser() *Parser {
	return &Parser{
		md: goldmark.New(
			goldmark.WithExtensions(meta.Meta),
		),
	}
}

func (p *Parser) ParseFile(path string) (*entities.Document, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	stat, err := os.Stat(path)
	if err != nil {
		return nil, err
	}

	hashBytes := sha256.Sum256(content)
	hashStr := hex.EncodeToString(hashBytes[:])

	doc := &entities.Document{
		ID:         hashStr, // Simple ID generation, could be UUID or path hash
		Path:       path,
		Title:      filepath.Base(path),
		Hash:       hashStr,
		ModifiedAt: stat.ModTime(),
		IndexedAt:  time.Now(),
		Metadata:   make(map[string]interface{}),
	}

	p.extractAST(doc, content)
	return doc, nil
}

func (p *Parser) extractAST(doc *entities.Document, content []byte) {
	ctx := parserContext()
	node := p.md.Parser().Parse(text.NewReader(content), gmparser.WithContext(ctx))

	metaData := meta.Get(ctx)
	if metaData != nil {
		for k, v := range metaData {
			doc.Metadata[k] = v
		}
	}

	// Traverse AST for sections, links, and tags
	ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		// Simplistic extraction logic for sections, can be expanded
		if heading, ok := n.(*ast.Heading); ok {
			doc.Sections = append(doc.Sections, entities.Section{
				ID:      doc.ID + "-" + string(heading.Text(content)), // Simplify
				Heading: string(heading.Text(content)),
				Level:   heading.Level,
			})
		}

		if textNode, ok := n.(*ast.Text); ok {
			val := string(textNode.Segment.Value(content))
			if strings.HasPrefix(val, "[[") && strings.HasSuffix(val, "]]") {
				target := strings.TrimSuffix(strings.TrimPrefix(val, "[["), "]]")
				doc.Links = append(doc.Links, entities.Link{
					SourceDocID: doc.ID,
					TargetDocID: target, // Will need resolution
				})
			}
		}
		return ast.WalkContinue, nil
	})
}

func parserContext() gmparser.Context {
	return gmparser.NewContext()
}

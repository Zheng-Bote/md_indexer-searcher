package parser

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
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
	title := filepath.Base(path)
	title = strings.TrimSuffix(title, filepath.Ext(title))

	doc := &entities.Document{
		ID:               hashStr,
		Path:             path,
		Title:            title,
		Hash:             hashStr,
		ModifiedAt:       stat.ModTime(),
		IndexedAt:        time.Now(),
		Metadata:         make(map[string]interface{}),
		EntityProperties: make(map[string]string),
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
		
		// Entity Extraction Logic
		if typeVal, ok := doc.Metadata["type"].(string); ok && typeVal != "" {
			doc.IsEntity = true
			doc.EntityType = typeVal
			
			if nameVal, ok := doc.Metadata["name"].(string); ok && nameVal != "" {
				doc.EntityName = nameVal
			} else {
				doc.EntityName = doc.Title
			}

			for k, v := range doc.Metadata {
				if k == "type" || k == "name" { continue }
				
				switch val := v.(type) {
				case string:
					doc.EntityProperties[k] = val
				case int, int64, float64, bool:
					doc.EntityProperties[k] = fmt.Sprintf("%v", val)
				case []interface{}:
					// If it's a list, treat as relation (e.g. uses: [A, B])
					for _, item := range val {
						if targetStr, ok := item.(string); ok {
							// We use names as target IDs for relations for now, since we might not know their hash yet
							doc.EntityRelations = append(doc.EntityRelations, entities.EntityRelation{
								SourceID: doc.EntityName, // Temporarily use name as ID linking
								TargetID: targetStr,
								Type:     strings.ToUpper(k),
							})
						}
					}
				}
			}
		}
	}

		currentSectionIdx := -1

	ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if heading, ok := n.(*ast.Heading); ok && entering {
			headingText := string(heading.Text(content))
			doc.Sections = append(doc.Sections, entities.Section{
				ID:      doc.ID + "-" + headingText,
				Heading: headingText,
				Level:   heading.Level,
				Content: "",
			})
			currentSectionIdx = len(doc.Sections) - 1
			return ast.WalkContinue, nil
		}

		if textNode, ok := n.(*ast.Text); ok && entering {
			val := string(textNode.Segment.Value(content))
			
			if strings.HasPrefix(val, "[[") && strings.HasSuffix(val, "]]") {
				target := strings.TrimSuffix(strings.TrimPrefix(val, "[["), "]]")
				doc.Links = append(doc.Links, entities.Link{
					SourceDocID: doc.ID,
					TargetDocID: target,
				})
			}

			if currentSectionIdx >= 0 {
				doc.Sections[currentSectionIdx].Content += val + " "
			}
		}

		if _, ok := n.(*ast.Paragraph); ok && !entering {
			if currentSectionIdx >= 0 {
				doc.Sections[currentSectionIdx].Content += "\\n\\n"
			}
		}

		return ast.WalkContinue, nil
	})

	for i := range doc.Sections {
		doc.Sections[i].Content = strings.TrimSpace(doc.Sections[i].Content)
		doc.Content += doc.Sections[i].Heading + "\\n" + doc.Sections[i].Content + "\\n\\n"
	}
	doc.Content = strings.TrimSpace(doc.Content)
}

func parserContext() gmparser.Context {
	return gmparser.NewContext()
}

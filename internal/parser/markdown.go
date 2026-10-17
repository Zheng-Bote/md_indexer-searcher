// SPDX-License-Identifier: Apache-2.0

package parser

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/md-indexer/internal/entities"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark-meta"
	"github.com/yuin/goldmark/ast"
	gmparser "github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

var (
	wikilinkRegex   = regexp.MustCompile(`\[\[(.*?)\]\]`)
	tagRegex        = regexp.MustCompile(`(?:^|\s)#([a-zA-Z0-9_\-]+)`)
	inlineCodeRegex = regexp.MustCompile("`[^`]*`")
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
				if k == "type" || k == "name" {
					continue
				}

				switch val := v.(type) {
				case string:
					doc.EntityProperties[k] = val
				case int, int64, float64, bool:
					doc.EntityProperties[k] = fmt.Sprintf("%v", val)
				case []interface{}:
					// If it's a list, treat as relation (e.g. uses: [A, B])
					for _, item := range val {
						if targetStr, ok := item.(string); ok {
							target := normalizeLinkTarget(targetStr)
							if target == "" {
								continue
							}
							doc.EntityRelations = append(doc.EntityRelations, entities.EntityRelation{
								SourceID: doc.EntityName,
								TargetID: target,
								Type:     strings.ToUpper(k),
							})
						}
					}
				}
			}
		}
	}

	currentSectionIdx := -1
	ensureSection := func() int {
		if currentSectionIdx < 0 {
			doc.Sections = append(doc.Sections, entities.Section{
				ID:      doc.ID + "-overview",
				Heading: "Overview",
				Level:   1,
				Content: "",
			})
			currentSectionIdx = len(doc.Sections) - 1
		}
		return currentSectionIdx
	}

	ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if heading, ok := n.(*ast.Heading); ok && entering {
			headingText := string(heading.Text(content))
			p.extractLinksAndTags(doc, headingText)
			doc.Sections = append(doc.Sections, entities.Section{
				ID:      doc.ID + "-" + headingText,
				Heading: headingText,
				Level:   heading.Level,
				Content: "",
			})
			currentSectionIdx = len(doc.Sections) - 1
			return ast.WalkContinue, nil
		}

		if para, ok := n.(*ast.Paragraph); ok && entering {
			var sb strings.Builder
			for i := 0; i < para.Lines().Len(); i++ {
				line := para.Lines().At(i)
				sb.Write(line.Value(content))
			}
			paraText := sb.String()
			p.extractLinksAndTags(doc, paraText)

			idx := ensureSection()
			doc.Sections[idx].Content += paraText + "\n\n"
		}

		if fcb, ok := n.(*ast.FencedCodeBlock); ok && entering {
			var sb strings.Builder
			for i := 0; i < fcb.Lines().Len(); i++ {
				line := fcb.Lines().At(i)
				sb.Write(line.Value(content))
			}
			codeText := sb.String()

			idx := ensureSection()
			doc.Sections[idx].Content += "```\n" + codeText + "```\n\n"
		}

		if cb, ok := n.(*ast.CodeBlock); ok && entering {
			var sb strings.Builder
			for i := 0; i < cb.Lines().Len(); i++ {
				line := cb.Lines().At(i)
				sb.Write(line.Value(content))
			}
			codeText := sb.String()

			idx := ensureSection()
			doc.Sections[idx].Content += codeText + "\n\n"
		}

		if tb, ok := n.(*ast.TextBlock); ok && entering {
			var sb strings.Builder
			for i := 0; i < tb.Lines().Len(); i++ {
				line := tb.Lines().At(i)
				sb.Write(line.Value(content))
			}
			tbText := sb.String()
			p.extractLinksAndTags(doc, tbText)

			idx := ensureSection()
			doc.Sections[idx].Content += "- " + tbText + "\n"
		}

		return ast.WalkContinue, nil
	})

	for i := range doc.Sections {
		doc.Sections[i].Content = strings.TrimSpace(doc.Sections[i].Content)
		doc.Content += doc.Sections[i].Heading + "\n" + doc.Sections[i].Content + "\n\n"
	}
	doc.Content = strings.TrimSpace(doc.Content)
}

func (p *Parser) extractLinksAndTags(doc *entities.Document, rawText string) {
	// Strip inline code spans so backticked expressions like `[[which treefrog]]` or `#include` are ignored.
	cleanText := inlineCodeRegex.ReplaceAllString(rawText, " ")

	// Extract inline Wikilinks [[Target]], [[Target|Alias]], [[Target#Heading]]
	matches := wikilinkRegex.FindAllStringSubmatch(cleanText, -1)
	for _, m := range matches {
		if len(m) > 1 {
			target := normalizeLinkTarget(m[1])
			if target != "" {
				doc.Links = append(doc.Links, entities.Link{
					SourceDocID: doc.ID,
					TargetDocID: target,
				})
				if doc.IsEntity {
					doc.EntityRelations = append(doc.EntityRelations, entities.EntityRelation{
						SourceID: doc.EntityName,
						TargetID: target,
						Type:     "LINKS_TO",
					})
				}
			}
		}
	}

	// Extract inline hashtags #tag (ignoring purely numeric tags like #1)
	tagMatches := tagRegex.FindAllStringSubmatch(cleanText, -1)
	for _, tm := range tagMatches {
		if len(tm) > 1 {
			tag := strings.TrimSpace(tm[1])
			if tag != "" && !isAllDigits(tag) {
				found := false
				for _, existing := range doc.Tags {
					if existing == tag {
						found = true
						break
					}
				}
				if !found {
					doc.Tags = append(doc.Tags, tag)
				}
			}
		}
	}
}

// normalizeLinkTarget extracts the canonical note title from a wikilink or frontmatter relation target
// (e.g. "[[folder/MQTT Broker#Setup|Broker]]" -> "MQTT Broker").
func normalizeLinkTarget(raw string) string {
	s := strings.TrimSpace(raw)
	if strings.HasPrefix(s, "[[") && strings.HasSuffix(s, "]]") {
		s = strings.TrimSpace(s[2 : len(s)-2])
	}
	if idx := strings.Index(s, "|"); idx >= 0 {
		s = s[:idx]
	}
	if idx := strings.Index(s, "#"); idx >= 0 {
		s = s[:idx]
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if idx := strings.LastIndexAny(s, `/\`); idx >= 0 {
		s = s[idx+1:]
	}
	s = strings.TrimSuffix(s, ".md")
	return strings.TrimSpace(s)
}

func isAllDigits(s string) bool {
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return len(s) > 0
}

func parserContext() gmparser.Context {
	return gmparser.NewContext()
}

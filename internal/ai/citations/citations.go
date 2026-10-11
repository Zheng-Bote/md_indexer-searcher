// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

package citations

import (
	"regexp"
	"strings"

	"github.com/md-indexer/internal/ai/model"
)

var citationRegex = regexp.MustCompile(`\[Doc:\s*([^\]#]+)(?:#([^\]]+))?\]`)

// ExtractCitations extracts all `[Doc: <path>#<section>]` citations from the generated answer.
func ExtractCitations(text string) []model.SourceCitation {
	matches := citationRegex.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return nil
	}

	seen := make(map[string]bool)
	var citations []model.SourceCitation

	for _, m := range matches {
		path := strings.TrimSpace(m[1])
		sec := ""
		if len(m) > 2 {
			sec = strings.TrimSpace(m[2])
		}

		key := path + "#" + sec
		if !seen[key] {
			seen[key] = true
			citations = append(citations, model.SourceCitation{
				Path:    path,
				Section: sec,
			})
		}
	}

	return citations
}

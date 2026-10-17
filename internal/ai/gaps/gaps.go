// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

package gaps

import (
	"regexp"
	"strings"

	"github.com/md-indexer/internal/ai/model"
)

var gapIndicators = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(?:do not have|missing|unknown|not found in the knowledge base|insufficient information|undocumented)`),
	regexp.MustCompile(`(?i)(?:cannot find any reference to|no documentation exists for)`),
	regexp.MustCompile(`(?i)(?:keine information|nicht dokumentiert|fehlt|fehlen|nicht in der wissensbasis|unbekannt)`),
}

// DetectGaps analyzes the answer text and identifies reported knowledge gaps.
func DetectGaps(answer string) []model.KnowledgeGap {
	var out []model.KnowledgeGap
	lines := strings.Split(answer, "\n")

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		for _, re := range gapIndicators {
			if re.MatchString(trimmed) {
				out = append(out, model.KnowledgeGap{
					Entity: "general",
					Reason: trimmed,
				})
				break
			}
		}
	}

	return out
}

// MergeGaps combines retrieval-time and LLM-reported knowledge gaps without duplicates.
func MergeGaps(groups ...[]model.KnowledgeGap) []model.KnowledgeGap {
	var out []model.KnowledgeGap
	seen := make(map[string]bool)
	for _, grp := range groups {
		for _, g := range grp {
			key := strings.ToLower(strings.TrimSpace(g.Entity)) + "|" + strings.ToLower(strings.TrimSpace(g.Reason))
			if g.Reason == "" || seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, g)
		}
	}
	return out
}

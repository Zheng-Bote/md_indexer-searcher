// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

package gaps

import (
	"regexp"
	"strings"
)

var gapIndicators = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(?:do not have|missing|unknown|not found in the knowledge base|insufficient information|undocumented)`),
	regexp.MustCompile(`(?i)(?:cannot find any reference to|no documentation exists for)`),
}

// DetectGaps analyzes the answer text and identifies reported knowledge gaps.
func DetectGaps(answer string) []string {
	var gaps []string
	lines := strings.Split(answer, "\n")

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		for _, re := range gapIndicators {
			if re.MatchString(trimmed) {
				gaps = append(gaps, trimmed)
				break
			}
		}
	}

	return gaps
}

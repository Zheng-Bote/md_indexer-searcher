// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

package classifier

import (
	"regexp"
	"strings"

	"github.com/md-indexer/internal/ai/model"
)

// Classifier categorizes user queries into intents.
type Classifier interface {
	Classify(query string) model.Intent
}

// RuleClassifier uses keyword and regex heuristics to determine query intent.
type RuleClassifier struct{}

// NewRuleClassifier creates a new RuleClassifier.
func NewRuleClassifier() *RuleClassifier {
	return &RuleClassifier{}
}

// Patterns cover English and German phrasings. RE2's \b is ASCII-only, so German words that start
// with an umlaut are listed outside the \b group.
var (
	adrPattern          = regexp.MustCompile(`(?i)\b(adr|decision|decide|architecture decision|record|rationale|entscheidung\w*|begründung)\b`)
	impactPattern       = regexp.MustCompile(`(?i)\b(impact|break|affect|consequence|change|if we modify|what happens if|which .* (use|uses|depend on|depends on)|auswirkung\w*|betrifft|beeinflusst|folgen|was passiert wenn|welche .* (verwenden|verwendet|nutzen|nutzt|benutzen|hängen))\b|änder(n|ung)`)
	relationshipPattern = regexp.MustCompile(`(?i)\b(relationship|relates to|linked with|depends on|connects to|between .* and|beziehung\w*|zusammenhang|hängt .* ab|verbunden mit|zwischen .* und)\b`)
	comparePattern      = regexp.MustCompile(`(?i)\b(compare|versus|vs\.?|difference between|pros and cons|vergleich\w*|unterschied\w*|vor.? und nachteile)\b`)
	summaryPattern      = regexp.MustCompile(`(?i)\b(summarize|summary|overview|explain briefly|tl;?dr|zusammenfass\w*)\b|überblick`)
	gapPattern          = regexp.MustCompile(`(?i)\b(gap|missing|unclear|unknown|undocumented|what is not|what is missing|fehlt|fehlen\w*|lücke\w*|undokumentiert|unklar|unbekannt)\b`)
	entityPattern       = regexp.MustCompile(`(?i)\b(what is|who is|component|entity|service|module|was ist|wer ist|komponente\w*|dienst|modul\w*)\b`)
)

// Classify determines the user query's primary intent.
func (c *RuleClassifier) Classify(query string) model.Intent {
	q := strings.TrimSpace(query)
	if q == "" {
		return model.IntentSearch
	}

	if gapPattern.MatchString(q) {
		return model.IntentKnowledgeGap
	}
	if comparePattern.MatchString(q) {
		return model.IntentCompare
	}
	if adrPattern.MatchString(q) {
		return model.IntentADR
	}
	if impactPattern.MatchString(q) {
		return model.IntentImpact
	}
	if relationshipPattern.MatchString(q) {
		return model.IntentRelationship
	}
	if summaryPattern.MatchString(q) {
		return model.IntentSummary
	}
	if entityPattern.MatchString(q) {
		return model.IntentEntity
	}

	return model.IntentSearch
}

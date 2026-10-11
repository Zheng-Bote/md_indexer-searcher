// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

package classifier

import (
	"testing"

	"github.com/md-indexer/internal/ai/model"
)

func TestRuleClassifier_Classify(t *testing.T) {
	c := NewRuleClassifier()

	tests := []struct {
		query    string
		expected model.Intent
	}{
		{"What ADR exists for SQLite?", model.IntentADR},
		{"What is the architecture decision regarding modernc?", model.IntentADR},
		{"What is the impact if we change the parser?", model.IntentImpact},
		{"How does service A relates to service B?", model.IntentRelationship},
		{"Compare SQLite vs PostgreSQL", model.IntentCompare},
		{"Can you summarize the search engine docs?", model.IntentSummary},
		{"What is undocumented or missing in the security model?", model.IntentKnowledgeGap},
		{"What is the GoldmarkParser component?", model.IntentEntity},
		{"cgo dependency rules", model.IntentSearch},
		{"", model.IntentSearch},
	}

	for _, tt := range tests {
		got := c.Classify(tt.query)
		if got != tt.expected {
			t.Errorf("query %q: expected %s, got %s", tt.query, tt.expected, got)
		}
	}
}

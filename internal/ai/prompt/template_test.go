// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

package prompt

import (
	"strings"
	"testing"

	"github.com/md-indexer/internal/ai/model"
)

func TestBuildPrompt(t *testing.T) {
	sys, user := BuildPrompt("What is the DB?", "SQLite is used.", model.IntentADR)

	if !strings.Contains(sys, "Architecture Decision Records") {
		t.Errorf("expected ADR note in system prompt")
	}
	if !strings.Contains(user, "[Knowledge Context]") {
		t.Errorf("expected knowledge context in user prompt")
	}
	if !strings.Contains(user, "SQLite is used.") {
		t.Errorf("expected context text in user prompt")
	}
	if !strings.Contains(user, "What is the DB?") {
		t.Errorf("expected question in user prompt")
	}
}

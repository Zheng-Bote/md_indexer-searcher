// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

package conversation

import (
	"strings"
	"testing"
)

func TestSession(t *testing.T) {
	s := NewSession()
	s.AddTurn("What is the DB?", "It is SQLite.")

	msgs := s.Messages()
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}

	hist := s.FormatHistory()
	if !strings.Contains(hist, "User: What is the DB?") {
		t.Errorf("history missing user prompt")
	}
	if !strings.Contains(hist, "Assistant: It is SQLite.") {
		t.Errorf("history missing assistant reply")
	}

	s.Clear()
	if len(s.Messages()) != 0 {
		t.Errorf("expected 0 messages after clear")
	}
}

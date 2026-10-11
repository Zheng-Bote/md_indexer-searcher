// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

package citations

import (
	"testing"
)

func TestExtractCitations(t *testing.T) {
	text := `According to [Doc: docs/adr/001-sqlite.md#Context], we use SQLite.
Also see [Doc: docs/components/parser.md] for details.`

	citations := ExtractCitations(text)
	if len(citations) != 2 {
		t.Fatalf("expected 2 citations, got %d", len(citations))
	}

	if citations[0].Path != "docs/adr/001-sqlite.md" || citations[0].Section != "Context" {
		t.Errorf("unexpected citation 0: %+v", citations[0])
	}

	if citations[1].Path != "docs/components/parser.md" || citations[1].Section != "" {
		t.Errorf("unexpected citation 1: %+v", citations[1])
	}
}

func TestExtractCitations_Empty(t *testing.T) {
	citations := ExtractCitations("No citations here.")
	if len(citations) != 0 {
		t.Errorf("expected 0 citations, got %d", len(citations))
	}
}

// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

package gaps

import (
	"testing"
)

func TestDetectGaps(t *testing.T) {
	ans := `Based on the context, we use SQLite.
However, there is insufficient information regarding the backup strategy.
Also, no documentation exists for disaster recovery.`

	gaps := DetectGaps(ans)
	if len(gaps) != 2 {
		t.Fatalf("expected 2 gaps, got %d", len(gaps))
	}
}

func TestDetectGaps_NoGaps(t *testing.T) {
	ans := `Everything is fully documented in [Doc: docs/adr.md].`
	gaps := DetectGaps(ans)
	if len(gaps) != 0 {
		t.Errorf("expected 0 gaps, got %d", len(gaps))
	}
}

// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

package hybrid

// Source names used as candidate list keys and SourceType values.
const (
	SourceFTS    = "fts"
	SourceVector = "vector"
	SourceEntity = "entity"
	SourceGraph  = "graph"
)

// Candidate is a section proposed by one or more retrieval sources.
// Per-source scores are higher-is-better within their source; FinalScore is set by fusion.
type Candidate struct {
	SectionID  string
	DocumentID string

	Path    string
	Title   string
	Heading string
	Content string

	FTSScore    float64
	VectorScore float64
	EntityScore float64
	GraphScore  float64

	FinalScore float64

	SourceType string
}

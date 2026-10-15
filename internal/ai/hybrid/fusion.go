// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

package hybrid

import "sort"

// Weights scale each source's contribution to the fused score.
type Weights struct {
	FTS    float64
	Vector float64
	Graph  float64
	Entity float64
}

func (w Weights) of(source string) float64 {
	switch source {
	case SourceFTS:
		return w.FTS
	case SourceVector:
		return w.Vector
	case SourceGraph:
		return w.Graph
	case SourceEntity:
		return w.Entity
	}
	return 0
}

const (
	// rrfK damps the advantage of top ranks; 60 is the value from the original RRF paper.
	rrfK                   = 60
	maxSectionsPerDocument = 3
)

// Fuse merges ranked candidate lists with weighted Reciprocal Rank Fusion:
// FinalScore = Σ weight(source) / (rrfK + rank in source). Each list must be ordered best-first.
// Duplicate sections merge their per-source scores; SourceType becomes the largest contributor.
// At most maxSectionsPerDocument sections per document and maxResults candidates are returned.
func Fuse(lists map[string][]Candidate, w Weights, maxResults int) []Candidate {
	merged := make(map[string]*Candidate)
	best := make(map[string]float64)

	// Iterate sources in a fixed order so ties resolve deterministically.
	for _, source := range []string{SourceFTS, SourceVector, SourceEntity, SourceGraph} {
		weight := w.of(source)
		for rank, c := range lists[source] {
			contribution := weight / float64(rrfK+rank+1)
			m, ok := merged[c.SectionID]
			if !ok {
				cp := c
				cp.FinalScore = 0
				m = &cp
				merged[c.SectionID] = m
			}
			m.FinalScore += contribution
			m.FTSScore = max(m.FTSScore, c.FTSScore)
			m.VectorScore = max(m.VectorScore, c.VectorScore)
			m.EntityScore = max(m.EntityScore, c.EntityScore)
			m.GraphScore = max(m.GraphScore, c.GraphScore)
			if contribution > best[c.SectionID] {
				best[c.SectionID] = contribution
				m.SourceType = source
			}
		}
	}

	all := make([]*Candidate, 0, len(merged))
	for _, m := range merged {
		all = append(all, m)
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].FinalScore != all[j].FinalScore {
			return all[i].FinalScore > all[j].FinalScore
		}
		return all[i].SectionID < all[j].SectionID
	})

	perDoc := make(map[string]int)
	out := make([]Candidate, 0, maxResults)
	for _, c := range all {
		if len(out) >= maxResults {
			break
		}
		if perDoc[c.DocumentID] >= maxSectionsPerDocument {
			continue
		}
		perDoc[c.DocumentID]++
		out = append(out, *c)
	}
	return out
}

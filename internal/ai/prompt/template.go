// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

package prompt

import (
	"fmt"
	"strings"

	"github.com/md-indexer/internal/ai/model"
)

const baseSystemPrompt = `You are a helpful, precise, and strictly grounded AI assistant for a local Markdown knowledge base.
Follow these rules strictly:
1. Answer the question using ONLY the facts present in the provided [Knowledge Context] below.
2. If the context does not contain enough information to answer the question, clearly state: "I do not have sufficient information in the knowledge base to answer this question." and list what specific information is missing.
3. NEVER make up facts, URLs, components, or decisions not explicitly mentioned in the context.
4. Whenever referencing information from a source, cite it inline using the format: [Doc: <path>#<section>] or [Doc: <path>].
5. Keep your tone professional, concise, and direct.`

// BuildPrompt creates the full user prompt containing system instructions, context, and question.
func BuildPrompt(question string, contextText string, intent model.Intent) (systemPrompt string, userPrompt string) {
	intentNote := ""
	switch intent {
	case model.IntentADR:
		intentNote = "\nNote: The user is specifically asking about Architecture Decision Records (ADRs). Highlight rationale and consequences."
	case model.IntentImpact:
		intentNote = "\nNote: The user is asking about architectural impact. Highlight dependencies and downstream components."
	case model.IntentKnowledgeGap:
		intentNote = "\nNote: The user is explicitly querying for knowledge gaps. Identify missing documentation or unlinked components."
	}

	sys := baseSystemPrompt + intentNote

	var sb strings.Builder
	sb.WriteString("[Knowledge Context]\n")
	sb.WriteString(contextText)
	sb.WriteString("\n\n")
	sb.WriteString(fmt.Sprintf("[User Question]\n%s\n", question))

	return sys, sb.String()
}

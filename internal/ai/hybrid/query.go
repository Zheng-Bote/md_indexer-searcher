// SPDX-FileCopyrightText: 2026 Zheng-Bote
// SPDX-License-Identifier: Apache-2.0

package hybrid

import (
	"strings"
	"unicode"
)

// stopwords are frequent German and English words that carry no retrieval signal.
// Without them an OR query matches nearly every document ("wie", "ich", "the", ...).
var stopwords = toSet(`
aber alle allem allen aller alles als also am an ander andere anderen auch auf aus bei beim bin bis bist da damit dann
das dass dein deine dem den denn der des dich die dies diese diesem diesen dieser dieses dir doch dort du durch ein eine
einem einen einer eines er es etwas euch euer für gibt hab habe haben hat hatte hier ich ihr ihre im in ist ja jede jedem
jeden jeder jedes kann kein keine können mache machen macht man mein meine mich mir mit muss nach nicht nichts noch nun nur
ob oder ohne sehr sein seine sich sie sind so soll sollte sondern über um und uns unser unter vom von vor war waren warum
was weil welche welchem welchen welcher welches wenn wer werden wie wieso wir wird wo woher wohin zu zum zur zwischen
funktioniert funktionieren erkläre erklären bitte gibt's
a about all also an and any are as at be been but by can could did do does doing for from had has have how i if in into
is it its me my no not of on or our please should so some than that the their them then there these they this those to
too us was we were what when where which who whom why will with would you your explain work works working tell show
`)

func toSet(words string) map[string]bool {
	set := make(map[string]bool)
	for _, w := range strings.Fields(words) {
		set[w] = true
	}
	return set
}

// Terms splits a query like the FTS5 unicode61 tokenizer (on every non letter/digit rune),
// lowercases it, and drops stopwords and single-rune terms.
func Terms(query string) []string {
	parts := strings.FieldsFunc(strings.ToLower(query), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	seen := make(map[string]bool)
	var terms []string
	for _, t := range parts {
		if len([]rune(t)) < 2 || stopwords[t] || seen[t] {
			continue
		}
		seen[t] = true
		terms = append(terms, t)
	}
	return terms
}

// ftsMatch builds an FTS5 MATCH expression that ORs the quoted terms.
// Quoting keeps words like "and", "not" or "near" from being parsed as FTS operators.
func ftsMatch(terms []string) string {
	quoted := make([]string, len(terms))
	for i, t := range terms {
		quoted[i] = `"` + t + `"`
	}
	return strings.Join(quoted, " OR ")
}

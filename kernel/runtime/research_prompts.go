// SPDX-License-Identifier: MIT

// research_prompts.go owns the LLM-input prompt builders:
// buildResearchPlanPrompt, buildResearchSynthPrompt,
// buildRefutedWarning, and buildResearchVerifyPrompt.
// The parsers / extractors / regex patterns that consume
// the LLM output live in research_helpers.go.
package runtime

import (
	"fmt"
	"strings"
)

// buildResearchPlanPrompt renders the planning prompt.
func buildResearchPlanPrompt(question string, max int) string {
	return fmt.Sprintf("Question: %s\n\nReturn a JSON array of at most %d specific sub-questions whose "+
		"answers together fully address the question. If the question is already atomic, return a "+
		"one-element array.", question, max)
}

// buildResearchSynthPrompt renders the synthesis prompt with numbered sources.
func buildResearchSynthPrompt(question string, sources []ResearchSource) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Question: %s\n\nSources:\n", question)
	for _, s := range sources {
		title := s.Title
		if title == "" {
			title = s.URL
		}
		fmt.Fprintf(&b, "\n[%s] %s (%s)\n%s\n", s.ID, title, s.URL, s.Text)
	}
	b.WriteString("\nWrite the cited answer now. Every claim needs an [S#] citation, and use only these sources.")
	return b.String()
}

// buildRefutedWarning renders a Markdown callout appended to the answer when the
// adversarial pass refuted one or more cited claims, so the refutation travels
// with the synthesis text and not only in the separate claims list.
func buildRefutedWarning(refuted []ResearchClaim) string {
	var b strings.Builder
	b.WriteString("\n\n---\n\n> ⚠️ **Adversarial verification refuted the following claim(s)** — the cited source did not support them; treat them as unverified:\n>\n")
	for _, c := range refuted {
		b.WriteString("> - ")
		b.WriteString(strings.TrimSpace(c.Text))
		if n := strings.TrimSpace(c.Note); n != "" {
			b.WriteString(" — _")
			b.WriteString(n)
			b.WriteString("_")
		}
		b.WriteString("\n")
	}
	return b.String()
}

// parseSubQuestions extracts up to max sub-questions from a planner reply. It
// tolerates a JSON array (optionally inside a code fence) or a numbered/bulleted

// buildResearchVerifyPrompt renders the adversarial check for one claim against
// the text of the source(s) it cites.
func buildResearchVerifyPrompt(c ResearchClaim, byID map[string]ResearchSource) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Claim: %s\n\nCited source(s):\n", c.Text)
	found := false
	for _, id := range c.SourceIDs {
		s, ok := byID[id]
		if !ok {
			continue
		}
		found = true
		title := s.Title
		if title == "" {
			title = s.URL
		}
		fmt.Fprintf(&b, "\n[%s] %s (%s)\n%s\n", s.ID, title, s.URL, clip(s.Text, researchVerifyTextMax))
	}
	if !found {
		b.WriteString("\n(no cited source text available)\n")
	}
	b.WriteString("\nVerdict (SUPPORTED / REFUTED / UNCERTAIN) then one reason line:")
	return b.String()
}

// reSupportedWord matches "SUPPORTED" as a whole word so it never fires inside
// "UNSUPPORTED".

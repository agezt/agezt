// SPDX-License-Identifier: MIT

// Research: parsers (SubQuestions / SearchHits / Verdict) +
// extractors (JSONArray / CitedSources / Claims) +
// classifiers (Confidence / classifyResearchVerdict /
// firstIndexOf) + the regex variables each helper uses.
// The LLM-input prompt builders (4 buildXxx funcs) live
// in research_prompts.go. Code extracted from research.go
// during the Day-67 god-file split. Public API unchanged.
package runtime

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// list, always includes the original question, and dedupes case-insensitively.
func parseSubQuestions(raw, question string, max int) []string {
	raw = strings.TrimSpace(raw)
	var parsed []string
	if j := extractJSONArray(raw); j != "" {
		var arr []string
		if err := json.Unmarshal([]byte(j), &arr); err == nil {
			for _, s := range arr {
				if s = strings.TrimSpace(s); s != "" {
					parsed = append(parsed, s)
				}
			}
		}
	}
	if len(parsed) == 0 {
		for _, line := range strings.Split(raw, "\n") {
			s := strings.TrimSpace(line)
			s = strings.TrimLeft(s, "-*•0123456789.)\t ")
			s = strings.TrimSpace(s)
			if len([]rune(s)) >= 5 {
				parsed = append(parsed, s)
			}
		}
	}
	seen := map[string]bool{}
	final := make([]string, 0, max)
	for _, s := range append([]string{question}, parsed...) {
		s = strings.TrimSpace(s)
		key := strings.ToLower(s)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		final = append(final, s)
		if len(final) >= max {
			break
		}
	}
	if len(final) == 0 {
		return []string{question}
	}
	return final
}

// parseSearchHits parses the websearch tool's JSON output into hits with an
// absolute http(s) URL. Malformed output yields an empty slice, never an error.
func parseSearchHits(output string) []researchHit {
	var hits []researchHit
	if j := extractJSONArray(strings.TrimSpace(output)); j != "" {
		_ = json.Unmarshal([]byte(j), &hits)
	}
	out := make([]researchHit, 0, len(hits))
	for _, h := range hits {
		if strings.HasPrefix(h.URL, "http://") || strings.HasPrefix(h.URL, "https://") {
			out = append(out, h)
		}
	}
	return out
}

// extractJSONArray returns the substring from the first '[' to the last ']',
// or "" if there is no bracketed span.
func extractJSONArray(s string) string {
	i := strings.IndexByte(s, '[')
	j := strings.LastIndexByte(s, ']')
	if i >= 0 && j > i {
		return s[i : j+1]
	}
	return ""
}

var researchCiteRe = regexp.MustCompile(`\[S(\d+)\]`)

// extractCitedSources returns the sorted, distinct source indices (1..n) cited
// as [S#] in the synthesis. Out-of-range indices are ignored.
func extractCitedSources(markdown string, n int) []int {
	set := map[int]bool{}
	for _, m := range researchCiteRe.FindAllStringSubmatch(markdown, -1) {
		var idx int
		if _, err := fmt.Sscanf(m[1], "%d", &idx); err == nil && idx >= 1 && idx <= n {
			set[idx] = true
		}
	}
	out := make([]int, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Ints(out)
	return out
}

// researchConfidence is the fraction of gathered sources the answer cited,
// clamped to [0,1].
func researchConfidence(cited, total int) float64 {
	if total <= 0 {
		return 0
	}
	c := float64(cited) / float64(total)
	if c > 1 {
		c = 1
	}
	return c
}

// sentenceSplit breaks synthesis prose into candidate claim sentences. It splits
// on newlines and sentence terminators while keeping the terminator, and is
// deliberately simple — the verifier tolerates slightly ragged fragments.
var sentenceSplitRe = regexp.MustCompile(`(?:[.!?](?:\s|$)|\n)+`)

// extractClaims lifts cited sentences ([S#]) out of the synthesis into claims,
// each tagged with the in-range source IDs it references. Sentences without a
// citation, and a trailing "Confidence:" line, are skipped.
func extractClaims(markdown string, n int) []ResearchClaim {
	var claims []ResearchClaim
	for _, raw := range sentenceSplitRe.Split(markdown, -1) {
		s := strings.TrimSpace(raw)
		if s == "" {
			continue
		}
		low := strings.ToLower(s)
		if strings.HasPrefix(low, "confidence:") || strings.HasPrefix(low, "sources:") {
			continue
		}
		idx := extractCitedSources(s, n)
		if len(idx) == 0 {
			continue
		}
		ids := make([]string, 0, len(idx))
		for _, i := range idx {
			ids = append(ids, fmt.Sprintf("S%d", i))
		}
		text := s
		if !strings.HasSuffix(text, ".") {
			text += "."
		}
		claims = append(claims, ResearchClaim{Text: text, SourceIDs: ids})
	}
	return claims
}

// "UNSUPPORTED".
var reSupportedWord = regexp.MustCompile(`\bSUPPORTED\b`)

// parseResearchVerdict reads the verifier's reply into a normalized verdict and a
// short reason. The verdict is classified from the FIRST non-empty line ALONE:
// scanning the whole reply let a trailing reason such as "the source does not
// contradict it" flip a leading SUPPORTED to refuted.
func parseResearchVerdict(reply string) (verdict, note string) {
	reply = strings.TrimSpace(reply)
	verdict = "uncertain"
	for _, line := range strings.Split(reply, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		verdict = classifyResearchVerdict(line)
		break
	}
	// Reason = the first non-empty line that is not just the verdict word.
	for _, line := range strings.Split(reply, "\n") {
		l := strings.TrimSpace(line)
		if l == "" {
			continue
		}
		up := strings.ToUpper(l)
		if up == "SUPPORTED" || up == "REFUTED" || up == "UNCERTAIN" {
			continue
		}
		note = clip(l, 240)
		break
	}
	return verdict, note
}

// classifyResearchVerdict maps one verdict line to a normalized verdict. The
// LEFTMOST verdict keyword wins, so a reason clause on the same line (e.g.
// "SUPPORTED — not contradicted by the source") cannot override the verdict the
// model leads with. Refute signals are matched before bare "SUPPORTED" because
// "UNSUPPORTED"/"NOT SUPPORTED" contain it.
func classifyResearchVerdict(line string) string {
	up := strings.ToUpper(line)
	ref := firstIndexOf(up, "REFUTED", "UNSUPPORTED", "NOT SUPPORTED", "CONTRADICT")
	sup := -1
	if m := reSupportedWord.FindStringIndex(up); m != nil {
		sup = m[0]
	}
	switch {
	case ref >= 0 && (sup < 0 || ref < sup):
		return "refuted"
	case sup >= 0:
		return "supported"
	default:
		return "uncertain"
	}
}

// firstIndexOf returns the smallest index at which any of subs occurs in s, or
// -1 if none occur.
func firstIndexOf(s string, subs ...string) int {
	best := -1
	for _, sub := range subs {
		if i := strings.Index(s, sub); i >= 0 && (best < 0 || i < best) {
			best = i
		}
	}
	return best
}

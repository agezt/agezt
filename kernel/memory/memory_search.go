// SPDX-License-Identifier: MIT

package memory

// Memory search + scoring: Scored type + Search + keywordOverlap +
// recencyFactor + tokenize. Carved out of memory.go during the Day
// 192 god-file split so the main file can stay focused on the
// Record type + accessors + content addressing, and the store file
// can stay focused on the CRUD methods.
// Public API unchanged.

import (
	"strings"
	"unicode"
)

// Scored is a record paired with its retrieval score.
type Scored struct {
	Record Record  `json:"record"`
	Score  float64 `json:"score"`
}

// Search ranks the usable records in rs against query and returns the top
// `limit` by score (descending). A record scores on keyword overlap between
// the query tokens and the record's subject+content+tags, weighted by
// confidence and recency. Records with zero keyword overlap are excluded, as
// are tombstoned, superseded, suspended, and expired records.
//
// Ranking is a pure function of (rs, query, limit, nowMS): given the same
// inputs it returns the same ordering, with ties broken by LastSeenMS
// (newer first) then id — so callers and tests get stable output.
//
// nowMS is the reference time for the recency factor; callers pass the
// current wall clock (tests pass a fixed value).
func Search(rs []Record, query string, limit int, nowMS int64) []Scored {
	qTokens := tokenize(query)
	out := make([]Scored, 0, len(rs))
	if len(qTokens) == 0 {
		return out
	}
	for _, r := range rs {
		if !r.Usable(nowMS) {
			continue
		}
		overlap := keywordOverlap(qTokens, r)
		if overlap == 0 {
			continue
		}
		conf := r.Confidence
		if conf <= 0 {
			conf = 0.5 // never zero-out a hit purely on missing confidence
		}
		score := float64(overlap) * (0.5 + conf) * recencyFactor(r.LastSeenMS, nowMS)
		out = append(out, Scored{Record: r, Score: score})
	}
	sortScored(out)
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// keywordOverlap counts how many distinct query tokens appear anywhere in the
// record's searchable text (subject + content + tag keys/values — searchText,
// shared with the embedder so both signals see the same surface).
func keywordOverlap(qTokens []string, r Record) int {
	haystack := make(map[string]struct{})
	for _, t := range tokenize(searchText(r)) {
		haystack[t] = struct{}{}
	}
	n := 0
	seen := make(map[string]struct{}, len(qTokens))
	for _, t := range qTokens {
		if _, dup := seen[t]; dup {
			continue
		}
		seen[t] = struct{}{}
		if _, ok := haystack[t]; ok {
			n++
		}
	}
	return n
}

// recencyFactor decays linearly-ish with age: 1.0 for "now", asymptoting
// toward (but never reaching) 0 for old records. A record never drops out of
// retrieval on recency alone — it only loses ranking weight.
func recencyFactor(lastSeenMS, nowMS int64) float64 {
	ageMS := nowMS - lastSeenMS
	if ageMS <= 0 {
		return 1.0
	}
	const dayMS = 24 * 60 * 60 * 1000
	ageDays := float64(ageMS) / float64(dayMS)
	return 1.0 / (1.0 + ageDays)
}

// tokenize lowercases s and splits on any non-alphanumeric rune, dropping
// empties and 1-character tokens (noise). Stable and dependency-free.
func tokenize(s string) []string {
	fields := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
	out := fields[:0]
	for _, f := range fields {
		if len(f) <= 1 {
			continue
		}
		out = append(out, f)
	}
	return out
}


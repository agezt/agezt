// SPDX-License-Identifier: MIT

// websearch_helpers.go owns the parser-side plumbing: the regex
// variables used against the DuckDuckGo LITE markup, parseResults
// (link + snippet extraction), cleanURL / cleanText (HTML &
// whitespace normalisation), and the soft / err result formatters
// that turn parsed hits into agent.Result. The Tool struct +
// Invoke live in websearch.go.
package websearch

import (
	"encoding/json"
	"html"
	"net/url"
	"regexp"
	"strings"

	"github.com/agezt/agezt/kernel/agent"
)

// reLink matches the DuckDuckGo LITE result anchors; reSnippet matches the
// adjacent snippet cell. The lite markup is a plain table: the anchor carries
// href BEFORE a (single- or double-quoted) class='result-link', and the snippet
// is a <td class='result-snippet'>. Deliberately tolerant of quote style.
var (
	reLink    = regexp.MustCompile(`(?s)<a[^>]*href="([^"]+)"[^>]*class=['"]result-link['"][^>]*>(.*?)</a>`)
	reSnippet = regexp.MustCompile(`(?s)<td[^>]*class=['"]result-snippet['"][^>]*>(.*?)</td>`)
	reTag     = regexp.MustCompile(`<[^>]+>`)
	reSpace   = regexp.MustCompile(`\s+`)
)

// parseResults extracts up to limit hits from a DuckDuckGo lite result page.
func parseResults(body string, limit int) []Result {
	links := reLink.FindAllStringSubmatch(body, -1)
	snips := reSnippet.FindAllStringSubmatch(body, -1)
	out := make([]Result, 0, len(links))
	for i, m := range links {
		u := cleanURL(m[1])
		if u == "" {
			continue
		}
		title := cleanText(m[2])
		if title == "" {
			continue
		}
		snip := ""
		if i < len(snips) {
			snip = cleanText(snips[i][1])
		}
		out = append(out, Result{Title: title, URL: u, Snippet: snip})
		if len(out) >= limit {
			break
		}
	}
	return out
}

// cleanURL strips DuckDuckGo's redirect wrapper (//duckduckgo.com/l/?uddg=…)
// so the model gets the real destination URL.
func cleanURL(raw string) string {
	u := strings.TrimSpace(raw)
	if u == "" {
		return ""
	}
	if strings.HasPrefix(u, "//") {
		u = "https:" + u
	}
	if pu, err := url.Parse(u); err == nil {
		if real := pu.Query().Get("uddg"); real != "" {
			u = real
		}
	}
	if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
		return ""
	}
	return u
}

// cleanText strips HTML tags, unescapes entities, and collapses whitespace.
func cleanText(s string) string {
	s = reTag.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	s = reSpace.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

// softResult renders the {query, count, results, note?} payload the model
// receives. note carries a graceful "why empty" explanation when present; it
// never sets IsError, so a no-result search reads as a fact, not a failure.
func softResult(query string, results []Result, note string) agent.Result {
	if results == nil {
		results = []Result{}
	}
	out := map[string]any{
		"query":   query,
		"count":   len(results),
		"results": results,
	}
	if note != "" {
		out["note"] = note
	}
	enc, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return errResult("marshal: " + err.Error())
	}
	return agent.Result{
		Output:            string(enc),
		ObservationTrust:  agent.ObservationUntrusted,
		ObservationSource: "web_search:" + query,
	}
}

func errResult(msg string) agent.Result {
	return agent.Result{Output: "web_search: " + msg, IsError: true}
}

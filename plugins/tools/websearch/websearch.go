// SPDX-License-Identifier: MIT

// Package websearch is the in-process web-search tool. It runs a keyword
// query against a public search engine (DuckDuckGo's no-JS HTML endpoint)
// and returns the top results as structured {title, url, snippet} records —
// the capability that lets the agent *discover* a URL, not just fetch one it
// was already given (M627).
//
// Design notes:
//   - Keyless: DuckDuckGo's html endpoint needs no API key, so the tool works
//     out of the box with no operator secret.
//   - SSRF-guarded: the request goes through a netguard-protected client that
//     refuses internal/metadata addresses, exactly like the http/browser tools.
//   - Fail-soft: a network error or an unparseable page returns an empty result
//     set with a note, never a hard error — a flaky search must not fail a run.
//
// The engine host is fixed (the operator cannot point it at an arbitrary
// host), so the only operator-controlled input is the query string; that is
// why its capability (edict.CapWebSearch) is a low-risk network read.
package websearch

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	stdhttp "net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/netguard"
)

// DefaultTimeout caps a single search request.
const DefaultTimeout = 15 * time.Second

// maxResponseBytes caps the result page we parse.
const maxResponseBytes = 1 << 20 // 1 MiB

// DefaultLimit is how many results we return when the model doesn't ask for a
// specific count; MaxLimit caps it so one call can't flood the context.
const (
	DefaultLimit = 6
	MaxLimit     = 15
)

// engineURL is the no-JS DuckDuckGo LITE endpoint. Fixed by design — the
// operator never controls the target host, only the query. (M830: the older
// html.duckduckgo.com/html/ endpoint now answers bot GETs with an HTTP 202
// anti-bot challenge and no results; the lite endpoint still serves a plain,
// parseable results page.)
const engineURL = "https://lite.duckduckgo.com/lite/"

// Tool is the web_search implementation of agent.Tool.
type Tool struct {
	// HTTP overrides the default client. When nil, the tool builds a
	// netguard-protected client (default-deny to internal/metadata addresses)
	// honouring AllowLoopback/AllowPrivate. Setting it bypasses the guard.
	HTTP *stdhttp.Client
	// AllowLoopback / AllowPrivate relax the egress guard for the default
	// client. Default false — the search engine is public, so neither is needed
	// in normal use; they exist only for parity with the other network tools and
	// for tests that point engineURL at a local stub.
	AllowLoopback bool
	AllowPrivate  bool
	// UserAgent is sent on every request. A browser-like UA is the default
	// because the HTML endpoint serves a stripped/blocked page to obvious bots.
	UserAgent string
	// Endpoint overrides engineURL (tests point it at a local fixture server).
	Endpoint string
	// OnBlock, if set, is called (resolved IP, reason) when the egress guard
	// refuses a dial — wired by the daemon to journal a netguard.blocked event.
	OnBlock func(ip, reason string)
}

// DefaultUserAgent mimics a desktop browser; the engine returns a usable page
// for it where a generic client UA gets an empty/blocked response.
const DefaultUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) " +
	"AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"

// New returns a Tool with safe defaults: SSRF-guarded egress, 15s timeout,
// browser-like User-Agent.
func New() *Tool { return &Tool{UserAgent: DefaultUserAgent} }

func (t *Tool) client() *stdhttp.Client {
	if t.HTTP != nil {
		return t.HTTP
	}
	var opts []netguard.Option
	if t.AllowLoopback {
		opts = append(opts, netguard.AllowLoopback())
	}
	if t.AllowPrivate {
		opts = append(opts, netguard.AllowPrivate())
	}
	if t.OnBlock != nil {
		opts = append(opts, netguard.OnBlock(t.OnBlock))
	}
	return netguard.New(opts...).HTTPClient(DefaultTimeout)
}

// Definition implements agent.Tool.
func (t *Tool) Definition() agent.ToolDef {
	return agent.ToolDef{
		Name: "web_search",
		Description: "Search the web for a keyword query and return the top results " +
			"as a list of {title, url, snippet}. Use this to DISCOVER pages when you " +
			"don't already have a URL; then fetch the most relevant one with the " +
			"http or browser.read tool to read its contents.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "required": ["query"],
  "properties": {
    "query": {"type":"string", "description":"The search query (plain keywords)."},
    "limit": {"type":"integer", "description":"Max results to return (default 6, max 15)."}
  }
}`),
	}
}

type searchInput struct {
	Query string `json:"query"`
	Limit int    `json:"limit,omitempty"`
}

// Result is one parsed search hit.
type Result struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

// Invoke implements agent.Tool.
func (t *Tool) Invoke(ctx context.Context, raw json.RawMessage) (agent.Result, error) {
	var in searchInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return agent.Result{}, fmt.Errorf("web_search: parse input: %w", err)
	}
	q := strings.TrimSpace(in.Query)
	if q == "" {
		return errResult("query required"), nil
	}
	limit := in.Limit
	if limit <= 0 {
		limit = DefaultLimit
	}
	if limit > MaxLimit {
		limit = MaxLimit
	}

	endpoint := t.Endpoint
	if endpoint == "" {
		endpoint = engineURL
	}
	reqURL := endpoint + "?q=" + url.QueryEscape(q)
	req, err := stdhttp.NewRequestWithContext(ctx, stdhttp.MethodGet, reqURL, nil)
	if err != nil {
		return errResult("build request: " + err.Error()), nil
	}
	ua := t.UserAgent
	if ua == "" {
		ua = DefaultUserAgent
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Accept", "text/html")

	resp, err := t.client().Do(req)
	if err != nil {
		// Fail-soft: a flaky search must not fail the run.
		return softResult(q, nil, "search request failed: "+err.Error()), nil
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if resp.StatusCode >= 400 {
		return softResult(q, nil, fmt.Sprintf("search engine returned HTTP %d", resp.StatusCode)), nil
	}

	results := parseResults(string(body), limit)
	if len(results) == 0 {
		return softResult(q, nil, "no results found"), nil
	}
	return softResult(q, results, ""), nil
}

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
	return agent.Result{Output: string(enc)}
}

func errResult(msg string) agent.Result {
	return agent.Result{Output: "web_search: " + msg, IsError: true}
}

// SPDX-License-Identifier: MIT

package websearch

// This file owns the public Tool surface (New/Invoke/Definition + types
// + consts); the parser/cleaners/result-formatters live in
// websearch_helpers.go.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	stdhttp "net/http"
	"net/url"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/edict"
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

// Tool is the web_search implementation of toolapi.Tool.
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

// SetOnBlock installs the egress-guard audit callback (toolreg.NetguardAware).
func (t *Tool) SetOnBlock(fn func(ip, reason string)) { t.OnBlock = fn }

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

// Definition implements toolapi.Tool.
func (t *Tool) Definition() toolapi.ToolDef {
	return toolapi.ToolDef{
		Name:       "web_search",
		Capability: toolapi.ToolCapability{Name: string(edict.CapWebSearch)},
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
		Effect: toolapi.ToolEffect{
			Class: toolapi.EffectReversible,
			PredictedEffects: []string{
				"Send the query to the configured public search endpoint and return parsed result metadata.",
			},
			AffectedResources: []string{"web search endpoint", "network egress logs"},
			RollbackNotes:     "No local durable state is changed; the outbound search request cannot be unsent.",
			Confidence:        0.85,
		},
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

// Invoke implements toolapi.Tool.
func (t *Tool) Invoke(ctx context.Context, raw json.RawMessage) (toolapi.Result, error) {
	var in searchInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return toolapi.Result{}, fmt.Errorf("web_search: parse input: %w", err)
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

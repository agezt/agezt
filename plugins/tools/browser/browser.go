// SPDX-License-Identifier: MIT

// Package browser is the in-process web-reader tool. It fetches a URL
// over HTTP/HTTPS, strips scripts/styles, decodes HTML entities, and
// returns the visible text content to the model — turning "raw HTML
// the model has to parse" into "readable prose the model can quote."
//
// **Scope (M1.x).** Stdlib-only. No headless-browser binary, no
// JavaScript execution. This is a pragmatic "user agent" tool, not
// a Playwright bridge:
//
//   - Server-rendered pages (news articles, docs sites, GitHub,
//     blogs, Wikipedia) read cleanly.
//   - Single-page apps that defer rendering to client-side
//     JavaScript come back as a near-empty shell with a `<noscript>`
//     hint. The agent sees that and knows to fall back.
//
// A real headless-Chromium integration would unlock JS-rendered
// content but would require either (a) a multi-MB binary
// dependency, or (b) shelling out to operator-installed Chrome with
// platform-specific paths. Both violate the lean-deps policy. v2
// could add an opt-in `--render` mode that spawns chrome --headless
// when the operator has it; out of scope for v1.
//
// **Tool name.** Exposed as `browser.read` (not just `browser`) so
// the namespace stays open for future verbs like `browser.search`
// (search-engine query) or `browser.screenshot` (requires chrome).
package browser

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	stdhttp "net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/netguard"
)

// DefaultTimeout caps a single fetch.
const DefaultTimeout = 30 * time.Second

// DefaultMaxChars caps the text returned to the model. Most useful
// pages fit in 64KB of text; longer pages truncate with a clear
// marker so the agent knows it's incomplete.
const DefaultMaxChars = 64 * 1024

// MaxFetchBytes caps the raw HTML we'll download before truncating.
// 4MB is enough for almost any sensible page; the multiplier vs
// MaxChars accounts for HTML overhead (~10× tags+styles vs text).
const MaxFetchBytes = 4 * 1024 * 1024

// Tool is the browser tool implementation of agent.Tool.
type Tool struct {
	// AllowedHosts mirrors the http tool's semantics: case-insensitive
	// bare hostnames, "*.example.com" one-level wildcards. Empty +
	// AllowAll=false = default-deny per DECISIONS F2.
	AllowedHosts []string
	// AllowAll bypasses the host check (tests / trusted contexts).
	AllowAll bool
	// HTTP overrides the default client (tests use httptest.Client). When nil,
	// the tool builds a netguard-protected client (default-deny to internal /
	// metadata addresses) honouring AllowLoopback/AllowPrivate. Setting it
	// bypasses the guard — an explicit caller choice.
	HTTP *stdhttp.Client
	// AllowLoopback / AllowPrivate relax the egress guard for the default client
	// (loopback, RFC1918+ULA). Default false: even an allowlisted/AllowAll host
	// cannot reach internal addresses, so the agent can't read a page off the
	// metadata endpoint or a co-located service. Neither unblocks 169.254.0.0/16.
	AllowLoopback bool
	AllowPrivate  bool
	// OnBlock, if set, is called (resolved IP, reason) when the egress guard
	// refuses a dial — wired by the daemon to journal a netguard.blocked event
	// for audit (M109). Ignored when HTTP is injected.
	OnBlock func(ip, reason string)
	// UserAgent is sent on every request. A real browser-like value
	// reduces the chance of WAFs / CDN edge rules treating the agent
	// as a bot.
	UserAgent string
	// MaxChars caps the returned text. Zero falls back to DefaultMaxChars.
	MaxChars int
	// Cookies is an in-memory per-host cookie jar (M1.mm) shared
	// across every Invoke. Lets the agent follow login-then-read
	// flows (read the login form, POST creds via the http tool to
	// pick up Set-Cookie, then browser.read protected pages with
	// the session). nil → cookies disabled (forces stateless reads
	// — the default for back-compat).
	//
	// We use net/http's stdlib cookiejar.Jar via an interface here
	// so the field can be wired by the daemon after constructing
	// the Tool without the package needing to import the jar.
	Cookies stdhttp.CookieJar
}

// New returns a Tool with safe defaults (default-deny hosts, default-deny
// internal egress).
func New() *Tool {
	return &Tool{
		UserAgent: "Mozilla/5.0 (compatible; agezt-browser/0.1)",
		MaxChars:  DefaultMaxChars,
	}
}

// client returns the fetch client: the injected one if set, else a fresh
// netguard-protected client that refuses internal/metadata addresses on every
// hop (initial + redirects), relaxed by AllowLoopback/AllowPrivate.
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
	c := netguard.New(opts...).HTTPClient(DefaultTimeout)
	// Enforce the host allowlist on every redirect hop, not just the initial URL
	// (M254, mirroring the http tool's M251 fix). netguard blocks internal IPs on
	// each hop, but the host allowlist was checked only once — so an allowlisted
	// page that 302-redirects to an arbitrary external host would be fetched
	// anyway. Re-check per hop and cap the chain.
	c.CheckRedirect = func(req *stdhttp.Request, via []*stdhttp.Request) error {
		if len(via) >= maxRedirects {
			return fmt.Errorf("browser: stopped after %d redirects", maxRedirects)
		}
		if !t.AllowAll && !hostAllowed(req.URL.Host, t.AllowedHosts) {
			return fmt.Errorf("%w: %s (redirect target)", ErrHostDenied, req.URL.Host)
		}
		return nil
	}
	return c
}

// maxRedirects caps a single fetch's redirect chain. Matches Go's default; made
// explicit because setting CheckRedirect replaces that default.
const maxRedirects = 10

// EnableCookies attaches a fresh in-memory cookie jar to the tool
// (M1.mm). Wraps net/http/cookiejar so the daemon doesn't have to
// import it directly to enable session-following reads.
func (t *Tool) EnableCookies() error {
	jar, err := newDefaultJar()
	if err != nil {
		return err
	}
	t.Cookies = jar
	return nil
}

// Definition implements agent.Tool.
func (t *Tool) Definition() agent.ToolDef {
	return agent.ToolDef{
		Name: "browser.read",
		Description: "Fetch a web page and return its visible text content " +
			"(scripts, styles, and most markup stripped; HTML entities " +
			"decoded). Use this for reading articles, documentation, blog " +
			"posts, search results, and other server-rendered pages. " +
			"Single-page apps that render via JavaScript will return mostly " +
			"empty — fall back to a different source if so.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "required": ["url"],
  "properties": {
    "url": {"type":"string", "description":"Absolute http/https URL to fetch."},
    "max_chars": {"type":"integer", "description":"Optional truncation cap on returned text. Default 65536."}
  }
}`),
		Effect: agent.ToolEffect{
			Class: agent.EffectReversible,
			PredictedEffects: []string{
				"Fetch one allow-listed web page with HTTP GET and return visible text to the model.",
				"May send configured cookies to the target host when the browser cookie jar is enabled.",
			},
			AffectedResources: []string{"allowed browser.read hosts", "optional in-memory browser cookie jar"},
			RollbackNotes:     "Network reads cannot be unsent, but no local durable state is changed; clear the in-memory cookie jar to discard session carryover.",
			Confidence:        0.8,
		},
	}
}

type browserInput struct {
	URL      string `json:"url"`
	MaxChars int    `json:"max_chars,omitempty"`
}

// ErrHostDenied mirrors plugins/tools/http's sentinel.
var ErrHostDenied = errors.New("browser: host not in allowlist")

// Invoke implements agent.Tool.
func (t *Tool) Invoke(ctx context.Context, raw json.RawMessage) (agent.Result, error) {
	var in browserInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return agent.Result{}, fmt.Errorf("browser: parse input: %w", err)
	}
	if strings.TrimSpace(in.URL) == "" {
		return agent.Result{}, errors.New("browser: url required")
	}

	u, err := url.Parse(in.URL)
	if err != nil {
		return agent.Result{}, fmt.Errorf("browser: parse url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return agent.Result{}, fmt.Errorf("browser: scheme %q not allowed (only http/https)", u.Scheme)
	}
	if !t.AllowAll && !hostAllowed(u.Host, t.AllowedHosts) {
		return agent.Result{}, fmt.Errorf("%w: %s", ErrHostDenied, u.Host)
	}

	req, err := stdhttp.NewRequestWithContext(ctx, "GET", in.URL, nil)
	if err != nil {
		return agent.Result{}, fmt.Errorf("browser: build request: %w", err)
	}
	req.Header.Set("User-Agent", t.UserAgent)
	// Accept text/html primarily; some sites send JSON when they
	// see only application/* and that's harder to extract from.
	req.Header.Set("Accept", "text/html,application/xhtml+xml,*/*;q=0.8")

	client := t.client()
	// Per-Invoke cookie jar attach. Mutating the shared client's
	// Jar would be a data race across concurrent Invokes; instead
	// we make a shallow copy of the client when a jar is configured
	// and the client doesn't already carry one. The copy reuses
	// the same Transport so connection pooling carries over.
	if t.Cookies != nil && client.Jar == nil {
		shim := *client
		shim.Jar = t.Cookies
		client = &shim
	}
	resp, err := client.Do(req)
	if err != nil {
		return agent.Result{}, fmt.Errorf("browser: fetch: %w", err)
	}
	defer resp.Body.Close()

	// Cap raw download. Read up to MaxFetchBytes+1 to detect overflow,
	// then truncate cleanly.
	rawBody, err := io.ReadAll(io.LimitReader(resp.Body, MaxFetchBytes+1))
	if err != nil {
		return agent.Result{}, fmt.Errorf("browser: read body: %w", err)
	}
	truncatedRaw := false
	if len(rawBody) > MaxFetchBytes {
		rawBody = rawBody[:MaxFetchBytes]
		truncatedRaw = true
	}

	if resp.StatusCode/100 != 2 {
		// Non-2xx: surface the status as a tool error rather than
		// returning the error body as content; the agent should
		// react to the failure, not quote the error page.
		return agent.Result{}, fmt.Errorf("browser: HTTP %d from %s", resp.StatusCode, in.URL)
	}

	text := HTMLToText(string(rawBody))

	maxChars := t.MaxChars
	if maxChars <= 0 {
		maxChars = DefaultMaxChars
	}
	if in.MaxChars > 0 && in.MaxChars < maxChars {
		maxChars = in.MaxChars
	}
	truncatedText := false
	if len(text) > maxChars {
		// Back the cut up to a UTF-8 rune boundary so a multi-byte rune straddling
		// maxChars — common in non-English page text — is dropped whole rather than
		// split into invalid UTF-8 sent to the model.
		cut := maxChars
		for cut > 0 && !utf8.RuneStart(text[cut]) {
			cut--
		}
		text = text[:cut]
		truncatedText = true
	}

	// Bundle metadata + text into a single JSON object — the http
	// tool does the same so the model has a consistent shape across
	// network tools (status, content-type, body all in one place).
	if truncatedText {
		text += "\n\n…[truncated]"
	}
	out := map[string]any{
		"url":          in.URL,
		"status":       resp.StatusCode,
		"content_type": resp.Header.Get("Content-Type"),
		"raw_bytes":    len(rawBody),
		"text_chars":   len(text),
		"text":         text,
	}
	if truncatedRaw {
		out["truncated_raw"] = true
	}
	if truncatedText {
		out["truncated_text"] = true
	}
	enc, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return agent.Result{}, fmt.Errorf("browser: marshal result: %w", err)
	}
	return agent.Result{
		Output:            string(enc),
		ObservationTrust:  agent.ObservationUntrusted,
		ObservationSource: in.URL,
	}, nil
}

// hostAllowed reports whether host (with optional :port) matches
// any entry in allowed. Each entry may be a bare hostname or a
// "*.example.com" one-level wildcard. Case-insensitive. Duplicated
// from plugins/tools/http because keeping each tool self-contained
// is cheaper than building shared allowlist infrastructure for
// what's effectively the same eight-line check.
func hostAllowed(host string, allowed []string) bool {
	if len(allowed) == 0 {
		return false
	}
	// Drop port from host for matching.
	if i := strings.IndexByte(host, ':'); i >= 0 {
		host = host[:i]
	}
	host = strings.ToLower(host)
	for _, a := range allowed {
		a = strings.ToLower(strings.TrimSpace(a))
		if a == "" {
			continue
		}
		if strings.HasPrefix(a, "*.") {
			suffix := a[1:] // ".example.com"
			if strings.HasSuffix(host, suffix) && strings.Count(host, ".") == strings.Count(a, ".") {
				return true
			}
		} else if a == host {
			return true
		}
	}
	return false
}

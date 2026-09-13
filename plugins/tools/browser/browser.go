// SPDX-License-Identifier: MIT

// Package browser: web-reader tool — consts + Tool struct + New + SetOnBlock
// + client (the stdlib HTTP client builder) + EnableCookies + Definition +
// browserInput struct (the contract surface). Invoke (the SSRF-guarded
// fetch → strip → decode → truncate pipeline) + hostAllowed moved to
// browser_invoke.go. Day-211 god-file split. Public API unchanged.
package browser


import (
	"encoding/json"
	"errors"
	"fmt"
	stdhttp "net/http"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/netguard"
)
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

// SetOnBlock installs the egress-guard audit callback (toolreg.NetguardAware).
func (t *Tool) SetOnBlock(fn func(ip, reason string)) { t.OnBlock = fn }

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
		Name:       "browser.read",
		Capability: agent.ToolCapability{Name: string(edict.CapBrowserRead)},
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

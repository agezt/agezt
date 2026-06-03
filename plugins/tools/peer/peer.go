// SPDX-License-Identifier: MIT

// Package peer is the mesh delegation tool (ROADMAP P6-MULTI / M8): it lets one
// Agezt node hand a self-contained task to a *peer* Agezt node and get the
// answer back, by driving the peer's native REST surface
// (POST /api/v1/runs, kernel/restapi). This composes the REST API into a
// node-to-node primitive — cooperating Jarvis nodes, each governing its own runs.
//
// Peers are operator-configured (AGEZT_PEERS); the local agent only names which
// peer and what task. Because the call ships a task to an external node (an
// outward, side-effecting action), it is gated Ask-first by Edict
// (remote_run capability). The peer runs the task through its own governed loop
// — delegation does not bypass the peer's Edict/journal, and the returned
// correlation id makes the remote run auditable on that node.
package peer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/meshctx"
	"github.com/agezt/agezt/kernel/tenantctx"
)

// DefaultTimeout caps one remote run.
const DefaultTimeout = 5 * time.Minute

// DefaultCacheTTL is how long an auto-routing model-discovery result is reused
// before a peer is re-probed. Model inventories change rarely, so a short TTL makes
// repeated auto-routes cheap (no /models probe per call) while bounding staleness.
const DefaultCacheTTL = 60 * time.Second

// MaxAnswerBytes truncates a peer's answer so a runaway remote can't blow the
// context budget.
const MaxAnswerBytes = 60 * 1024

// Peer is a configured remote Agezt node.
type Peer struct {
	Name  string
	URL   string // base URL, e.g. http://host:8800 (no trailing /api/v1)
	Token string // Bearer token for the peer's REST API
}

// poster performs the HTTP POST to a peer; injectable for tests.
type poster func(ctx context.Context, endpoint, token string, body []byte) (status int, respBody []byte, err error)

// lister fetches a peer's routable model ids (GET /api/v1/models); injectable for
// tests. Used only for auto-routing — picking a peer for a requested model when the
// caller named none.
type lister func(ctx context.Context, p Peer) (models []string, err error)

// modelCacheEntry is a cached model-discovery result for one peer.
type modelCacheEntry struct {
	models []string
	at     time.Time
}

// Tool implements agent.Tool. Constructed only when at least one peer is
// configured; see New.
type Tool struct {
	Peers map[string]Peer
	// TenantPeers maps a tenant id to that tenant's own peer set (M219). A run carrying
	// a tenant id (stamped by its kernel via tenantctx) uses its tenant's set; a tenant
	// with no entry — and the primary — falls back to Peers. The fallback is to the
	// GLOBAL set, never another tenant's, so the mapping is leak-safe.
	TenantPeers map[string]map[string]Peer
	Timeout     time.Duration
	CacheTTL    time.Duration // auto-routing discovery cache TTL; <=0 uses DefaultCacheTTL
	post        poster
	list        lister
	now         func() time.Time // injectable clock for the cache (nil = time.Now)

	mu    sync.Mutex
	cache map[string]modelCacheEntry
}

// New builds a peer Tool from the global peer set. Returns nil when none are
// configured (tool disabled).
func New(peers map[string]Peer) *Tool {
	return NewWithTenants(peers, nil)
}

// NewWithTenants builds a peer Tool with a global peer set plus optional per-tenant
// overrides (M219). Returns nil only when BOTH are empty (tool disabled), so a
// deployment that configures peers only per-tenant still gets the tool.
func NewWithTenants(peers map[string]Peer, tenantPeers map[string]map[string]Peer) *Tool {
	if len(peers) == 0 && len(tenantPeers) == 0 {
		return nil
	}
	return &Tool{Peers: peers, TenantPeers: tenantPeers, post: httpPost, list: httpListModels, cache: map[string]modelCacheEntry{}}
}

// peersFor returns the peer set this run should route against: the run's tenant's own
// set when it has one, otherwise the global set. The fallback is always to the global
// set — never another tenant's — so a misattributed or absent tenant id can only ever
// degrade to the primary's peers, not leak a different tenant's (M219).
func (t *Tool) peersFor(ctx context.Context) map[string]Peer {
	if id := tenantctx.Tenant(ctx); id != "" {
		if tp, ok := t.TenantPeers[id]; ok && len(tp) > 0 {
			return tp
		}
	}
	return t.Peers
}

// clock returns the Tool's time source (time.Now unless overridden for tests).
func (t *Tool) clock() time.Time {
	if t.now != nil {
		return t.now()
	}
	return time.Now()
}

// cachedModels returns a peer's model ids, reusing a recent discovery result within
// the cache TTL so repeated auto-routes don't re-probe every peer. Errors are not
// cached (a transient discovery failure shouldn't suppress a later retry). The
// network call runs without the lock held so concurrent discoveries don't serialize.
func (t *Tool) cachedModels(ctx context.Context, p Peer) ([]string, error) {
	ttl := t.CacheTTL
	if ttl <= 0 {
		ttl = DefaultCacheTTL
	}
	now := t.clock()

	t.mu.Lock()
	if t.cache == nil {
		t.cache = map[string]modelCacheEntry{}
	}
	// Key the cache by URL, not name: across per-tenant peer sets the same name can
	// point at different nodes, so a name key could return another peer's models (M219).
	if e, ok := t.cache[p.URL]; ok && now.Sub(e.at) < ttl {
		models := e.models
		t.mu.Unlock()
		return models, nil
	}
	t.mu.Unlock()

	models, err := t.list(ctx, p)
	if err != nil {
		return nil, err
	}
	t.mu.Lock()
	t.cache[p.URL] = modelCacheEntry{models: models, at: now}
	t.mu.Unlock()
	return models, nil
}

func (t *Tool) Definition() agent.ToolDef {
	return agent.ToolDef{
		Name: "remote_run",
		Description: "Delegate a self-contained task to a PEER Agezt node and return its answer. " +
			"The peer runs the task through its own governed agent loop (its tools, its policy) and " +
			"reports back. Use to hand work to a node with different capabilities, data access, or " +
			"location. Optionally pin which model the peer should use with `model`; if you set " +
			"`model` but omit `peer`, a peer that serves that model is chosen automatically. " +
			"Available peers: " + peerNamesOf(t.Peers) + ".",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "properties": {
    "peer": {
      "type": "string",
      "description": "Which configured peer to run on. Omit to use the only peer when exactly one is configured."
    },
    "task": {
      "type": "string",
      "description": "The complete, self-contained instruction for the peer node."
    },
    "model": {
      "type": "string",
      "description": "Optional. Pin the model the peer routes this task to (must be one the peer can serve). Omit to let the peer use its own default model."
    }
  },
  "required": ["task"]
}`),
	}
}

// peerNamesOf renders a sorted, comma-joined list of peer names from a given set —
// used both for the static tool description (global set) and for error messages scoped
// to the run's effective set (M219).
func peerNamesOf(peers map[string]Peer) string {
	names := make([]string, 0, len(peers))
	for n := range peers {
		names = append(names, n)
	}
	sort.Strings(names)
	return strings.Join(names, ", ")
}

func (t *Tool) Invoke(ctx context.Context, input json.RawMessage) (agent.Result, error) {
	var in struct {
		Peer  string `json:"peer"`
		Task  string `json:"task"`
		Model string `json:"model"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return agent.Result{Output: "invalid input: " + err.Error(), IsError: true}, nil
	}
	task := strings.TrimSpace(in.Task)
	if task == "" {
		return agent.Result{Output: "task is required", IsError: true}, nil
	}
	// Mesh loop guard (M209): if this run is already at the hop limit, delegating
	// further would push the peer past it (and be refused there). Refuse locally with a
	// clear message rather than make a doomed round-trip. A non-delegated run is hop 0.
	if maxHops := meshctx.MaxHopsFromEnv(); meshctx.Hop(ctx) >= maxHops {
		return agent.Result{Output: fmt.Sprintf(
			"remote_run: mesh delegation hop limit (%d) reached — refusing to delegate further to avoid a federation loop",
			maxHops), IsError: true}, nil
	}
	model := strings.TrimSpace(in.Model)

	to := t.Timeout
	if to <= 0 {
		to = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, to)
	defer cancel()

	// Pick the candidate peer(s). A named peer is the sole candidate; otherwise, when a
	// model is requested and several peers are configured, auto-route across the peers
	// that serve it in order (M203).
	candidates, err := t.routeCandidates(ctx, strings.TrimSpace(in.Peer), model)
	if err != nil {
		return agent.Result{Output: err.Error(), IsError: true}, nil
	}

	// Forward the model only when the caller pinned one; an absent/empty model lets
	// the peer use its own default (the restapi runRequest falls back to it), so the
	// default behaviour is byte-for-byte unchanged.
	payload := map[string]string{"intent": task}
	if model != "" {
		payload["model"] = model
	}
	body, _ := json.Marshal(payload)

	var tried []string
	for i, peer := range candidates {
		endpoint := strings.TrimRight(peer.URL, "/") + "/api/v1/runs"
		status, respBody, perr := t.post(ctx, endpoint, peer.Token, body)
		if perr != nil {
			// Transport failure: no response means the task never ran on this peer, so
			// it is safe to fall back to the next serving peer (M206). A peer that
			// RESPONDS — even with an error status — is NOT retried elsewhere, since it
			// may already have executed side effects.
			tried = append(tried, peer.Name)
			if i+1 < len(candidates) {
				continue
			}
			if len(candidates) == 1 {
				return agent.Result{Output: fmt.Sprintf("remote_run: POST %s failed: %v", endpoint, perr), IsError: true}, nil
			}
			return agent.Result{Output: fmt.Sprintf("remote_run: all %d peers serving %q unreachable (%s); last error: %v",
				len(candidates), model, strings.Join(tried, ", "), perr), IsError: true}, nil
		}

		var resp struct {
			CorrelationID string `json:"correlation_id"`
			Status        string `json:"status"`
			Answer        string `json:"answer"`
			Error         string `json:"error"`
			Model         string `json:"model"`
		}
		_ = json.Unmarshal(respBody, &resp)

		if status < 200 || status >= 300 || resp.Status == "failed" {
			msg := resp.Error
			if msg == "" {
				msg = fmt.Sprintf("status %d", status)
			}
			out := fmt.Sprintf("remote_run on peer %q failed: %s", peer.Name, msg)
			if resp.CorrelationID != "" {
				out += fmt.Sprintf(" (peer correlation: %s)", resp.CorrelationID)
			}
			return agent.Result{Output: out, IsError: true}, nil
		}

		return agent.Result{Output: render(peer.Name, resp.Model, resp.CorrelationID, resp.Answer)}, nil
	}
	// routeCandidates returns a non-empty list or an error, so the loop always returns.
	return agent.Result{Output: "remote_run: no candidate peer", IsError: true}, nil
}

func render(peerName, model, corr, answer string) string {
	var b strings.Builder
	a := strings.TrimSpace(answer)
	if a == "" {
		b.WriteString("The peer returned no answer.")
	} else {
		b.WriteString(truncate(a, MaxAnswerBytes))
	}
	// The peer echoes the model it actually routed to; surface it so the delegating
	// node's transcript records which remote model produced the answer.
	if m := strings.TrimSpace(model); m != "" {
		fmt.Fprintf(&b, "\n\n[peer=%s model=%s correlation=%s]", peerName, m, corr)
	} else {
		fmt.Fprintf(&b, "\n\n[peer=%s correlation=%s]", peerName, corr)
	}
	return b.String()
}

// routeCandidates returns the ordered peer(s) the task may run on. A named peer is
// the sole candidate. With no name, a requested model, and more than one peer, it
// returns every peer that serves the model in name order (M203/M206) — the first is
// the primary, the rest are fallbacks used only if an earlier one is unreachable.
// Otherwise it falls back to resolve (sole-peer / ambiguous-name rules).
func (t *Tool) routeCandidates(ctx context.Context, name, model string) ([]Peer, error) {
	peers := t.peersFor(ctx) // the run's effective peer set (tenant override or global)
	if name == "" && model != "" && len(peers) > 1 {
		return t.serversForModel(ctx, peers, model)
	}
	p, err := resolve(peers, name)
	if err != nil {
		return nil, err
	}
	return []Peer{p}, nil
}

// serversForModel returns every peer in the given set that lists the requested model,
// in name order so the choice is deterministic. A peer that can't be reached for
// discovery is noted but doesn't abort the search; if no peer serves the model, the
// error names which peers were checked and which were unreachable.
func (t *Tool) serversForModel(ctx context.Context, peers map[string]Peer, model string) ([]Peer, error) {
	names := make([]string, 0, len(peers))
	for n := range peers {
		names = append(names, n)
	}
	sort.Strings(names)

	var servers []Peer
	var unreachable []string
	for _, n := range names {
		p := peers[n]
		models, err := t.cachedModels(ctx, p)
		if err != nil {
			unreachable = append(unreachable, n)
			continue
		}
		for _, m := range models {
			if m == model {
				servers = append(servers, p)
				break
			}
		}
	}
	if len(servers) == 0 {
		msg := fmt.Sprintf("remote_run: no configured peer serves model %q (checked: %s", model, peerNamesOf(peers))
		if len(unreachable) > 0 {
			msg += "; unreachable: " + strings.Join(unreachable, ", ")
		}
		return nil, fmt.Errorf("%s)", msg)
	}
	return servers, nil
}

// resolve picks the named peer from the given set, or the sole peer when name is empty.
func resolve(peers map[string]Peer, name string) (Peer, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		if len(peers) == 1 {
			for _, p := range peers {
				return p, nil
			}
		}
		return Peer{}, fmt.Errorf("remote_run: a peer name is required (configured: %s)", peerNamesOf(peers))
	}
	p, ok := peers[name]
	if !ok {
		return Peer{}, fmt.Errorf("remote_run: unknown peer %q (configured: %s)", name, peerNamesOf(peers))
	}
	return p, nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + fmt.Sprintf("\n… [truncated %d bytes]", len(s)-max)
}

// httpPost is the default poster: a JSON POST with a Bearer token.
func httpPost(ctx context.Context, endpoint, token string, body []byte) (int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	// Forward the delegation hop count +1 so the peer (and the chain beyond it) is
	// bounded against federation loops (M209).
	req.Header.Set(meshctx.HopHeader, strconv.Itoa(meshctx.Hop(ctx)+1))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	rb, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return resp.StatusCode, rb, nil
}

// httpListModels is the default lister: GET {url}/api/v1/models, returning the
// peer's routable model ids. The response is bounded-read (1 MiB) so a hostile peer
// can't exhaust memory during auto-routing discovery.
func httpListModels(ctx context.Context, p Peer) ([]string, error) {
	endpoint := strings.TrimRight(p.URL, "/") + "/api/v1/models"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	if p.Token != "" {
		req.Header.Set("Authorization", "Bearer "+p.Token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}
	var body struct {
		Models []string `json:"models"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body); err != nil {
		return nil, err
	}
	return body.Models, nil
}

// ParsePeers parses the AGEZT_PEERS spec: a comma-separated list of peers, each
// "name=url|token" (token optional). Whitespace is trimmed; the URL must be
// http(s). A malformed entry is a hard error so a misconfigured mesh is caught
// at startup.
func ParsePeers(spec string) (map[string]Peer, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil, nil
	}
	peers := map[string]Peer{}
	for _, raw := range strings.Split(spec, ",") {
		entry := strings.TrimSpace(raw)
		if entry == "" {
			continue
		}
		name, rest, ok := strings.Cut(entry, "=")
		name = strings.TrimSpace(name)
		if !ok || name == "" {
			return nil, fmt.Errorf("peer: entry %q must be name=url[|token]", entry)
		}
		urlStr, token, _ := strings.Cut(rest, "|")
		urlStr = strings.TrimSpace(urlStr)
		token = strings.TrimSpace(token)
		u, err := url.Parse(urlStr)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return nil, fmt.Errorf("peer %q: invalid URL %q (need http(s)://host…)", name, urlStr)
		}
		// A duplicate name would silently overwrite the earlier entry — you'd think
		// you had N peers but the mesh would only know N-1, and `remote_run` to the
		// shadowed name would hit the wrong node. Reject it so the misconfig is caught
		// at startup, like other malformed specs (M215).
		if _, dup := peers[name]; dup {
			return nil, fmt.Errorf("peer %q is defined more than once", name)
		}
		peers[name] = Peer{Name: name, URL: urlStr, Token: token}
	}
	return peers, nil
}

// ParseTenantPeers decodes the AGEZT_TENANT_PEERS spec (M219): a JSON object mapping a
// tenant id to that tenant's own AGEZT_PEERS-style spec, e.g.
//
//	{"alpha":"nodeA=http://a:8800|tokA","beta":"nodeB=https://b:8800"}
//
// Each value is parsed with ParsePeers, so the same validation (URL scheme, duplicate
// name) applies per tenant. A JSON object rather than per-tenant env vars keeps every
// tenant id expressible (including ones with characters an env-var name can't hold).
// Empty / whitespace-only spec → nil, no error. A tenant whose value parses to no peers
// is dropped (it would just fall back to the global set anyway).
func ParseTenantPeers(spec string) (map[string]map[string]Peer, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil, nil
	}
	var raw map[string]string
	if err := json.Unmarshal([]byte(spec), &raw); err != nil {
		return nil, fmt.Errorf("tenant peers: invalid JSON: %w", err)
	}
	out := map[string]map[string]Peer{}
	for tenant, peerSpec := range raw {
		tenant = strings.TrimSpace(tenant)
		if tenant == "" {
			return nil, fmt.Errorf("tenant peers: empty tenant id")
		}
		peers, err := ParsePeers(peerSpec)
		if err != nil {
			return nil, fmt.Errorf("tenant %q peers: %w", tenant, err)
		}
		if len(peers) > 0 {
			out[tenant] = peers
		}
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// Describe renders a one-line banner summary of the peers (tokens redacted).
func Describe(peers map[string]Peer) string {
	if len(peers) == 0 {
		return ""
	}
	names := make([]string, 0, len(peers))
	for n := range peers {
		names = append(names, n)
	}
	sort.Strings(names)
	parts := make([]string, 0, len(peers))
	for _, n := range names {
		p := peers[n]
		auth := ""
		if p.Token != "" {
			auth = " (token)"
		}
		parts = append(parts, fmt.Sprintf("%s→%s%s", n, p.URL, auth))
	}
	return fmt.Sprintf("%d peer(s): %s", len(peers), strings.Join(parts, ", "))
}

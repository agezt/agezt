// SPDX-License-Identifier: MIT
//
// Peer tool: types + lifecycle (NewWithTenants) + Definition + the
// peerNamesOf / peersFor / clock / cachedModels accessors.
// Extracted from peer.go during the Day-202 god-file split.
// Public API unchanged.
package peer

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/edict"
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
		Name:       "remote_run",
		Capability: agent.ToolCapability{Name: string(edict.CapRemoteRun)},
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
		Effect: agent.ToolEffect{
			Class: agent.EffectCompensable,
			PredictedEffects: []string{
				"POST a task to a configured peer Agezt node and wait for the remote governed run to finish.",
				"May execute side effects on the peer according to that peer's own tools and policy.",
			},
			AffectedResources: []string{"configured peer Agezt nodes", "remote run journal and remote tool surfaces"},
			RollbackNotes:     "Local fallback only retries peers before execution; after a peer accepts the run, compensation must happen on that peer using its correlation id.",
			Confidence:        0.55,
		},
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

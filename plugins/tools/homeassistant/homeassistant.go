// SPDX-License-Identifier: MIT

// Package homeassistant is the in-process Home Assistant control tool. Where the
// homeassistant CHANNEL is outbound-only (it pushes a brief to a notify service),
// this TOOL makes the smart home READABLE and ACTIONABLE from inside an agent run:
// the agent can read entity states ("is the living-room light on?", "what's the
// thermostat set to?") and call services ("turn the porch light off", "set the
// bedroom to 20°C"). It turns Agezt into something that can actually act on the
// house, not just announce into it.
//
// Two operations against the HA REST API (developers.home-assistant.io/docs/api/rest):
//
//   - get_states  → GET {base}/api/states[/{entity_id}]   (read; low risk)
//   - call_service→ POST {base}/api/services/{domain}/{service}  (actuate; physical)
//
// Security (SPEC-04 §1.7) — this tool touches the physical world, so it is
// fail-closed on two independent axes:
//
//   - The HA URL and token are OPERATOR-pinned config; the agent never supplies
//     the host, so there is no SSRF / arbitrary-egress surface (unlike the http
//     tool, which is why this tool needs no netguard — the destination is fixed).
//   - call_service is gated by a SERVICE allowlist (e.g. "light.turn_on",
//     "climate.*"): empty → no service is callable. get_states is gated by a READ
//     entity allowlist (e.g. "sensor.*", "light.living_room", "*"): empty → no
//     state is readable, and a bulk read is FILTERED to the allowlist so a
//     prompt-injected agent can't enumerate the whole house. The two axes map to
//     distinct Edict capabilities (homeassistant.read = Allow by default,
//     homeassistant.call = AskFirst), so an operator can let the agent read freely
//     while still confirming every actuation.
//
// The token is never logged. Response bodies are size-capped before the model
// sees them. The HTTP client is injectable so behaviour is unit-testable without
// a live Home Assistant.
package homeassistant

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/agent"
)

// DefaultTimeout caps a single HA request.
const DefaultTimeout = 30 * time.Second

// MaxResponseBytes truncates the response body the model receives. A full
// /api/states dump can be large; a bulk read past this cap fails to parse and
// returns guidance to query a specific entity instead of silently truncating.
const MaxResponseBytes = 256 * 1024

// Tool is the Home Assistant control tool (agent.Tool).
type Tool struct {
	// BaseURL is the Home Assistant root (e.g. "http://homeassistant.local:8123").
	BaseURL string
	// Token is a long-lived access token (Settings → Profile → Long-Lived Tokens).
	Token string
	// AllowedServices gates call_service. Each entry is "domain.service"
	// (e.g. "light.turn_on"), a "domain.*" whole-domain wildcard
	// (e.g. "climate.*"), or "*" for any service (DANGEROUS). Empty → no service
	// is callable (fail-closed).
	AllowedServices []string
	// ReadEntities gates get_states. Each entry is an "entity_id"
	// (e.g. "light.living_room"), a "domain.*" wildcard (e.g. "sensor.*"), or "*"
	// for any entity. Empty → no state is readable (fail-closed). A bulk read is
	// filtered to this list.
	ReadEntities []string
	// HTTP overrides the request client; nil → a DefaultTimeout client. The HA
	// host is config-pinned, so no egress guard is needed (the agent can't choose
	// the destination).
	HTTP *http.Client
}

// New returns a Tool with safe defaults (both allowlists empty = fail-closed).
func New() *Tool { return &Tool{} }

func (t *Tool) client() *http.Client {
	if t.HTTP != nil {
		return t.HTTP
	}
	return &http.Client{Timeout: DefaultTimeout}
}

// Definition implements agent.Tool. The description names which axes are enabled
// so the model doesn't attempt an operation that's fail-closed off.
func (t *Tool) Definition() agent.ToolDef {
	var enabled []string
	if len(t.ReadEntities) > 0 {
		enabled = append(enabled, "get_states (read "+strings.Join(t.ReadEntities, ", ")+")")
	}
	if len(t.AllowedServices) > 0 {
		enabled = append(enabled, "call_service ("+strings.Join(t.AllowedServices, ", ")+")")
	}
	avail := strings.Join(enabled, "; ")
	if avail == "" {
		avail = "(nothing enabled)"
	}
	return agent.ToolDef{
		Name: "homeassistant",
		Description: "Read smart-home entity states and call Home Assistant services to control the house " +
			"(lights, climate, switches, locks, …). Enabled here: " + avail + ". " +
			"You can only read allow-listed entities and call allow-listed services; anything else is refused.",
		InputSchema: json.RawMessage(`{
  "type": "object",
  "required": ["operation"],
  "properties": {
    "operation": {"type":"string","enum":["get_states","call_service"],"description":"get_states reads entity state; call_service performs an action."},
    "entity_id": {"type":"string","description":"For get_states: the entity to read (omit to read all allow-listed entities). For call_service: the target entity (e.g. light.living_room)."},
    "domain":    {"type":"string","description":"For call_service: the service domain (e.g. light, climate, switch, lock)."},
    "service":   {"type":"string","description":"For call_service: the service name (e.g. turn_on, turn_off, set_temperature)."},
    "data":      {"type":"object","description":"For call_service: extra service data (e.g. {\"brightness\":128} or {\"temperature\":20}). Merged with entity_id."}
  }
}`),
	}
}

type haInput struct {
	Operation string          `json:"operation"`
	EntityID  string          `json:"entity_id,omitempty"`
	Domain    string          `json:"domain,omitempty"`
	Service   string          `json:"service,omitempty"`
	Data      json.RawMessage `json:"data,omitempty"`
}

// Invoke implements agent.Tool.
func (t *Tool) Invoke(ctx context.Context, raw json.RawMessage) (agent.Result, error) {
	var in haInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return errResult("invalid input: " + err.Error()), nil
	}
	if strings.TrimSpace(t.BaseURL) == "" || strings.TrimSpace(t.Token) == "" {
		return errResult("homeassistant: base URL and token required"), nil
	}
	switch strings.ToLower(strings.TrimSpace(in.Operation)) {
	case "get_states":
		return t.getStates(ctx, in)
	case "call_service":
		return t.callService(ctx, in)
	case "":
		return errResult("operation required (get_states or call_service)"), nil
	default:
		return errResult("unknown operation " + in.Operation + " (want get_states or call_service)"), nil
	}
}

// getStates reads entity state(s), gated and (for bulk reads) filtered by the
// read allowlist.
func (t *Tool) getStates(ctx context.Context, in haInput) (agent.Result, error) {
	if len(t.ReadEntities) == 0 {
		return errResult("homeassistant: reading states is not enabled (no read allowlist configured)"), nil
	}
	base := strings.TrimRight(t.BaseURL, "/")

	if id := strings.TrimSpace(in.EntityID); id != "" {
		if !matchAllowed(t.ReadEntities, id) {
			return errResult(fmt.Sprintf("homeassistant: entity %q not in read allowlist (%s)", id, strings.Join(t.ReadEntities, ", "))), nil
		}
		body, status, err := t.do(ctx, http.MethodGet, base+"/api/states/"+url.PathEscape(id), nil)
		if err != nil {
			return errResult(err.Error()), nil
		}
		if status/100 != 2 {
			return errResult(fmt.Sprintf("homeassistant: get state %q returned status %d", id, status)), nil
		}
		return agent.Result{Output: string(body)}, nil
	}

	// Bulk read: fetch all, then FILTER to the allowlist so a non-allowed entity
	// is never surfaced (a prompt-injected agent can't enumerate the house).
	body, status, err := t.do(ctx, http.MethodGet, base+"/api/states", nil)
	if err != nil {
		return errResult(err.Error()), nil
	}
	if status/100 != 2 {
		return errResult(fmt.Sprintf("homeassistant: get states returned status %d", status)), nil
	}
	var all []map[string]any
	if err := json.Unmarshal(body, &all); err != nil {
		return errResult("homeassistant: could not parse states (the response may be too large — query a specific entity_id)"), nil
	}
	kept := make([]map[string]any, 0, len(all))
	for _, e := range all {
		id, _ := e["entity_id"].(string)
		if id != "" && matchAllowed(t.ReadEntities, id) {
			kept = append(kept, e)
		}
	}
	out, err := json.MarshalIndent(map[string]any{"count": len(kept), "states": kept}, "", "  ")
	if err != nil {
		return errResult("marshal: " + err.Error()), nil
	}
	return agent.Result{Output: string(out)}, nil
}

// callService actuates a Home Assistant service, gated by the service allowlist.
func (t *Tool) callService(ctx context.Context, in haInput) (agent.Result, error) {
	domain := strings.ToLower(strings.TrimSpace(in.Domain))
	service := strings.ToLower(strings.TrimSpace(in.Service))
	if domain == "" || service == "" {
		return errResult("homeassistant: call_service requires both domain and service"), nil
	}
	key := domain + "." + service
	if !matchAllowed(t.AllowedServices, key) {
		avail := strings.Join(t.AllowedServices, ", ")
		if avail == "" {
			avail = "(none — no service allowlist configured)"
		}
		return errResult(fmt.Sprintf("homeassistant: service %q not in allowlist; allowed: %s", key, avail)), nil
	}

	// Build the service-data body: the model's data object, with entity_id merged
	// in when supplied. entity_id always wins so the call targets exactly what the
	// agent named.
	payload := map[string]any{}
	if len(in.Data) > 0 {
		if err := json.Unmarshal(in.Data, &payload); err != nil {
			return errResult("homeassistant: invalid data object: " + err.Error()), nil
		}
	}
	if id := strings.TrimSpace(in.EntityID); id != "" {
		payload["entity_id"] = id
	}
	enc, err := json.Marshal(payload)
	if err != nil {
		return errResult("marshal: " + err.Error()), nil
	}

	base := strings.TrimRight(t.BaseURL, "/")
	endpoint := base + "/api/services/" + url.PathEscape(domain) + "/" + url.PathEscape(service)
	body, status, err := t.do(ctx, http.MethodPost, endpoint, enc)
	if err != nil {
		return errResult(err.Error()), nil
	}
	if status/100 != 2 {
		return errResult(fmt.Sprintf("homeassistant: call %s returned status %d", key, status)), nil
	}
	// HA returns the list of states it changed. Surface it (capped) so the model
	// can confirm the action took effect.
	return agent.Result{Output: fmt.Sprintf("called %s ok\n%s", key, string(body))}, nil
}

// do performs one request with the bearer token and a size-capped body read.
func (t *Tool) do(ctx context.Context, method, endpoint string, body []byte) ([]byte, int, error) {
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, r)
	if err != nil {
		return nil, 0, fmt.Errorf("homeassistant: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+t.Token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := t.client().Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("homeassistant: request: %w", err)
	}
	defer resp.Body.Close()
	out, err := io.ReadAll(io.LimitReader(resp.Body, MaxResponseBytes+1))
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("homeassistant: read response: %w", err)
	}
	if len(out) > MaxResponseBytes {
		out = out[:MaxResponseBytes]
	}
	return out, resp.StatusCode, nil
}

// matchAllowed reports whether target is permitted by patterns. A pattern is an
// exact dotted id ("light.turn_on"), a "domain.*" whole-domain wildcard, or "*"
// for anything. Matching is case-insensitive. The domain is the segment before
// the first dot.
func matchAllowed(patterns []string, target string) bool {
	target = strings.ToLower(strings.TrimSpace(target))
	if target == "" {
		return false
	}
	tdom := target
	if d, _, ok := strings.Cut(target, "."); ok {
		tdom = d
	}
	for _, p := range patterns {
		p = strings.ToLower(strings.TrimSpace(p))
		if p == "" {
			continue
		}
		if p == "*" || p == target {
			return true
		}
		if strings.HasSuffix(p, ".*") && p[:len(p)-2] == tdom {
			return true
		}
	}
	return false
}

// Capabilities returns a human-readable summary for the daemon banner /
// `agt status` (sorted for determinism). Empty axes are omitted.
func (t *Tool) Capabilities() string {
	var parts []string
	if n := len(t.ReadEntities); n > 0 {
		parts = append(parts, fmt.Sprintf("read=%d", n))
	}
	if n := len(t.AllowedServices); n > 0 {
		parts = append(parts, fmt.Sprintf("services=%d", n))
	}
	sort.Strings(parts)
	return strings.Join(parts, ", ")
}

func errResult(msg string) agent.Result {
	return agent.Result{Output: msg, IsError: true}
}

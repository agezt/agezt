// SPDX-License-Identifier: MIT

// Package http is the in-process HTTP tool. It performs GET/POST requests
// to a configured set of allowed hosts and returns the response status,
// headers, and body to the model.
//
// Scope (M1.a): GET/POST with a host-allowlist. Edict integration (TASKS
// P1-EDICT-01) will replace the local allowlist with a per-call policy
// decision when it lands; the tool keeps the allowlist as a defence in
// depth.
//
// Defaults: default-deny per DECISIONS F2 — the tool refuses every host
// unless either the AllowAll flag or a non-empty AllowedHosts list is
// configured. Schemes are restricted to http/https.
package http

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	stdhttp "net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/platform/netout"
)

// DefaultTimeout caps a single request.
const DefaultTimeout = 30 * time.Second

// MaxResponseBytes truncates the response body the model receives.
const MaxResponseBytes = 256 * 1024

// MaxRequestBodyBytes caps the request body the model can send.
const MaxRequestBodyBytes = 256 * 1024

// Tool is the http tool implementation of toolapi.Tool.
type Tool struct {
	// AllowedHosts is the case-insensitive set of bare hostnames the tool
	// will contact (no scheme, no path). "*.example.com" wildcards are
	// supported for one level of subdomain. Empty = no host allowed
	// (unless AllowAll is true).
	AllowedHosts []string
	// AllowAll bypasses the host check. Use only in trusted contexts
	// (tests, local development); production should rely on Edict.
	AllowAll bool
	// HTTP overrides the default client. When nil, the tool builds a
	// netguard-protected client (default-deny to internal/metadata addresses)
	// honouring AllowLoopback/AllowPrivate. Setting it bypasses the guard — an
	// explicit caller choice.
	HTTP *stdhttp.Client
	// AllowLoopback permits the default client to reach 127.0.0.0/8 and ::1
	// (a local sidecar/service). Default false: even an allowlisted or AllowAll
	// host cannot reach loopback, so the agent can't pivot into co-located
	// admin surfaces.
	AllowLoopback bool
	// AllowPrivate permits the default client to reach RFC1918 + IPv6 ULA (the
	// local network). Default false. Neither flag unblocks the cloud metadata
	// endpoint (169.254.169.254).
	AllowPrivate bool
	// UserAgent is sent on every request. Default: "agezt-http/0.1".
	UserAgent string
	// OnBlock, if set, is called (resolved IP, reason) whenever the egress
	// guard refuses a dial — the daemon wires it to journal a netguard.blocked
	// event for audit (M109). Ignored when HTTP is injected.
	OnBlock func(ip, reason string)
}

// New returns a Tool with safe defaults: default-deny hosts, default-deny
// internal egress (SSRF guard), 30s timeout.
func New() *Tool {
	return &Tool{UserAgent: "agezt-http/0.1"}
}

// SetOnBlock installs the egress-guard audit callback (toolreg.NetguardAware).
func (t *Tool) SetOnBlock(fn func(ip, reason string)) { t.OnBlock = fn }

// client returns the request client: the injected one if set, else a fresh
// netguard-protected client that refuses internal/metadata addresses on every
// hop (initial + redirects), relaxed by AllowLoopback/AllowPrivate.
func (t *Tool) client() *stdhttp.Client {
	if t.HTTP != nil {
		return t.HTTP
	}
	// The egress posture re-checks the allowlist on every redirect hop (M251):
	// an allowlisted host that 302s elsewhere must not carry the request — and
	// any Authorization header the agent set — to a host the operator never
	// allowed.
	return t.egress().Client(DefaultTimeout)
}

// egress is this tool's outbound posture (kernel/platform/netout).
func (t *Tool) egress() netout.Egress {
	return netout.Egress{
		AnyHost:       t.AllowAll,
		AllowedHosts:  t.AllowedHosts,
		AllowLoopback: t.AllowLoopback,
		AllowPrivate:  t.AllowPrivate,
		OnBlock:       t.OnBlock,
	}
}

// Definition implements toolapi.Tool.
func (t *Tool) Definition() toolapi.ToolDef {
	hosts := strings.Join(t.AllowedHosts, ", ")
	if t.AllowAll {
		hosts = "all hosts allowed by tool config"
	} else if hosts == "" {
		hosts = "none configured"
	}
	return toolapi.ToolDef{
		Name: "http",
		Capability: toolapi.ToolCapability{
			Name:  string(edict.CapHTTPGet),
			Field: "method",
			ByValue: map[string]string{
				"post": string(edict.CapHTTPPost),
			},
		},
		Description: "Fetch a URL (GET) or POST a JSON/text body to it. " +
			"Hosts must be in the tool's allowlist; otherwise the call is denied.",
		Effect: toolapi.ToolEffect{
			Class: toolapi.EffectCompensable,
			PredictedEffects: []string{
				"perform an outbound HTTP GET or POST to an allowed host",
				"POST requests may mutate remote state controlled by that service",
			},
			AffectedResources: []string{"allowed HTTP hosts: " + hosts},
			RollbackNotes:     "GET requests need no rollback. POST requests have no guaranteed rollback; compensate with a service-specific follow-up request if needed.",
			Confidence:        0.65,
		},
		InputSchema: json.RawMessage(`{
  "type": "object",
  "required": ["method","url"],
  "properties": {
    "method":   {"type":"string", "enum":["GET","POST"]},
    "url":      {"type":"string", "description":"Absolute http/https URL."},
    "headers":  {"type":"object", "additionalProperties":{"type":"string"}},
    "body":     {"type":"string", "description":"For POST: the request body (raw string)."},
    "content_type": {"type":"string", "description":"For POST: Content-Type header. Defaults to application/json."}
  }
}`),
	}
}

type httpInput struct {
	Method      string            `json:"method"`
	URL         string            `json:"url"`
	Headers     map[string]string `json:"headers,omitempty"`
	Body        string            `json:"body,omitempty"`
	ContentType string            `json:"content_type,omitempty"`
}

// ErrHostDenied is returned (as a tool error) when the requested host, or a
// redirect target, is outside the allowlist.
var ErrHostDenied = netout.ErrHostDenied

// Invoke implements toolapi.Tool.
func (t *Tool) Invoke(ctx context.Context, raw json.RawMessage) (toolapi.Result, error) {
	var in httpInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return toolapi.Result{}, fmt.Errorf("http: parse input: %w", err)
	}

	method := strings.ToUpper(strings.TrimSpace(in.Method))
	switch method {
	case "GET", "POST":
	case "":
		return errResult("method required (GET or POST)"), nil
	default:
		return errResult("method " + method + " not allowed in M1"), nil
	}

	u, err := url.Parse(in.URL)
	if err != nil {
		return errResult("invalid url: " + err.Error()), nil
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errResult("scheme must be http or https"), nil
	}
	if u.Host == "" {
		return errResult("url missing host"), nil
	}
	if !t.hostAllowed(u.Hostname()) {
		return errResult(fmt.Sprintf("http: %v: %s", ErrHostDenied, u.Hostname())), nil
	}

	if len(in.Body) > MaxRequestBodyBytes {
		return errResult(fmt.Sprintf("body too large: %d > %d", len(in.Body), MaxRequestBodyBytes)), nil
	}

	var bodyReader io.Reader
	if method == "POST" && in.Body != "" {
		bodyReader = bytes.NewReader([]byte(in.Body))
	}

	req, err := stdhttp.NewRequestWithContext(ctx, method, in.URL, bodyReader)
	if err != nil {
		return errResult("build request: " + err.Error()), nil
	}
	req.Header.Set("User-Agent", t.UserAgent)
	if method == "POST" {
		ct := in.ContentType
		if ct == "" {
			ct = "application/json"
		}
		req.Header.Set("Content-Type", ct)
	}
	for k, v := range in.Headers {
		req.Header.Set(k, v)
	}

	resp, err := t.client().Do(req)
	if err != nil {
		return errResult("http: " + err.Error()), nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxResponseBytes+1))
	if err != nil {
		return errResult("read body: " + err.Error()), nil
	}
	truncated := false
	if len(body) > MaxResponseBytes {
		body = body[:MaxResponseBytes]
		truncated = true
	}

	out := map[string]any{
		"status":      resp.StatusCode,
		"status_text": resp.Status,
		"headers":     flattenHeaders(resp.Header),
		"body":        string(body),
		"body_bytes":  len(body),
		"truncated":   truncated,
		"final_url":   resp.Request.URL.String(),
	}
	enc, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return errResult("marshal: " + err.Error()), nil
	}
	return toolapi.Result{
		Output:            string(enc),
		IsError:           resp.StatusCode >= 400,
		ObservationTrust:  toolapi.ObservationUntrusted,
		ObservationSource: resp.Request.URL.String(),
	}, nil
}

// hostAllowed applies the shared allowlist grammar (netout.Egress.HostAllowed).
func (t *Tool) hostAllowed(host string) bool { return t.egress().HostAllowed(host) }

// flattenHeaders converts http.Header to map[string]string by joining
// multi-value headers with commas (the model rarely needs them split).
func flattenHeaders(h stdhttp.Header) map[string]string {
	out := make(map[string]string, len(h))
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		out[k] = strings.Join(h.Values(k), ", ")
	}
	return out
}

func errResult(msg string) toolapi.Result {
	return toolapi.Result{Output: msg, IsError: true}
}

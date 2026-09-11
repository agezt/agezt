// SPDX-License-Identifier: MIT

package webui

// Server security/auth helpers: the host allowlist, hook allowlist,
// token-presented checks, and middleware chain. Carved out of webui.go
// during the Day 26 god file split #1 so the main file can focus on
// routing + asset serving.

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
	kernelauth "github.com/agezt/agezt/kernel/auth"
	"github.com/agezt/agezt/kernel/convo"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/httpserver"
)

func (s *Server) allowHook(key string) bool {
	now := time.Now().UnixMilli()
	s.hookRLMu.Lock()
	defer s.hookRLMu.Unlock()
	s.hookRLOnce.Do(func() { s.hookRL = make(map[string]*hookBucket) })

	b, ok := s.hookRL[key]
	if !ok {
		if len(s.hookRL) >= hookRLMaxBuckets {
			for k, v := range s.hookRL { // evict idle buckets; if none, drop one arbitrary
				if v.lastSeen < now-hookRLIdleEvictMs {
					delete(s.hookRL, k)
				}
			}
			if len(s.hookRL) >= hookRLMaxBuckets {
				for k := range s.hookRL {
					delete(s.hookRL, k)
					break
				}
			}
		}
		b = &hookBucket{windowEnd: now + 60_000}
		s.hookRL[key] = b
	}
	b.lastSeen = now
	if now >= b.windowEnd {
		b.count = 0
		b.windowEnd = now + 60_000
	}
	if b.count >= hookRatePerMin+hookRateBurst {
		return false
	}
	b.count++
	return true
}

// handleWorkflowHook accepts POST /hooks/<workflow-name> from external
// systems. The secret rides the X-Agezt-Secret header (or ?secret= for
// callers that can't set headers). A JSON body becomes
// {{trigger.payload.body}}; query params (minus secret) ride as
// {{trigger.payload.query}}. Responds 202 with the run's correlation id —
// the run itself proceeds async under the daemon's governance.
func (s *Server) handleWorkflowHook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/hooks/")
	if name == "" || strings.Contains(name, "/") {
		http.Error(w, "webhook refused", http.StatusNotFound)
		return
	}
	// Throttle before doing any work (VULN-007): cap fires per workflow+source so a
	// leaked secret can't be looped into unbounded paid runs. Keyed pre-auth on the
	// path + source IP, so a prober also can't burn budget probing secrets.
	if !s.allowHook(name + "|" + streamClientKey(r)) {
		w.Header().Set("Retry-After", "60")
		http.Error(w, "rate limited", http.StatusTooManyRequests)
		return
	}
	secret := r.Header.Get("X-Agezt-Secret")
	if secret == "" {
		secret = r.URL.Query().Get("secret")
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, webhookBodyCap+1))
	if err != nil || len(raw) > webhookBodyCap {
		http.Error(w, "body too large", http.StatusRequestEntityTooLarge)
		return
	}
	var body any
	if len(raw) > 0 {
		if json.Unmarshal(raw, &body) != nil {
			body = string(raw) // non-JSON bodies ride verbatim
		}
	}
	query := map[string]any{}
	for k, v := range r.URL.Query() {
		if k == "secret" || len(v) == 0 {
			continue
		}
		query[k] = v[0]
	}
	payload := map[string]any{"kind": "webhook", "body": body}
	if len(query) > 0 {
		payload["query"] = query
	}
	// Generous ctx: async hooks answer in milliseconds; reply-mode hooks
	// (M812) legitimately hold until the run finishes (2m control-plane cap).
	ctx, cancel := context.WithTimeout(r.Context(), 130*time.Second)
	defer cancel()
	res, err := s.client.Call(ctx, controlplane.CmdWorkflowWebhook, map[string]any{
		"ref": name, "secret": secret, "payload": payload,
	})
	if err != nil {
		// Post-auth run failures are honest (the caller knew the secret);
		// auth refusals stay uniform — never tell a prober WHY (unknown
		// name, bad secret, and disabled all read the same 403).
		if strings.Contains(err.Error(), "webhook run failed") {
			writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
			return
		}
		http.Error(w, "webhook refused", http.StatusForbidden)
		return
	}
	// Reply mode (M812): the run finished synchronously — hand its outputs
	// back to the caller with a 200.
	if outputs, ok := res["outputs"]; ok {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":             true,
			"workflow":       res["workflow"],
			"correlation_id": res["correlation_id"],
			"executed":       res["executed"],
			"outputs":        outputs,
		})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"accepted":       true,
		"workflow":       res["workflow"],
		"correlation_id": res["correlation_id"],
	})
}

// runStreamProxy is the Chat view's send button: it runs a free-text intent
// through the governed loop (controlplane.CmdRun) and streams the agent's events
// — llm tokens, tool calls, the final answer — straight to the browser as SSE, so
// the conversation renders live (like any chat UI). Unlike planRunProxy (which
// relays only a terminal result, leaning on the /events firehose), here each event
// IS the chat payload, so it's forwarded inline.
func (s *Server) runStreamProxy() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		args, ok := s.decodeAllowedBody(w, r, []string{"intent", "model", "history", "system", "agent", "execution_profile", "auto_approve_caps"})
		if !ok {
			return
		}
		intent, _ := args["intent"].(string)
		if strings.TrimSpace(intent) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "intent is required"})
			return
		}
		// Multi-turn continuity (M591): the Chat view sends the prior turns as
		// `history`; fold them with this turn into one transcript intent — the
		// same convo mapping the OpenAI-compatible API uses — so the governed loop
		// (single-intent by design) sees the whole conversation. `history` is
		// dropped from the args the control plane receives; CmdRun only ever sees
		// the resolved intent.
		turns := historyTurns(args["history"])
		delete(args, "history")
		if len(turns) > 0 {
			turns = append(turns, convo.Turn{Role: "user", Text: intent})
			args["intent"] = convo.TranscriptIntent(turns)
		}

		sse, ok := httpserver.StartSSE(w, r)
		if !ok {
			return
		}
		defer sse.Close()
		write := func(obj any) { _ = sse.WriteJSON(obj) }
		write(map[string]any{"kind": "open"})

		ctx, cancel := context.WithTimeout(r.Context(), planRunTimeout)
		defer cancel()
		res, err := s.client.Stream(ctx, controlplane.CmdRun, args, func(ev *event.Event) {
			write(map[string]any{
				"kind":           string(ev.Kind),
				"subject":        ev.Subject,
				"payload":        ev.Payload,
				"correlation_id": ev.CorrelationID,
			})
		})
		if err != nil {
			write(map[string]any{"kind": "error", "error": err.Error()})
			return
		}
		write(map[string]any{"kind": "done", "result": res})
	}
}

// toolInstallProxy is the CLI Toolbox install button (M956): it runs the host
// package manager for the requested tools via controlplane.CmdToolboxInstall and
// streams the per-tool progress events + final summary to the browser as SSE,
// exactly like runStreamProxy. Each event IS the install-progress payload, so
// it's forwarded inline. Only `names` is forwarded from the body.
func (s *Server) toolInstallProxy() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		args, ok := s.decodeAllowedBody(w, r, []string{"names"})
		if !ok {
			return
		}
		if len(stringList(args["names"])) == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "names (non-empty list) is required"})
			return
		}

		sse, ok := httpserver.StartSSE(w, r)
		if !ok {
			return
		}
		defer sse.Close()
		write := func(obj any) { _ = sse.WriteJSON(obj) }
		write(map[string]any{"kind": "open"})

		ctx, cancel := context.WithTimeout(r.Context(), planRunTimeout)
		defer cancel()
		res, err := s.client.Stream(ctx, controlplane.CmdToolboxInstall, args, func(ev *event.Event) {
			write(map[string]any{
				"kind":    string(ev.Kind),
				"subject": ev.Subject,
				"payload": ev.Payload,
			})
		})
		if err != nil {
			write(map[string]any{"kind": "error", "error": err.Error()})
			return
		}
		write(map[string]any{"kind": "done", "result": res})
	}
}

// marketStreamProxy installs or uninstalls a marketplace pack, streaming the
// per-item progress events (skill/mcp/tool) + final record to the browser as
// SSE — mirroring toolInstallProxy. Only the whitelisted keys are forwarded.
func (s *Server) marketStreamProxy(cmd string, keys []string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		args, ok := s.decodeAllowedBody(w, r, keys)
		if !ok {
			return
		}
		if strings.TrimSpace(toStr(args["name"])) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "name is required"})
			return
		}
		sse, ok := httpserver.StartSSE(w, r)
		if !ok {
			return
		}
		defer sse.Close()
		write := func(obj any) { _ = sse.WriteJSON(obj) }
		write(map[string]any{"kind": "open"})

		ctx, cancel := context.WithTimeout(r.Context(), planRunTimeout)
		defer cancel()
		res, err := s.client.Stream(ctx, cmd, args, func(ev *event.Event) {
			write(map[string]any{"kind": string(ev.Kind), "subject": ev.Subject, "payload": ev.Payload})
		})
		if err != nil {
			write(map[string]any{"kind": "error", "error": err.Error()})
			return
		}
		write(map[string]any{"kind": "done", "result": res})
	}
}

// toStr coerces a decoded JSON body value to a string (empty for non-strings).
func toStr(v any) string {
	s, _ := v.(string)
	return s
}

// stringList coerces a decoded JSON array of strings (body value) to []string.
func stringList(raw any) []string {
	list, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(list))
	for _, x := range list {
		if s, ok := x.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}

// maxHistoryTurns bounds how many prior turns the Chat view can fold into one
// run's intent, so a long thread can't grow the intent without limit. The most
// recent turns are kept (the tail carries the live context).
const maxHistoryTurns = 40

// historyTurns parses the optional `history` body field — a JSON array of
// {role, text} objects (as decoded into []any of map[string]any) — into convo
// turns, skipping malformed/blank entries and keeping only the most recent
// maxHistoryTurns. Returns nil when there is no usable history (the single-shot
// path, unchanged).
func historyTurns(raw any) []convo.Turn {
	list, ok := raw.([]any)
	if !ok || len(list) == 0 {
		return nil
	}
	turns := make([]convo.Turn, 0, len(list))
	for _, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		role, _ := m["role"].(string)
		text, _ := m["text"].(string)
		if strings.TrimSpace(role) == "" || strings.TrimSpace(text) == "" {
			continue
		}
		turns = append(turns, convo.Turn{Role: role, Text: text})
	}
	if len(turns) > maxHistoryTurns {
		turns = turns[len(turns)-maxHistoryTurns:]
	}
	return turns
}

// secure applies the defensive response headers (CSP et al.) to a handler but
// does NOT require a token. Used for public, secret-free static subresources
// (the bundle + favicon) that the browser must be able to load without a token.
func (s *Server) secure(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		setSecurityHeaders(w)
		if !s.hostAllowed(r.Host) {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		if !sameOriginMutation(r) {
			http.Error(w, "forbidden origin", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

// shellAuth gates the SPA shell: the token always opens it, and when a console
// password is configured (M933) the shell is served credential-free too — it
// has to be, or a token-less browser could never reach the login screen. The
// shell is compiled UI code with no data; route policy still guards every data
// route. Host/Origin checks and security headers are applied outside the router.
// With no password configured a token-less visit gets a hint page instead of a
// bare "unauthorized", pointing at the banner URL / password setup.
func (s *Server) shellAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.tokenPresented(r) && s.consolePassword() == "" {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte("unauthorized — open the console via the tokened URL from the daemon banner,\n" +
				"or set a console password (Config Center → Interfaces → Web UI password, or AGEZT_WEB_PASSWORD)\n" +
				"to enable password login at this address.\n"))
			return
		}
		next(w, r)
	}
}

// setSecurityHeaders applies defensive response headers to every web UI route
// (set before the auth check so even 401s carry them). This is a control surface:
//   - X-Frame-Options DENY — the dashboard has state-mutating controls
//     (approve/halt/resume/decide), so framing is denied to block clickjacking.
//   - Referrer-Policy no-referrer — the page URL carries the auth token in
//     `?token=`, so the referrer is suppressed to keep it out of any Referer header.
//   - X-Content-Type-Options nosniff — stop content-type sniffing/confusion.
//   - Content-Security-Policy (static): the SPA loads only external, same-origin
//     hashed JS/CSS, so `script-src 'self'` admits the genuine bundle and refuses
//     any inline/injected script — STRICTER than the old per-nonce scheme (which
//     existed to allow one inline block). `style-src 'self' 'unsafe-inline'` is
//     required because React Flow / Radix inject runtime inline styles (measured
//     transforms) that can't be hashed at build time; 'unsafe-inline' on
//     style-src enables no code execution. `connect-src 'self'` confines fetch +
//     EventSource to the daemon; the rest closes framing/exfil/pivot avenues.
func setSecurityHeaders(w http.ResponseWriter) {
	h := w.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("Content-Security-Policy",
		"default-src 'none'; "+
			"script-src 'self'; "+
			"style-src 'self' 'unsafe-inline'; "+
			"connect-src 'self'; "+
			"img-src 'self' data:; "+
			"font-src 'self' data:; "+
			"base-uri 'none'; "+
			"form-action 'none'; "+
			"frame-ancestors 'none'")
}

func (s *Server) hostAllowed(hostport string) bool {
	host := hostName(hostport)
	if host == "" {
		return false
	}
	lower := strings.ToLower(host)
	if lower == "localhost" {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return !ip.IsUnspecified()
	}
	s.hostPolicyMu.RLock()
	defer s.hostPolicyMu.RUnlock()
	return s.allowedHosts[lower]
}

func sameOriginMutation(r *http.Request) bool {
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	}
	if strings.EqualFold(r.Header.Get("Sec-Fetch-Site"), "cross-site") {
		return false
	}
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	return strings.EqualFold(canonicalHostPort(u.Host), canonicalHostPort(r.Host))
}

func hostName(hostport string) string {
	h := strings.TrimSpace(hostport)
	if h == "" {
		return ""
	}
	if strings.HasPrefix(h, "[") {
		if end := strings.Index(h, "]"); end > 0 {
			return strings.TrimSuffix(h[1:end], ".")
		}
	}
	if host, _, err := net.SplitHostPort(h); err == nil {
		return strings.TrimSuffix(strings.Trim(host, "[]"), ".")
	}
	if strings.Count(h, ":") == 1 {
		if host, _, ok := strings.Cut(h, ":"); ok {
			return strings.TrimSuffix(host, ".")
		}
	}
	return strings.TrimSuffix(strings.Trim(h, "[]"), ".")
}

func canonicalHostPort(hostport string) string {
	h := strings.TrimSpace(strings.ToLower(hostport))
	if h == "" {
		return ""
	}
	if host, port, err := net.SplitHostPort(h); err == nil {
		return strings.TrimSuffix(strings.Trim(host, "[]"), ".") + ":" + port
	}
	return strings.TrimSuffix(strings.Trim(h, "[]"), ".")
}

// tokenPresented reports whether the request carries the valid console token.
// Shell/deep-link requests still accept ?token= because the daemon banner URL
// is how an operator first opens the console.
func (s *Server) tokenPresented(r *http.Request) bool {
	return s.tokenPresentedFrom(r, true)
}

func (s *Server) dataTokenPresented(r *http.Request) bool {
	// /events uses the ephemeral SSE token (minted once at startup) in the
	// query string instead of the main console token, because the browser's
	// EventSource API cannot set custom headers (VULN query-string-token).
	// Other data routes default to Bearer header / session cookie; the
	// allowQueryTokensForData legacy flag preserves ?token= for test support.
	if r.URL.Path == "/events" {
		if s.sseTokenMatch(r.URL.Query().Get("st")) || s.tokenPresentedFrom(r, false) {
			return true
		}
		// Fallback for programmatic / non-browser SSE clients that pass the
		// main token in query (before the SPA was updated). Accept the main
		// token in query ONLY for /events and ONLY as a transition aid.
		return s.tokenMatch(r.URL.Query().Get("token"))
	}
	allowQuery := s.allowQueryTokensForData
	return s.tokenPresentedFrom(r, allowQuery)
}

func (s *Server) tokenPresentedFrom(r *http.Request, allowQuery bool) bool {
	if s.token == "" {
		return false // never serve without a configured token
	}
	if allowQuery {
		if s.tokenMatch(r.URL.Query().Get("token")) {
			return true
		}
	}
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return s.tokenMatch(strings.TrimPrefix(h, "Bearer "))
	}
	return false
}

// authorized gates a DATA route. Default (M933): the token OR a password
// session opens it — the password is an alternative door, so a token-less
// browser that logged in with the console password works. Strict mode
// (AGEZT_WEB_PASSWORD_STRICT=on) restores M817 compose: token AND session,
// for operators who exposed the console beyond loopback and want two factors.
// With no password configured, the token alone suffices (pre-M817 behaviour).
func (s *Server) authorized(r *http.Request) bool {
	pw := s.consolePassword()
	if pw == "" {
		return s.dataTokenPresented(r)
	}
	if s.passwordStrictOn() {
		return s.dataTokenPresented(r) && s.sessionValid(r)
	}
	return s.dataTokenPresented(r) || s.sessionValid(r)
}

// tokenMatch compares a presented token against the configured one in CONSTANT
// TIME, so an attacker who can reach the web UI can't recover the token
// byte-by-byte by timing the auth check. Mirrors the control-plane's
// shared auth verifier. Caller guarantees s.token != "".
func (s *Server) tokenMatch(presented string) bool {
	return s.tokenAuth != nil && s.tokenAuth.Authorize(presented, kernelauth.TierAdmin)
}

// sseTokenMatch compares a presented token against the ephemeral SSE token
// (minted once at startup). Used for the /events ?st= parameter — the browser's
// EventSource API cannot set custom headers, so the SSE token travels in the
// query string through a loopback-bound surface with no third party exposure.
func (s *Server) sseTokenMatch(presented string) bool {
	return s.sseAuth != nil && s.sseAuth.Authorize(presented, kernelauth.TierAdmin)
}

// handleSPA serves the embedded React single-page app. The hashed JS/CSS live
// under /assets/ (see handleAssets); this serves index.html at "/" and for any
// other non-API, non-asset path (client-side deep links like /runs), so a
// refresh on a sub-view re-loads the app rather than 404-ing. index.html is
// served no-cache so a daemon upgrade (new asset hashes) is picked up
// immediately rather than showing a stale shell that points at gone assets.

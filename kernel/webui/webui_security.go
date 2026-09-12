// SPDX-License-Identifier: MIT

// WebUI security handlers: allowHook + handleWorkflowHook + runStreamProxy + toolInstallProxy + marketStreamProxy.
// Code extracted from webui_security.go during the Day-75 god-file split. Public API unchanged.
package webui


import (
	"context"
	"encoding/json"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/convo"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/httpserver"
	"io"
	"net/http"
	"strings"
	"time"
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
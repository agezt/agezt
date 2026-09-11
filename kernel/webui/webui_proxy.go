// SPDX-License-Identifier: MIT

package webui

// Proxy handlers (the /api/* bridge that forwards read/write commands
// from the SPA to the control plane). Carved out of webui.go during the
// Day 26 god file split #2 so the main file can focus on routing +
// lifecycle.

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/httpserver"
)

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	// Subscribe before opening the stream so a subscribe failure is a plain
	// 500, not a broken SSE body. StartSSE bounds concurrent firehose streams
	// per client (V-009) — 429 when a client is over its generous cap.
	sub, err := s.bus.Subscribe(">", 256)
	if err != nil {
		http.Error(w, "subscribe: "+err.Error(), http.StatusInternalServerError)
		return
	}
	defer sub.Cancel()
	sse, ok := httpserver.StartSSE(w, r)
	if !ok {
		return
	}
	defer sse.Close()
	// An initial comment opens the stream so the browser's EventSource fires
	// onopen even before the first event.
	_ = sse.Comment("connected")

	ctx := r.Context()
	// A heartbeat keeps proxies from closing an idle stream.
	ping := time.NewTicker(20 * time.Second)
	defer ping.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ping.C:
			if err := sse.Comment("ping"); err != nil {
				return
			}
		case ev, ok := <-sub.C:
			if !ok {
				return
			}
			payload, err := json.Marshal(ev)
			if err != nil {
				continue
			}
			if err := sse.WriteData(string(payload)); err != nil {
				return
			}
		}
	}
}

// proxy returns a handler that runs one read-only control-plane command and
// relays its JSON result verbatim.
func (s *Server) proxy(cmd string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		res, err := s.client.Call(ctx, cmd, nil)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, res)
	}
}

// numericQueryArgs are the route arg keys whose control-plane handlers expect
// JSON numbers (argLimit/argFloat64/scheduleArgNumber all reject strings). A
// query string only carries text, so the query-arg proxies coerce these keys
// before forwarding; a value that doesn't parse rides through as the string so
// the server's "must be a number" error still names the real problem.
var numericQueryArgs = map[string]bool{
	"limit":           true,
	"since_ms":        true,
	"min_cost_mc":     true,
	"max_cost_mc":     true,
	"offset":          true,
	"older_than_days": true,
	"count":           true,
}

func queryArgValue(k, v string) any {
	if numericQueryArgs[k] {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return v
}

// readArgsProxy returns a GET handler for one allowlisted READ command that
// takes query arguments. It forwards only the route's allowlisted args (so the
// browser cannot pass arbitrary parameters) and relays the JSON result. Unlike
// writeProxy it permits GET, because the command is read-only.
func (s *Server) readArgsProxy(rr writeRoute) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		args := map[string]any{}
		for _, k := range rr.args {
			if v := strings.TrimSpace(r.URL.Query().Get(k)); v != "" {
				args[k] = queryArgValue(k, v)
			}
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		res, err := s.client.Call(ctx, rr.cmd, args)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, res)
	}
}

// writeProxy returns a handler for one allowlisted mutating command. It is
// POST-only (a GET — e.g. a prefetch or an <img> — must never halt the agent),
// copies the route's allowed args from the query string, and relays the result.
func (s *Server) writeProxy(wr writeRoute) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST required"})
			return
		}
		args := map[string]any{}
		for _, k := range wr.args {
			if v := strings.TrimSpace(r.URL.Query().Get(k)); v != "" {
				args[k] = queryArgValue(k, v)
			}
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		res, err := s.client.Call(ctx, wr.cmd, args)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, res)
	}
}

// decodeAllowedBody reads a JSON object from a POST body (size-capped) and
// returns only the route's allowlisted keys. It enforces POST and writes the
// error response itself; ok=false means the caller should return immediately.
// An unexpected key in the body is silently dropped — the control plane only
// ever sees the named arguments, mirroring writeProxy's query-arg allowlist.
func (s *Server) decodeAllowedBody(w http.ResponseWriter, r *http.Request, allowed []string) (map[string]any, bool) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "POST required"})
		return nil, false
	}
	var body map[string]any
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid JSON body: " + err.Error()})
		return nil, false
	}
	args := map[string]any{}
	for _, k := range allowed {
		if v, ok := body[k]; ok {
			args[k] = v
		}
	}
	return args, true
}

// jsonProxy returns a handler for one allowlisted mutating command whose
// arguments arrive as a JSON object in the request BODY. Unlike writeProxy
// (query-string args), this supports large values — a full plan JSON, a
// multi-line intent. Used by Flow Studio's Generate/Refine. The timeout is
// generous because these call the LLM.
func (s *Server) jsonProxy(jr writeRoute) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		args, ok := s.decodeAllowedBody(w, r, jr.args)
		if !ok {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
		defer cancel()
		res, err := s.client.Call(ctx, jr.cmd, args)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, res)
	}
}

// planRunProxy returns the handler for Flow Studio's Run button. CmdPlan
// streams RespEvent frames before its terminal result, so it cannot go through
// Call (which reads a single response); it is driven with Stream. The streamed
// events are discarded — the browser already receives plan.*/node.* live on the
// SSE /events firehose — but Stream must run to completion so the control-plane
// connection stays open for the run's whole duration (closing it early cancels
// the run's context, killing the plan mid-flight). The terminal result
// (plan_id + node_outputs) is relayed when the plan finishes.
func (s *Server) planRunProxy() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		args, ok := s.decodeAllowedBody(w, r, planRoute.args)
		if !ok {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), planRunTimeout)
		defer cancel()
		res, err := s.client.Stream(ctx, planRoute.cmd, args, func(*event.Event) {})
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, res)
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	// API payloads are live daemon state — never let a browser serve a stale
	// cached body after a mutation (no Cache-Control otherwise invites heuristic
	// caching, e.g. /api/routing showing an old chain after a save+reload).
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}


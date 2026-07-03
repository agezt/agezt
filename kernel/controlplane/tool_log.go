// SPDX-License-Identifier: MIT

package controlplane

// Tool-invocation audit log (M66) — a read-only view of the journal's
// tool.invoked + tool.result events (the agent loop journals one pair per tool
// call: tool, call_id, input, then output + error). `agt tool list` shows the
// tools the daemon ADVERTISES; this shows the calls the agent actually MADE and
// how each turned out, so an operator can audit "what did the agent run, and
// what broke?". It is the execution analogue of `agt edict log` (which audits
// the policy GATING of those same calls) — together they answer "was the call
// allowed?" and "what did it do?".

import (
	"encoding/json"
	"net"
	"sort"
	"strings"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/journal"
)

// toolOutputPreviewRunes bounds the one-line output/input excerpt folded into a
// tool-log row — long enough to read an error message or a short result, short
// enough to keep the response compact. Mirrors answerPreviewRunes' role for runs.
const toolOutputPreviewRunes = 100

func (s *Server) handleToolLog(conn net.Conn, req Request) {
	limit := defaultRunsLimit
	if raw, ok := req.Args["limit"]; ok {
		switch v := raw.(type) {
		case float64:
			limit = int(v)
		case int:
			limit = v
		case int64:
			limit = int(v)
		}
	}
	if limit < 1 {
		limit = 1
	}
	if limit > maxRunsLimit {
		limit = maxRunsLimit
	}
	// Cursor pagination (A2): opaque ts:seq token from the previous page; we
	// return rows strictly older than it. Absent/unparseable falls back to the
	// newest page. Shares journal.Cursor with /api/runs and the other logs.
	cursorMS, cursorSeq, cursorOK := journal.DecodeCursor(req.Args["cursor"])
	errorsOnly, _ := req.Args["errors"].(bool)
	toolFilter, _ := req.Args["tool"].(string)
	cutoff := sinceCutoff(req.Args["since_ms"]) // M65 helper: optional time window
	// Latency floor (M73): keep only calls at/above this wall-clock. 0 = no floor.
	var slowMS int64
	switch v := req.Args["slow_ms"].(type) {
	case float64:
		slowMS = int64(v)
	case int64:
		slowMS = v
	case int:
		slowMS = int64(v)
	}

	k, err := s.kernelFor(tenantOf(req))
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}

	// One row per tool.result (the always-present event: a policy-denied call
	// emits a result but no tool.invoked). A first-seen tool.invoked stashes the
	// call's input by call_id so the row can show what the agent asked for; since
	// the journal is in order, the invoked event precedes its result and the map
	// is already populated when we reach it.
	type invocation struct {
		ts, seq           int64
		actor, corr       string
		tool              string
		callID            string
		output            string
		isError           bool
		duration          int64 // M71: result.TS − invoked.TS, 0 when unknowable
		observationTrust  string
		observationSource string
		directiveLike     bool
		directiveMatches  []string
	}
	inputs := map[string]string{}   // call_id → input preview
	invokedTS := map[string]int64{} // call_id → tool.invoked timestamp (M71)
	results := make([]invocation, 0)
	if err := k.Journal().Range(func(e *event.Event) error {
		switch e.Kind {
		case event.KindToolInvoked:
			id, input := decodeToolInvoked(e.Payload)
			if id != "" {
				inputs[id] = input
				invokedTS[id] = e.TSUnixMS
			}
		case event.KindToolResult:
			if cutoff > 0 && e.TSUnixMS < cutoff {
				return nil // M65: outside the time window
			}
			decoded := decodeToolResult(e.Payload)
			if toolFilter != "" && decoded.tool != toolFilter {
				return nil
			}
			if errorsOnly && !decoded.isError {
				return nil
			}
			// Latency (M71) joins the call's invoked→result span by call_id. A
			// policy-denied call has no tool.invoked, so it has no latency (0).
			var dur int64
			if it, ok := invokedTS[decoded.callID]; ok && e.TSUnixMS >= it {
				dur = e.TSUnixMS - it
			}
			if slowMS > 0 && dur < slowMS {
				return nil // M73: faster than the latency floor (or unmeasurable)
			}
			results = append(results, invocation{
				ts: e.TSUnixMS, seq: e.Seq, actor: e.Actor, corr: e.CorrelationID,
				tool:              decoded.tool,
				callID:            decoded.callID,
				output:            decoded.output,
				isError:           decoded.isError,
				duration:          dur,
				observationTrust:  decoded.observationTrust,
				observationSource: decoded.observationSource,
				directiveLike:     decoded.directiveLike,
				directiveMatches:  decoded.directiveMatches,
			})
		}
		return nil
	}); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].ts != results[j].ts {
			return results[i].ts > results[j].ts
		}
		return results[i].seq > results[j].seq
	})
	// Cursor filter (A2): keep only rows strictly older than the cursor, in the
	// same descending (ts, seq) order, before applying the page limit.
	if cursorOK {
		kept := results[:0]
		for _, r := range results {
			if journal.KeepBeforeCursor(r.ts, r.seq, cursorMS, cursorSeq) {
				kept = append(kept, r)
			}
		}
		results = kept
	}
	if len(results) > limit {
		results = results[:limit]
	}

	out := make([]map[string]any, 0, len(results))
	for _, r := range results {
		out = append(out, map[string]any{
			"ts_unix_ms":     r.ts,
			"actor":          r.actor,
			"correlation_id": r.corr,
			"tool":           r.tool,
			"call_id":        r.callID,
			"input":          inputs[r.callID],
			"output":         r.output,
			"error":          r.isError,
			"duration_ms":    r.duration, // M71: invoked→result span (0 if unknowable)
			// Prompt-injection hygiene: keep provenance and directive-like taint
			// visible to operators instead of burying it in raw journal payloads.
			"observation_trust":  r.observationTrust,
			"observation_source": r.observationSource,
			"directive_like":     r.directiveLike,
			"directive_matches":  r.directiveMatches,
		})
	}
	// next_cursor (A2): the (ts, seq) of the last (oldest) emitted row, so the
	// client's next request pages past it. Only when the page is full.
	var nextCursor string
	if n := len(results); n > 0 {
		last := results[n-1]
		nextCursor = journal.NextCursor(last.ts, last.seq, n, limit)
	}
	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"invocations": out, "count": len(out), "next_cursor": nextCursor},
	})
}

// handleToolStats aggregates tool invocations (M67) — the execution-dashboard
// analogue of handleEdictStats. Folds the journal's tool.result events into
// total / errored / error-rate plus a per-tool breakdown ({calls, errors}).
// Optional tool scopes to one tool; since_ms windows by call time. Tenant-scoped.
func (s *Server) handleToolStats(conn net.Conn, req Request) {
	toolFilter, _ := req.Args["tool"].(string)
	cutoff := sinceCutoff(req.Args["since_ms"])

	k, err := s.kernelFor(tenantOf(req))
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}

	var total, errored int
	type toolAgg struct {
		calls, errors      int
		durSum, durSamples int64 // M75: per-tool latency, to show which TOOL is slow
	}
	byTool := map[string]*toolAgg{}
	invokedTS := map[string]int64{} // call_id → tool.invoked timestamp (M71)
	durations := make([]int64, 0)   // per-call latency, for the distribution (M71)
	// Failure-mode breakdown (M79): bucket error outputs by their message so an
	// operator sees WHAT is failing (denied / not-available / timeout / …), not
	// just how many — the tool analogue of runs stats' failed_by_reason.
	errorsByMessage := map[string]int{}
	if err := k.Journal().Range(func(e *event.Event) error {
		if e.Kind == event.KindToolInvoked {
			if id, _ := decodeToolInvoked(e.Payload); id != "" {
				invokedTS[id] = e.TSUnixMS
			}
			return nil
		}
		if e.Kind != event.KindToolResult {
			return nil
		}
		if cutoff > 0 && e.TSUnixMS < cutoff {
			return nil
		}
		decoded := decodeToolResult(e.Payload)
		if toolFilter != "" && decoded.tool != toolFilter {
			return nil
		}
		tool := decoded.tool
		if tool == "" {
			tool = "unknown"
		}
		total++
		agg := byTool[tool]
		if agg == nil {
			agg = &toolAgg{}
			byTool[tool] = agg
		}
		agg.calls++
		if decoded.isError {
			errored++
			agg.errors++
			msg := decoded.output // already whitespace-collapsed + capped by decodeToolResult
			if msg == "" {
				msg = "(no message)"
			}
			errorsByMessage[msg]++
		}
		if it, ok := invokedTS[decoded.callID]; ok && e.TSUnixMS >= it {
			d := e.TSUnixMS - it
			durations = append(durations, d)
			agg.durSum += d
			agg.durSamples++
		}
		return nil
	}); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}

	errorRate := 0.0
	if total > 0 {
		errorRate = float64(errored) / float64(total)
	}
	byToolOut := make(map[string]any, len(byTool))
	for tool, agg := range byTool {
		entry := map[string]any{"calls": agg.calls, "errors": agg.errors}
		if agg.durSamples > 0 {
			entry["avg_ms"] = agg.durSum / agg.durSamples // M75: per-tool mean latency
		}
		byToolOut[tool] = entry
	}
	// Latency distribution (M71) over calls with a joinable invoked→result span,
	// reusing the nearest-rank durationStats so it reads like runs stats' block.
	dstats := durationStats(durations)

	var sinceMS int64
	switch v := req.Args["since_ms"].(type) {
	case float64:
		sinceMS = int64(v)
	case int64:
		sinceMS = v
	case int:
		sinceMS = int64(v)
	}
	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			"total":             total,
			"errored":           errored,
			"error_rate":        errorRate,
			"by_tool":           byToolOut,
			"tools":             len(byTool),
			"window_ms":         sinceMS,
			"errors_by_message": errorsByMessage, // M79: failure-mode breakdown
			"duration_ms": map[string]any{
				"count": len(durations),
				"avg":   dstats.avg,
				"min":   dstats.min,
				"max":   dstats.max,
				"p50":   dstats.p50,
				"p95":   dstats.p95,
			},
		},
	})
}

// decodeToolInvoked pulls call_id + a whitespace-collapsed input preview out of
// a tool.invoked payload (M66). Returns zero values on parse failure so a
// malformed event simply contributes no input annotation.
func decodeToolInvoked(payload json.RawMessage) (callID, input string) {
	if len(payload) == 0 {
		return "", ""
	}
	var p struct {
		CallID string          `json:"call_id"`
		Input  json.RawMessage `json:"input"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return "", ""
	}
	return p.CallID, previewString(string(p.Input))
}

type decodedToolResult struct {
	tool              string
	callID            string
	output            string
	isError           bool
	observationTrust  string
	observationSource string
	directiveLike     bool
	directiveMatches  []string
}

// decodeToolResult pulls tool + call_id + output preview + error flag out of a
// tool.result payload (M66), plus observation-security metadata when present.
// Returns zero values on parse failure.
func decodeToolResult(payload json.RawMessage) decodedToolResult {
	if len(payload) == 0 {
		return decodedToolResult{}
	}
	var p struct {
		Tool              string   `json:"tool"`
		CallID            string   `json:"call_id"`
		Output            string   `json:"output"`
		Error             bool     `json:"error"`
		ObservationTrust  string   `json:"observation_trust"`
		ObservationSource string   `json:"observation_source"`
		DirectiveLike     bool     `json:"directive_like"`
		DirectiveMatches  []string `json:"directive_matches"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return decodedToolResult{}
	}
	return decodedToolResult{
		tool:              p.Tool,
		callID:            p.CallID,
		output:            previewString(p.Output),
		isError:           p.Error,
		observationTrust:  p.ObservationTrust,
		observationSource: p.ObservationSource,
		directiveLike:     p.DirectiveLike,
		directiveMatches:  p.DirectiveMatches,
	}
}

// previewString collapses all whitespace runs to single spaces, trims, and
// truncates to toolOutputPreviewRunes with an ellipsis. Shared by the invoked
// (input) and result (output) decoders so both excerpts read the same way.
func previewString(s string) string {
	one := strings.Join(strings.Fields(s), " ")
	if one == "" {
		return ""
	}
	r := []rune(one)
	if len(r) > toolOutputPreviewRunes {
		return string(r[:toolOutputPreviewRunes]) + "…"
	}
	return one
}

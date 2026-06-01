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
	errorsOnly, _ := req.Args["errors"].(bool)
	toolFilter, _ := req.Args["tool"].(string)
	cutoff := sinceCutoff(req.Args["since_ms"]) // M65 helper: optional time window

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
		ts, seq     int64
		actor, corr string
		tool        string
		callID      string
		output      string
		isError     bool
	}
	inputs := map[string]string{} // call_id → input preview
	results := make([]invocation, 0)
	if err := k.Journal().Range(func(e *event.Event) error {
		switch e.Kind {
		case event.KindToolInvoked:
			id, input := decodeToolInvoked(e.Payload)
			if id != "" {
				inputs[id] = input
			}
		case event.KindToolResult:
			if cutoff > 0 && e.TSUnixMS < cutoff {
				return nil // M65: outside the time window
			}
			tool, id, output, isErr := decodeToolResult(e.Payload)
			if toolFilter != "" && tool != toolFilter {
				return nil
			}
			if errorsOnly && !isErr {
				return nil
			}
			results = append(results, invocation{
				ts: e.TSUnixMS, seq: e.Seq, actor: e.Actor, corr: e.CorrelationID,
				tool: tool, callID: id, output: output, isError: isErr,
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
		})
	}
	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"invocations": out, "count": len(out)},
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

// decodeToolResult pulls tool + call_id + output preview + error flag out of a
// tool.result payload (M66). Returns zero values on parse failure.
func decodeToolResult(payload json.RawMessage) (tool, callID, output string, isError bool) {
	if len(payload) == 0 {
		return "", "", "", false
	}
	var p struct {
		Tool   string `json:"tool"`
		CallID string `json:"call_id"`
		Output string `json:"output"`
		Error  bool   `json:"error"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return "", "", "", false
	}
	return p.Tool, p.CallID, previewString(p.Output), p.Error
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

// SPDX-License-Identifier: MIT

// Day 28 + 1 (post-merge): the Web UI Mission Control tile set calls
// `/api/spend/today` and `/api/attention`, but those routes were never wired
// during the IA cleanup — they silently no-op'd when the daemon returned 404.
// This file implements the two handlers and registers them in
// registerCockpitReadCommands (see registry.go), so the existing front-end
// hooks (useSpendToday at MissionControl.tsx:95 and useAttention at
// MissionControl.tsx:118) light up without further FE changes.

package controlplane

import (
	"net"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/approval"
	"github.com/agezt/agezt/kernel/governor"
)

// handleSpendToday serves CmdSpendToday. Returns the governor's microcents USD
// spent so far today under the slim shape the Web UI Mission Control "Spend
// today" tile consumes — just { "total": <integer microcents> }, no per-task
// breakdown, no ceiling, no other fields. Mirrors handleBudget's not-a-governor
// guard so test rigs that wire a raw provider see a clear error rather than a
// panic trace, and returns a 0 total (not a missing field) when the provider
// isn't tracking spend — the tile then renders "0¢" rather than "no data",
// which is the right call when the operator simply has no spend yet today.
func (s *Server) handleSpendToday(conn net.Conn, req Request) {
	gov, ok := s.k.Provider().(*governor.Governor)
	if !ok {
		// No governor in this process — return a zero total rather than an error
		// so the tile stays a calm, deterministic "0¢" instead of failing every
		// poll. Operators can tell from the absence of other governor-backed
		// panels (budget, rate-limit) when the governor isn't wired.
		s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"total": int64(0)}})
		return
	}
	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"total": gov.Snapshot().SpentMicrocents},
	})
}

// handleAttention serves CmdAttention. Returns a single, time-sorted, capped list
// of items requiring operator eyes, drawn from two sources:
//   - pending HITL approvals (Approvals().Pending) — always included regardless
//     of `window`; an open approval is an open task, not a time window.
//   - pulse asks (s.pulse.PendingAsks) — actionable observations the heart beat
//     raised under initiative=ask, only the ones newer than `window`.
//
// Args (both optional):
//   - window: time window for time-sensitive kinds (pulse_ask). Default 24h.
//             Approvals ignore the window. Accepts "5m", "1h", "24h", or any
//             string time.ParseDuration understands; falls back to 24h on bad
//             input rather than 400ing — the panel is a status read, not an
//             action.
//   - limit:  max items returned. Default 8, hard cap 50.
//
// Result:
//   { "items": [
//       { "id": "<approval-or-issue-key>",
//         "kind": "approval" | "pulse_ask",
//         "summary": "<one-line>",
//         "ts": <unix ms>,         // for approvals, the CreatedAt; for asks, the raise ts.
//         "href": "/approvals" | "/jarvis#ask-<issue_key>" } ],
//     "count": <int> }
//
// Items are sorted newest-first, then truncated to limit. The Web UI hook
// (useAttention) renders the list directly — no extra shape coercion.
func (s *Server) handleAttention(conn net.Conn, req Request) {
	window, limit := attentionArgs(req.Args)

	cutoff := time.Now().Add(-window).UnixMilli()

	type item struct {
		ID      string `json:"id"`
		Kind    string `json:"kind"`
		Summary string `json:"summary"`
		TS      int64  `json:"ts"`
		HRef    string `json:"href"`
	}
	items := make([]item, 0, 16)

	// Pending approvals are always included.
	if app := s.k.Approvals(); app != nil {
		for _, p := range app.Pending() {
			summary := attentionSummaryForApproval(p)
			items = append(items, item{
				ID:      p.ID,
				Kind:    "approval",
				Summary: summary,
				TS:      p.CreatedAt.UnixMilli(),
				HRef:    "/approvals",
			})
		}
	}

	// Pulse asks are time-windowed: stale observations are noise on this panel.
	if s.pulse != nil {
		for _, raw := range s.pulse.PendingAsks() {
			issue, _ := raw["issue_key"].(string)
			summary, _ := raw["summary"].(string)
			ts, _ := raw["ts_unix_ms"].(int64)
			if ts < cutoff {
				continue
			}
			if issue == "" {
				continue
			}
			items = append(items, item{
				ID:      issue,
				Kind:    "pulse_ask",
				Summary: summary,
				TS:      ts,
				HRef:    "/jarvis#ask-" + issue,
			})
		}
	}

	// Newest first so the most recent / most urgent rises to the top — operators
	// scan the panel top-down and the first three rows carry the highest signal.
	sort.Slice(items, func(i, j int) bool {
		if items[i].TS != items[j].TS {
			return items[i].TS > items[j].TS
		}
		return items[i].ID < items[j].ID
	})

	if len(items) > limit {
		items = items[:limit]
	}

	// Marshal through map[string]any so the JSON wire format matches the rest
	// of the read-route contract (Result is always a map at the top level).
	out := make([]map[string]any, 0, len(items))
	for _, it := range items {
		out = append(out, map[string]any{
			"id":      it.ID,
			"kind":    it.Kind,
			"summary": it.Summary,
			"ts":      it.TS,
			"href":    it.HRef,
		})
	}
	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"items": out, "count": len(out)},
	})
}

// attentionArgs parses the window/limit args with sensible fallbacks so the
// panel never 400s on bad input — the route is a status read, not a mutation,
// and the Mission Control tile stays calm even when the URL is hand-typed.
func attentionArgs(args map[string]any) (time.Duration, int) {
	window := 24 * time.Hour
	if raw, ok := args["window"]; ok {
		switch v := raw.(type) {
		case string:
			if d, err := time.ParseDuration(strings.TrimSpace(v)); err == nil && d > 0 {
				window = d
			}
		case float64:
			if v > 0 {
				window = time.Duration(v * float64(time.Second))
			}
		case int64:
			if v > 0 {
				window = time.Duration(v) * time.Second
			}
		}
	}
	limit := 8
	if raw, ok := args["limit"]; ok {
		switch v := raw.(type) {
		case string:
			if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n > 0 {
				limit = n
			}
		case float64:
			if v > 0 {
				limit = int(v)
			}
		case int64:
			if v > 0 {
				limit = int(v)
			}
		}
	}
	if limit > 50 {
		limit = 50
	}
	return window, limit
}

// attentionSummaryForApproval produces a one-line summary for the attention panel.
// Tool-cap approvals get "<tool> — <reason>"; broader capability requests get
// "<capability> requested by <actor>". Falls back to the approval ID so an empty
// reason never produces an empty card.
func attentionSummaryForApproval(p approval.Request) string {
	if p.ToolName != "" {
		if p.Reason != "" {
			return p.ToolName + " — " + p.Reason
		}
		return p.ToolName
	}
	if p.Capability != "" {
		who := p.Actor
		if who == "" {
			who = "agent"
		}
		if p.Reason != "" {
			return p.Capability + " requested by " + who + " — " + p.Reason
		}
		return p.Capability + " requested by " + who
	}
	return p.ID
}

// SPDX-License-Identifier: MIT

package controlplane

// Past-runs enumeration. Walks the journal once, pairs
// task.received and task.completed events by correlation_id,
// and emits a sorted summary. Read-only; the journal is the
// single source of truth so no caching/snapshot is needed —
// operators always see the latest.

import (
	"encoding/json"
	"net"
	"sort"

	"github.com/agezt/agezt/kernel/event"
)

const (
	defaultRunsLimit = 20
	maxRunsLimit     = 1_000
)

type runEntry struct {
	CorrelationID   string
	Intent          string
	StartedUnixMS   int64
	// StartedSeq is the journal seq of the task.received event;
	// used as a tie-break for the sort when two runs share the
	// same TSUnixMS (the bus's wall-clock resolution is 1ms, so
	// fast back-to-back submissions collide).
	StartedSeq      int64
	CompletedUnixMS int64
	Iters           int
	Completed       bool
}

func (s *Server) handleRunsList(conn net.Conn, req Request) {
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

	// Single forward walk: build per-correlation entry on
	// task.received, update on task.completed. We don't try to
	// stream early-stop after N — limit is applied post-sort, since
	// "last N runs" requires knowing all runs first (journal order
	// is by seq, not by run start time, and the same run's events
	// are interleaved with others under concurrency).
	runs := map[string]*runEntry{}
	err := s.k.Journal().Range(func(e *event.Event) error {
		switch e.Kind {
		case event.KindTaskReceived:
			entry, ok := runs[e.CorrelationID]
			if !ok {
				entry = &runEntry{CorrelationID: e.CorrelationID}
				runs[e.CorrelationID] = entry
			}
			entry.StartedUnixMS = e.TSUnixMS
			entry.StartedSeq = e.Seq
			// Pull intent out of the payload — agent.go writes it as
			// {"intent": "..."} on KindTaskReceived (see kernel/agent).
			if intent := extractIntent(e.Payload); intent != "" {
				entry.Intent = intent
			}
		case event.KindTaskCompleted:
			entry, ok := runs[e.CorrelationID]
			if !ok {
				// Completed without received? Only possible if the
				// journal was rotated mid-run; record the half we
				// have so the operator at least sees the chain id.
				entry = &runEntry{CorrelationID: e.CorrelationID}
				runs[e.CorrelationID] = entry
			}
			entry.CompletedUnixMS = e.TSUnixMS
			entry.Completed = true
			if iters := extractIters(e.Payload); iters > 0 {
				entry.Iters = iters
			}
		}
		return nil
	})
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}

	// Sort by StartedUnixMS DESC. Entries with zero start time (the
	// "completed without received" edge case) sort to the bottom.
	entries := make([]*runEntry, 0, len(runs))
	for _, r := range runs {
		entries = append(entries, r)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].StartedUnixMS != entries[j].StartedUnixMS {
			return entries[i].StartedUnixMS > entries[j].StartedUnixMS
		}
		// Same wall-clock millisecond: fall back to journal seq so
		// the newer-arrived run still sorts first.
		return entries[i].StartedSeq > entries[j].StartedSeq
	})
	if len(entries) > limit {
		entries = entries[:limit]
	}

	out := make([]map[string]any, 0, len(entries))
	for _, r := range entries {
		status := "running"
		duration := int64(0)
		if r.Completed {
			status = "completed"
			if r.StartedUnixMS > 0 {
				duration = r.CompletedUnixMS - r.StartedUnixMS
			}
		}
		out = append(out, map[string]any{
			"correlation_id":    r.CorrelationID,
			"intent":            r.Intent,
			"status":            status,
			"started_unix_ms":   r.StartedUnixMS,
			"completed_unix_ms": r.CompletedUnixMS,
			"duration_ms":       duration,
			"iters":             r.Iters,
		})
	}

	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			"runs":  out,
			"count": len(out),
		},
	})
}

// extractIntent pulls "intent" out of a task.received payload.
// Returns "" if missing or malformed — operator-facing rendering
// gracefully shows "(no intent)" rather than crashing.
func extractIntent(payload json.RawMessage) string {
	if len(payload) == 0 {
		return ""
	}
	var p struct {
		Intent string `json:"intent"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return ""
	}
	return p.Intent
}

// extractIters pulls "iters" out of a task.completed payload.
// Returns 0 on parse failure for the same reason as extractIntent.
func extractIters(payload json.RawMessage) int {
	if len(payload) == 0 {
		return 0
	}
	var p struct {
		// JSON numbers decode as float64 by default; accept either
		// shape so the field's wire type can evolve without breaking.
		Iters float64 `json:"iters"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return 0
	}
	return int(p.Iters)
}

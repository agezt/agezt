// SPDX-License-Identifier: MIT

package controlplane

// Prompt-cache savings aggregate (M293). Folds the journal's budget.consumed
// events into how many prompt tokens were served from / written to the provider
// cache, and how many microcents that saved versus paying the full input rate
// for every input token. Read-only, tenant-scoped, optional --since window.
//
// "Saved" is the no-cache baseline minus the recorded cost: for each call,
// governor.CostMicrocents(model, input, output) bills every input token at the
// full input rate (no cache discount), while the event's recorded
// cost_microcents already reflects the cache-read / cache-write rates (M289-291).
// The difference, summed and floored at zero, is the cache saving.

import (
	"encoding/json"
	"net"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/governor"
)

func (s *Server) handleCacheStats(conn net.Conn, req Request) {
	cutoff := sinceCutoff(req.Args["since_ms"])
	var sinceMS int64
	switch v := req.Args["since_ms"].(type) {
	case float64:
		sinceMS = int64(v)
	case int64:
		sinceMS = v
	case int:
		sinceMS = int64(v)
	}

	k, err := s.kernelFor(tenantOf(req))
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}

	var cached, written, calls int64
	var saved int64
	if err := k.Journal().Range(func(e *event.Event) error {
		if e.Kind != event.KindBudgetConsumed {
			return nil
		}
		if cutoff > 0 && e.TSUnixMS < cutoff {
			return nil
		}
		var p struct {
			Model                 string `json:"model"`
			InputTokens           int    `json:"input_tokens"`
			OutputTokens          int    `json:"output_tokens"`
			CachedInputTokens     int    `json:"cached_input_tokens"`
			CacheWriteInputTokens int    `json:"cache_write_input_tokens"`
			CostMicrocents        int64  `json:"cost_microcents"`
		}
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return nil // skip a malformed payload rather than abort the fold
		}
		calls++
		cached += int64(p.CachedInputTokens)
		written += int64(p.CacheWriteInputTokens)
		// No-cache baseline: every input token at the full input rate.
		baseline := governor.CostMicrocents(p.Model, p.InputTokens, p.OutputTokens)
		if d := baseline - p.CostMicrocents; d > 0 {
			saved += d
		}
		return nil
	}); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}

	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			"cached_input_tokens":      cached,
			"cache_write_input_tokens": written,
			"saved_microcents":         saved,
			"calls":                    calls,
			"window_ms":                sinceMS,
		},
	})
}

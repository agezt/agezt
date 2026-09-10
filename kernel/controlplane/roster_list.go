// SPDX-License-Identifier: MIT

package controlplane

// Agent list endpoint (M783) + its 1.5s TTL cache. Carved out of roster.go
// during the Day 24 god file split #2 so the main file can focus on
// lifecycle handlers (add/edit/remove/pause/revive/wake/repair).

import (
	"encoding/json"
	"hash/fnv"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/roster"
)

// agentModelChain builds a named agent's run chain: the resolved primary model
// first, then the profile's ordered fallbacks, skipping duplicates of the
// primary (so an explicit --model equal to a fallback doesn't try it twice).
func agentModelChain(primary string, fallbacks []string) []string {
	chain := []string{primary}
	for _, m := range fallbacks {
		if m = strings.TrimSpace(m); m != "" && m != primary {
			chain = append(chain, m)
		}
	}
	return chain
}

// profileView is the stable wire shape for one profile.
func profileView(p roster.Profile) map[string]any {
	b, _ := json.Marshal(p)
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	m["kind"] = p.Kind()
	m["managed"] = !p.AllowsDirectCall()
	return m
}

func (s *Server) handleAgentList(conn net.Conn, req Request) {
	// Cache layer (1.5s TTL): every poll of this endpoint triggered 11 full
	// journal.Range walks; with the 6–8s SPA poll cadence and N tabs polling
	// in parallel, that's a 11·N·(60/8) ≈ 82 journal walks/minute per viewer.
	// The cache key is a content hash of the underlying roster so profile
	// additions/edits invalidate it implicitly on the very next read; explicit
	// mutation handlers also call invalidateAgentListCache() to skip the TTL
	// window for the write that just happened.
	profiles := s.k.Roster().List()
	key := rosterContentHash(profiles)
	out, total, enabled, hit := s.tryServeAgentListCache(key, profiles)
	if !hit {
		statuses := s.agentStatusViews(profiles)
		out = make([]any, 0, len(profiles))
		enabled = 0
		for _, p := range profiles {
			view := profileView(p)
			if st, ok := statuses[p.Slug]; ok {
				view["status"] = st
			}
			out = append(out, view)
			if p.Enabled {
				enabled++
			}
		}
		total = len(out)
		s.storeAgentListCache(key, profiles, out, total, enabled)
	}

	// Cursor pagination (M-pending follow-up): the SPA's Agents / AgentPage /
	// Roster views load this on every poll, so for large rosters streaming the
	// whole thing makes the panel slow. Cursor encodes (CreatedMS, Slug) of the
	// LAST entry on the previous page; the server sorts DESC and skips entries
	// strictly newer than the cursor. List() returns ASC — reverse in place
	// once, then filter+truncate.
	//
	// Copy before reversing: when the cache hits, out shares its backing array
	// with s.agentListCacheResult. An in-place reverse would corrupt the cached
	// data for the next caller.
	out = append([]any(nil), out...)
	limit, err := argLimit(req.Args, 0, 1000)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	var cursorMS int64
	var cursorSlug string
	cursorOK := false
	if raw, _, err := argString(req.Args, "cursor"); err != nil {
		s.fail(conn, req, err)
		return
	} else if raw != "" {
		msStr, slug, _ := strings.Cut(raw, ":")
		if ms, err := strconv.ParseInt(msStr, 10, 64); err == nil {
			cursorMS, cursorSlug, cursorOK = ms, slug, true
		}
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	if cursorOK {
		filtered := out[:0]
		for _, raw := range out {
			v, _ := raw.(map[string]any)
			// profileView round-trips the wire shape through JSON, so int64
			// fields arrive as float64. Both forms are accepted; cover the
			// float64 case (typical) and int64 (defensive).
			var ms int64
			switch n := v["created_ms"].(type) {
			case float64:
				ms = int64(n)
			case int64:
				ms = n
			}
			slug, _ := v["slug"].(string)
			if ms > cursorMS {
				continue
			}
			if ms == cursorMS && slug >= cursorSlug {
				continue
			}
			filtered = append(filtered, raw)
		}
		out = filtered
	}
	var nextCursor string
	if limit > 0 && len(out) > limit {
		out = out[:limit]
		last := out[limit-1].(map[string]any)
		var lastMS int64
		switch n := last["created_ms"].(type) {
		case float64:
			lastMS = int64(n)
		case int64:
			lastMS = n
		}
		nextCursor = encodeAgentsCursor(lastMS, last["slug"].(string))
	}
	result := map[string]any{
		"profiles":      out,
		"count":         len(out),
		"total":         total,
		"enabled_count": enabled,
	}
	if nextCursor != "" {
		result["next_cursor"] = nextCursor
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: result})
}

// rosterContentHash produces a cheap, stable key for the agent-list cache
// from the live roster. It mixes (slug, updated_ms, enabled, retired,
// system) so any profile edit — not just a count change — flips the hash
// and invalidates the entry on the very next read. We don't hash
// journal-derived fields (last_activity, repair state, etc.) here because
// those are intentionally absorbed by the cached result on a 1.5s TTL.
func rosterContentHash(profiles []roster.Profile) uint64 {
	if len(profiles) == 0 {
		return 0
	}
	h := fnv.New64a()
	var buf []byte
	for _, p := range profiles {
		buf = buf[:0]
		buf = append(buf, p.Slug...)
		buf = append(buf, 0)
		buf = strconv.AppendInt(buf, p.UpdatedMS, 16)
		buf = append(buf, 0)
		if p.Enabled {
			buf = append(buf, 1)
		}
		if p.Retired {
			buf = append(buf, 1)
		}
		if p.System {
			buf = append(buf, 1)
		}
		buf = append(buf, 0)
		_, _ = h.Write(buf)
	}
	return h.Sum64()
}

const agentListCacheTTL = 1500 * time.Millisecond

// tryServeAgentListCache returns the cached result if the key matches AND
// the entry is younger than the TTL. hit=false forces the caller to
// recompute; the caller is then expected to call storeAgentListCache so
// the next request benefits. The TTL is 1.5s — short enough that a manual
// profile edit surfaces in ≤2s on a polling client, long enough to
// collapse 5+ in-flight polls of one tab to a single underlying walk.
func (s *Server) tryServeAgentListCache(key uint64, profiles []roster.Profile) (out []any, total, enabled int, hit bool) {
	s.agentListCacheMu.RLock()
	defer s.agentListCacheMu.RUnlock()
	if s.agentListCacheKey != key {
		return nil, 0, 0, false
	}
	if time.Since(s.agentListCacheAt) > agentListCacheTTL {
		return nil, 0, 0, false
	}
	// Re-validate the roster fingerprint — defence in depth against a hash
	// collision across different rosters. Cheap (a 16-byte buf × N profiles).
	if rosterContentHash(profiles) != s.agentListCacheKey {
		return nil, 0, 0, false
	}
	return s.agentListCacheResult, s.agentListCacheTotal, s.agentListCacheEnabled, true
}

func (s *Server) storeAgentListCache(key uint64, profiles []roster.Profile, out []any, total, enabled int) {
	s.agentListCacheMu.Lock()
	defer s.agentListCacheMu.Unlock()
	s.agentListCacheKey = key
	s.agentListCacheResult = out
	s.agentListCacheTotal = total
	s.agentListCacheEnabled = enabled
	s.agentListCacheAt = time.Now()
}

// invalidateAgentListCache drops the cached entry so the very next
// /api/agents call rebuilds from the journal. Mutation handlers call this
// before returning so the operator sees their edit immediately, without
// waiting for the 1.5s TTL.
func (s *Server) invalidateAgentListCache() {
	s.agentListCacheMu.Lock()
	s.agentListCacheKey = 0
	s.agentListCacheResult = nil
	s.agentListCacheMu.Unlock()
}

// encodeAgentsCursor packs (CreatedMS, Slug) into the opaque "<ms>:<slug>"
// cursor string the SPA echoes back in the next request. Slugs are guaranteed
// unique (validated at roster.Add time) and never contain ':' (slugRe), so
// strings.Cut on ':' round-trips losslessly.
func encodeAgentsCursor(ms int64, slug string) string {
	return strconv.FormatInt(ms, 10) + ":" + slug
}

// parseSeqCursor extracts the opaque "<seq>" cursor used by the journal-sorted
// endpoints (agents/activity, agents/repair_status). Returns (0, false) for a
// missing, empty, or unparseable cursor — callers treat that as "no cursor,
// return first page."
func parseSeqCursor(raw string) (int64, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false
	}
	seq, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || seq <= 0 {
		return 0, false
	}
	return seq, true
}

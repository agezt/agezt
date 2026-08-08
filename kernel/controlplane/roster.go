// SPDX-License-Identifier: MIT

package controlplane

// Agent roster CRUD handlers (M783) — the management path behind `agt agent`.
// Lifecycle changes go through the kernel so every create/edit/pause/resume/
// remove is journaled (roster.*) and auditable via `agt why`. Profiles are
// addressed by ref = id OR slug everywhere, so operators can say
// `agt agent show researcher` without copying ULIDs.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/internal/strutil"
	"github.com/agezt/agezt/kernel/board"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/roster"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/tools/overseertool"
)

type agentRepairRow struct {
	Seq                            int64
	TSUnixMS                       int64
	Agent                          string
	CorrelationID                  string
	Mode                           string
	Phase                          string
	Reason                         string
	Fingerprint                    string
	SelfRepairAttempt              int
	SelfRepairMaxAttempts          int
	Issues                         []string
	Applied                        []string
	Answer                         string
	Error                          string
	TargetAgent                    string
	TargetCorr                     string
	MailboxMessage                 string
	Resolution                     string
	ResolutionSummary              string
	DelegateTo                     string
	DelegatedBy                    string
	RootAgent                      string
	ChainDepth                     int
	IncidentID                     string
	RootIncidentID                 string
	ParentIncidentID               string
	NextEligibleMS                 int64
	RoutingTaskType                string
	RoutingTaskModelChain          []string
	PreviousRoutingTaskModelChain  []string
	RoutingForceGeneration         int
	PreviousRoutingForceGeneration int
}

type agentEscalationRow struct {
	MessageID         string
	From              string
	To                string
	Text              string
	TSUnixMS          int64
	Status            string
	ReplyCount        int
	Acked             bool
	SourceAgent       string
	Mode              string
	WakePhase         string
	WakeReason        string
	WakeError         string
	WakeCorrelationID string
	Fingerprint       string
	Resolution        string
	ResolutionSummary string
	DelegateTo        string
	OriginKind        string
	OriginAgent       string
	RootAgent         string
	ChainDepth        int
	IncidentID        string
	RootIncidentID    string
	ParentIncidentID  string
}

type agentRepairSummary struct {
	Latest        agentRepairRow
	HasLatest     bool
	InflightCount int
}

type agentRoutingPressure struct {
	Count      int
	LastReason string
	LastFailed string
	LastNext   string
	LastTSMS   int64
}

type agentRetryPressure struct {
	Count       int
	LastReason  string
	LastTSMS    int64
	NextAttempt int
	MaxAttempts int
}

type agentEscalationLoad struct {
	Open  int
	Acked int
}

type agentWakeStatus struct {
	ScheduleCount       int
	StandingCount       int
	EventSubjects       []string
	NextScheduledWakeMS int64
	NextScheduledLabel  string
}

type agentLiveStatus struct {
	ActiveRuns              int
	ActiveCorrelationID     string
	ActiveIntent            string
	ActiveStartedMS         int64
	ActiveModel             string
	ActiveSpentMc           int64
	ActivePhase             string
	ActiveLastEventMS       int64
	ActiveLastEventKind     string
	ActiveDetail            string
	ActiveTool              string
	ActiveIter              int
	ActiveWakeSource        string
	ActiveWakeReason        string
	ActiveScheduleID        string
	ActiveStandingID        string
	ActiveStandingName      string
	ActiveTriggerSubject    string
	ActiveParentCorrelation string
}

type agentLastActivity struct {
	TSUnixMS      int64
	Kind          string
	CorrelationID string
	Summary       string
}

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

// agentStatusAccums holds the per-agent state that the journal-derivable
// helpers accumulate. Roster-agentList page is the single consumer; collecting
// every accumulator in a SINGLE journal.Range pass turns what used to be
// O(journalSize) per helper (and 11× that across all helpers) into one
// O(journalSize) walk. Large, busy journals were reliably tripping the
// control-plane connection's 10-minute read deadline under the previous
// 11-Range design (each Range does a callback-driven O(n) walk over every
// durable event, with JSON-unmarshal + map lookups per event). The
// single-pass dispatch below is behavior-preserving: each accumulator's
// key-set and "latest wins" semantics match the original per-helper
// implementations (the retired per-helper methods have been deleted; this
// dispatch is now the only journal-derived roster-status path).
func (s *Server) handleAgentAdd(conn net.Conn, req Request) {
	raw, ok := req.Args["profile"]
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.profile required"})
		return
	}
	b, err := json.Marshal(raw)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.profile: " + err.Error()})
		return
	}
	var p roster.Profile
	if err := json.Unmarshal(b, &p); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.profile: " + err.Error()})
		return
	}
	normalizeAgentProfileKind(b, &p)
	p.System = false // System is kernel-owned (set only by guardian seeding); never accept it from a client (M961)
	if err := s.validateAgentHierarchyRefs(p); err != nil {
		s.fail(conn, req, err)
		return
	}
	saved, err := s.k.AddProfile(p)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	s.invalidateAgentListCache()
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"profile": profileView(saved)}})
}

// handleAgentEdit applies args.profile's MUTABLE fields to the profile named by
// args.ref, using a PATCH semantic: only fields explicitly present in the input
// JSON payload are applied; all other fields remain unchanged. This prevents a
// partial profile (e.g. only {"model":"gpt-5"}) from clearing soul, budget,
// policy fields, etc. Identity/lifecycle fields are protected by the store, so
// a stale client can't rename a slug or resurrect a paused agent.
func (s *Server) handleAgentEdit(conn net.Conn, req Request) {
	ref, err := requiredArgString(req.Args, "ref")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	raw, ok := req.Args["profile"]
	if !ok {
		s.failMsg(conn, req, "args.profile required")
		return
	}
	b, err := json.Marshal(raw)
	if err != nil {
		s.failMsg(conn, req, "args.profile: "+err.Error())
		return
	}
	// Parse the raw payload into a flat map to detect which top-level keys the
	// caller explicitly provided (as opposed to zero-value fields from omission).
	provided := map[string]bool{}
	if rawMap, _ := raw.(map[string]any); rawMap != nil {
		for k := range rawMap {
			provided[k] = true
		}
	}
	var in roster.Profile
	if err := json.Unmarshal(b, &in); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.profile: " + err.Error()})
		return
	}
	normalizeAgentProfileKind(b, &in)
	// If the caller provided "kind" (e.g. "subagent"), normalizeAgentProfileKind
	// may have set DirectCallable = false on `in`. Propagate that into the
	// provided set so applyAgentMutableProfilePatch applies it.
	if provided["kind"] && in.DirectCallable != nil && !*in.DirectCallable {
		provided["direct_callable"] = true
	}
	current, ok := s.k.Roster().Get(ref)
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown agent: " + ref})
		return
	}
	candidate := current
	applyAgentMutableProfilePatch(&candidate, in, provided)
	if err := s.validateAgentHierarchyRefs(candidate); err != nil {
		s.fail(conn, req, err)
		return
	}
	p, found, err := s.k.UpdateProfile(ref, func(dst *roster.Profile) {
		applyAgentMutableProfilePatch(dst, in, provided)
	})
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	if !found {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown agent: " + ref})
		return
	}
	s.invalidateAgentListCache()
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"profile": profileView(p)}})
}

// applyAgentMutableProfilePatch applies only those fields of `in` whose JSON key
// is present in `provided` to `dst`. This implements a PATCH merge where
// omitted fields keep their current value — fixing the classic "partial edit
// clears omitted fields" bug.
func applyAgentMutableProfilePatch(dst *roster.Profile, in roster.Profile, provided map[string]bool) {
	if provided["name"] {
		dst.Name = in.Name
	}
	if provided["soul"] {
		dst.Soul = in.Soul
	}
	if provided["instructions"] {
		dst.Instructions = in.Instructions
	}
	if provided["model"] {
		dst.Model = in.Model
	}
	if provided["fallbacks"] {
		dst.Fallbacks = in.Fallbacks
	}
	if provided["task_type"] {
		dst.TaskType = in.TaskType
	}
	if provided["max_cost_mc"] {
		dst.MaxCostMc = in.MaxCostMc
	}
	if provided["max_daily_mc"] {
		dst.MaxDailyMc = in.MaxDailyMc
	}
	if provided["memory_scope"] {
		dst.MemoryScope = in.MemoryScope
	}
	if provided["workdir"] {
		dst.Workdir = in.Workdir
	}
	if provided["owner_agent"] {
		dst.OwnerAgent = in.OwnerAgent
	}
	if provided["parent_agent"] {
		dst.ParentAgent = in.ParentAgent
	}
	if provided["direct_callable"] {
		dst.DirectCallable = in.DirectCallable
	}
	if provided["retry_policy"] {
		dst.RetryPolicy = in.RetryPolicy
	}
	if provided["health_policy"] {
		dst.HealthPolicy = in.HealthPolicy
	}
	if provided["self_repair"] {
		dst.SelfRepairPolicy = in.SelfRepairPolicy
	}
	if provided["noise_policy"] {
		dst.NoisePolicy = in.NoisePolicy
	}
	if provided["tool_allow"] {
		dst.ToolAllow = in.ToolAllow
	}
	if provided["tool_deny"] {
		dst.ToolDeny = in.ToolDeny
	}
	if provided["trust_ceiling"] {
		dst.TrustCeiling = in.TrustCeiling
	}
	if provided["execution_profile"] {
		dst.ExecutionProfile = strings.TrimSpace(in.ExecutionProfile)
	}
	if provided["config_overrides"] {
		dst.ConfigOverrides = in.ConfigOverrides
	}
	if provided["lifecycle"] {
		dst.Lifecycle = in.Lifecycle
	}
	if provided["tasklist"] {
		dst.TaskList = in.TaskList
	}
	if provided["description"] {
		dst.Description = in.Description
	}
}

func normalizeAgentProfileKind(raw []byte, p *roster.Profile) {
	var meta struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(raw, &meta); err != nil {
		return
	}
	if strings.EqualFold(strings.TrimSpace(meta.Kind), "subagent") {
		no := false
		p.DirectCallable = &no
	}
}

func (s *Server) validateAgentHierarchyRefs(p roster.Profile) error {
	for label, ref := range map[string]string{"owner_agent": p.OwnerAgent, "parent_agent": p.ParentAgent} {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		if strings.EqualFold(ref, strings.TrimSpace(p.Slug)) {
			return fmt.Errorf("roster: %s cannot point to the same agent", label)
		}
		target, ok := s.k.Roster().Get(ref)
		if !ok {
			return fmt.Errorf("roster: %s %q does not exist", label, ref)
		}
		if target.Retired {
			return fmt.Errorf("roster: %s %q is retired", label, ref)
		}
	}
	return nil
}

func managedSubagentDirectCallError(p roster.Profile, action string) string {
	manager := strings.TrimSpace(p.ParentAgent)
	if manager == "" {
		manager = strings.TrimSpace(p.OwnerAgent)
	}
	hint := "route the work through its parent/owner agent"
	if manager != "" {
		hint = "wake " + manager + " or delegate through it"
	}
	return "agent " + p.Slug + " is a managed sub-agent and cannot be " + action + " directly; " + hint
}

func (s *Server) handleAgentSetEnabled(conn net.Conn, req Request) {
	ref, err := requiredArgString(req.Args, "ref")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	// Accept enabled as a bool (CLI/JSON) or a "true"/"false"/"1"/"0" string
	// (the webui query-arg transport carries every value as a string).
	enabled := false
	switch v := req.Args["enabled"].(type) {
	case bool:
		enabled = v
	case string:
		enabled = strings.EqualFold(v, "true") || v == "1"
	}
	p, err := s.k.SetProfileEnabled(ref, enabled)
	if err != nil {
		if errors.Is(err, roster.ErrNotFound) {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown agent: " + ref})
			return
		}
		if errors.Is(err, roster.ErrRetired) {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "agent " + ref + " is retired — revive it first"})
			return
		}
		s.fail(conn, req, err)
		return
	}
	res := map[string]any{"profile": profileView(p)}
	if enabled {
		res["standing_paused"] = s.countAgentPausedStanding(p.Slug)
		res["schedules_paused"] = s.countAgentPausedSchedules(p.Slug)
	}
	s.invalidateAgentListCache()
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: res})
}

func (s *Server) handleAgentTaskUpdate(conn net.Conn, req Request) {
	ref, err := requiredArgString(req.Args, "ref")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	op, _, err := argString(req.Args, "op")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	op = strings.ToLower(strings.TrimSpace(op))
	if op == "" {
		op = "update"
	}
	if op != "add" && op != "update" && op != "remove" && op != "delete" {
		s.failMsg(conn, req, "args.op must be add, update, or remove")
		return
	}
	var in roster.AgentTask
	if raw, ok := req.Args["task"]; ok {
		b, err := json.Marshal(raw)
		if err != nil {
			s.failMsg(conn, req, "args.task: "+err.Error())
			return
		}
		if err := json.Unmarshal(b, &in); err != nil {
			s.failMsg(conn, req, "args.task: "+err.Error())
			return
		}
	}
	// Flat-arg overrides layered over args.task (both transports are live).
	for _, f := range []struct {
		key string
		dst *string
	}{
		{"id", &in.ID}, {"title", &in.Title}, {"description", &in.Description},
		{"scope", &in.Scope}, {"status", &in.Status},
	} {
		if v, present, err := argString(req.Args, f.key); err != nil {
			s.fail(conn, req, err)
			return
		} else if present {
			*f.dst = v
		}
	}
	titleProvided := hasArg(req.Args, "title") || taskFieldPresent(req.Args["task"], "title")
	scopeProvided := hasArg(req.Args, "scope") || taskFieldPresent(req.Args["task"], "scope")
	statusProvided := hasArg(req.Args, "status") || taskFieldPresent(req.Args["task"], "status")
	if op == "add" || (op == "update" && titleProvided) {
		if strings.TrimSpace(in.Title) == "" {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.title required"})
			return
		}
	}
	if scopeProvided {
		switch strings.TrimSpace(in.Scope) {
		case "", "cycle", "total":
		default:
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.scope must be cycle or total"})
			return
		}
	}
	if statusProvided {
		switch strings.TrimSpace(in.Status) {
		case "", "todo", "doing", "done", "blocked", "retired":
		default:
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.status must be todo, doing, done, blocked, or retired"})
			return
		}
	}
	var task roster.AgentTask
	found := false
	p, exists, err := s.k.UpdateProfile(ref, func(dst *roster.Profile) {
		switch op {
		case "add":
			task = in
			dst.TaskList = append(dst.TaskList, task)
			found = true
		case "update":
			id := strings.TrimSpace(in.ID)
			if id == "" {
				return
			}
			for i := range dst.TaskList {
				if dst.TaskList[i].ID != id {
					continue
				}
				if _, ok := req.Args["title"]; ok || in.Title != "" {
					dst.TaskList[i].Title = in.Title
				}
				if _, ok := req.Args["description"]; ok || in.Description != "" {
					dst.TaskList[i].Description = in.Description
				}
				if _, ok := req.Args["scope"]; ok || in.Scope != "" {
					dst.TaskList[i].Scope = in.Scope
				}
				if _, ok := req.Args["status"]; ok || in.Status != "" {
					dst.TaskList[i].Status = in.Status
				}
				task = dst.TaskList[i]
				found = true
				return
			}
		case "remove", "delete":
			id := strings.TrimSpace(in.ID)
			if id == "" {
				return
			}
			for i := range dst.TaskList {
				if dst.TaskList[i].ID != id {
					continue
				}
				task = dst.TaskList[i]
				dst.TaskList = append(append([]roster.AgentTask{}, dst.TaskList[:i]...), dst.TaskList[i+1:]...)
				found = true
				return
			}
		}
	})
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	if !exists {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown agent: " + ref})
		return
	}
	if !found {
		if strings.TrimSpace(in.ID) == "" && op != "add" {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.id required"})
			return
		}
		if op == "add" {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.title required"})
			return
		}
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown agent task: " + in.ID})
		return
	}
	if op == "add" {
		for _, t := range p.TaskList {
			if t.Title == strings.TrimSpace(in.Title) && (strings.TrimSpace(in.ID) == "" || t.ID == strings.TrimSpace(in.ID)) {
				task = t
			}
		}
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"updated": true,
		"profile": profileView(p),
		"task":    task,
	}})
}

func hasArg(args map[string]any, key string) bool {
	_, ok := args[key]
	return ok
}

func taskFieldPresent(raw any, key string) bool {
	obj, ok := raw.(map[string]any)
	if !ok {
		return false
	}
	_, ok = obj[key]
	return ok
}

// handleAgentImpact reports what depends on an agent — shown before retiring or
// removing so the operator sees the effects (M846).
func (s *Server) handleAgentImpact(conn net.Conn, req Request) {
	ref, err := requiredArgString(req.Args, "ref")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	p, ok := s.k.Roster().Get(ref)
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown agent: " + ref})
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: s.agentImpactResult(p)})
}

// handleAgentTombstone returns a read-only death certificate for an agent: its
// identity, lifecycle/retirement record, and durable resource footprint. Portable
// archival/audit artifact — it removes and mutates nothing (NEXT.md #7).
func (s *Server) handleAgentTombstone(conn net.Conn, req Request) {
	ref, err := requiredArgString(req.Args, "ref")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	p, ok := s.k.Roster().Get(ref)
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown agent: " + ref})
		return
	}
	impact := s.agentImpactResult(p)
	manager := strings.TrimSpace(p.ParentAgent)
	if manager == "" {
		manager = strings.TrimSpace(p.OwnerAgent)
	}
	tombstone := map[string]any{
		"slug":             p.Slug,
		"name":             p.Name,
		"kind":             p.Kind(),
		"system":           p.System,
		"description":      p.Description,
		"manager":          manager,
		"retired":          p.Retired,
		"retired_ms":       p.RetiredMS,
		"retired_reason":   p.RetiredReason,
		"lifecycle_mode":   strings.TrimSpace(p.Lifecycle.Mode),
		"completed_cycles": p.Lifecycle.CompletedCycles,
		"max_cycles":       p.Lifecycle.MaxCycles,
		"memory_scope":     strings.TrimSpace(p.MemoryScope),
		"model":            strings.TrimSpace(p.Model),
		// Durable footprint left behind — the counts the removal cascade would act on.
		"footprint": map[string]any{
			"standing_orders":  impact["standing_count"],
			"schedules":        impact["schedule_count"],
			"memories":         impact["memory_count"],
			"authored_shared":  impact["authored_shared_memory_count"],
			"skills":           impact["skill_count"],
			"configs":          impact["config_count"],
			"workspaces":       impact["workspace_count"],
			"workflow_refs":    impact["workflow_ref_count"],
			"mailbox_messages": impact["mailbox_message_count"],
			"subagents":        impact["subagent_count"],
		},
		// Mailbox/audit messages and workflow refs are retained by design, not
		// deleted, so the tombstone records them as the agent's lasting trace.
		"retained_by_design": map[string]any{
			"mailbox_messages": impact["mailbox_message_count"],
			"workflow_refs":    impact["workflow_ref_count"],
		},
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"tombstone": tombstone}})
}

// handleAgentGraveyard lists retired agents with their retirement age — the
// read-only retention-eligibility view (NEXT.md #7). Optional older_than_days
// filters to long-dead identities. It REPORTS only; archiving / hard-removal stays
// an explicit operator action (no auto-deletion).
func (s *Server) handleAgentGraveyard(conn net.Conn, req Request) {
	var olderThanDays float64
	switch v := req.Args["older_than_days"].(type) {
	case float64:
		olderThanDays = v
	case string:
		olderThanDays, _ = strconv.ParseFloat(strings.TrimSpace(v), 64)
	}
	nowMS := time.Now().UnixMilli()
	cutoffMS := int64(0)
	if olderThanDays > 0 {
		cutoffMS = nowMS - int64(olderThanDays*24*3600*1000)
	}
	rows := make([]map[string]any, 0)
	for _, p := range s.k.Roster().List() {
		if !p.Retired {
			continue
		}
		if cutoffMS > 0 && p.RetiredMS > cutoffMS {
			continue // not yet older than the requested window
		}
		ageDays := 0.0
		if p.RetiredMS > 0 {
			ageDays = float64(nowMS-p.RetiredMS) / (24 * 3600 * 1000)
		}
		rows = append(rows, map[string]any{
			"slug":           p.Slug,
			"name":           p.Name,
			"kind":           p.Kind(),
			"system":         p.System,
			"retired_ms":     p.RetiredMS,
			"retired_reason": p.RetiredReason,
			"age_days":       int(ageDays),
		})
	}
	// Oldest first — the most retention-eligible identities lead.
	sort.SliceStable(rows, func(i, j int) bool {
		return plInt64(rows[i], "retired_ms") < plInt64(rows[j], "retired_ms")
	})
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"graveyard":       rows,
		"count":           len(rows),
		"older_than_days": int(olderThanDays),
	}})
}

func plInt64(m map[string]any, key string) int64 {
	switch n := m[key].(type) {
	case int64:
		return n
	case int:
		return int64(n)
	case float64:
		return int64(n)
	default:
		return 0
	}
}

func (s *Server) handleAgentActivity(conn net.Conn, req Request) {
	ref, err := requiredArgString(req.Args, "ref")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	p, ok := s.k.Roster().Get(ref)
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown agent: " + ref})
		return
	}
	slug := p.Slug
	limit, err := argLimit(req.Args, 50, 500)
	if err != nil {
		s.fail(conn, req, err)
		return
	}

	// Pass 1: the correlation ids of runs this agent executed (task.received
	// carries the agent slug since M854). These also scope the council consults
	// and delegations that happened *during* the agent's runs.
	// Also collect activity events in the same pass to avoid O(2n) journal walks.
	runCorr := map[string]bool{}
	var items []map[string]any
	_ = s.k.Journal().Range(func(e *event.Event) error {
		// Build runCorr map
		if e.Kind == event.KindTaskReceived {
			var pl map[string]any
			if json.Unmarshal(e.Payload, &pl) == nil && plString(pl, "agent") == slug && e.CorrelationID != "" {
				runCorr[e.CorrelationID] = true
			}
		}
		// Check if this event is attributable to the agent
		var pl map[string]any
		_ = json.Unmarshal(e.Payload, &pl)
		summary, ok := agentActivitySummary(e, pl, slug, runCorr)
		if !ok {
			return nil
		}
		items = append(items, map[string]any{
			"seq":            e.Seq,
			"kind":           string(e.Kind),
			"ts_unix_ms":     e.TSUnixMS,
			"correlation_id": e.CorrelationID,
			"summary":        summary,
		})
		return nil
	})

	// Newest first, capped.
	sort.SliceStable(items, func(i, j int) bool {
		return items[i]["seq"].(int64) > items[j]["seq"].(int64)
	})
	total := len(items)

	// Cursor pagination (M-pending follow-up): the SPA's IncidentPage /
	// AgentPage views load this on every poll, and the journal can hold tens
	// of thousands of events. `cursor` is the opaque "<seq>" boundary of the
	// previous page; the server skips entries with seq >= cursorSeq (the list
	// is already sorted DESC, so strictly-older means strictly-smaller seq).
	cursorSeq, cursorOK := parseSeqCursor(stringArg(req.Args, "cursor"))
	if cursorOK {
		filtered := items[:0]
		for _, it := range items {
			if it["seq"].(int64) >= cursorSeq {
				continue
			}
			filtered = append(filtered, it)
		}
		items = filtered
	}
	var nextCursor string
	if limit > 0 && len(items) > limit {
		items = items[:limit]
		nextCursor = strconv.FormatInt(items[limit-1]["seq"].(int64), 10)
	}
	result := map[string]any{
		"slug": slug, "activity": items, "count": len(items), "total": total,
	}
	if nextCursor != "" {
		result["next_cursor"] = nextCursor
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: result})
}

// handleAgentRepairStatus folds the journal into one agent's autonomous
// self-repair history: queued/completed/failed doctor.auto_repair events,
// newest first, plus the current inflight fingerprints and effective cooldown.
func (s *Server) handleAgentRepairStatus(conn net.Conn, req Request) {
	ref, err := requiredArgString(req.Args, "ref")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	p, ok := s.k.Roster().Get(ref)
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown agent: " + ref})
		return
	}
	limit, err := argLimit(req.Args, 20, 100)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	cooldown := agentAutoRepairCooldown()
	var rows []agentRepairRow
	latestByFingerprint := map[string]agentRepairRow{}
	_ = s.k.Journal().Range(func(e *event.Event) error {
		if e.Subject != "doctor.auto_repair" || e.Kind != event.KindInfo {
			return nil
		}
		var pl map[string]any
		if json.Unmarshal(e.Payload, &pl) != nil || plString(pl, "agent") != p.Slug {
			return nil
		}
		row := agentRepairRow{
			Seq:                            e.Seq,
			TSUnixMS:                       e.TSUnixMS,
			Agent:                          plString(pl, "agent"),
			CorrelationID:                  e.CorrelationID,
			Mode:                           plString(pl, "mode"),
			Phase:                          plString(pl, "phase"),
			Reason:                         plString(pl, "reason"),
			Fingerprint:                    plString(pl, "fingerprint"),
			SelfRepairAttempt:              plInt(pl, "self_repair_attempt"),
			SelfRepairMaxAttempts:          plInt(pl, "self_repair_max_attempts"),
			Issues:                         plStrings(pl, "issues"),
			Applied:                        plStrings(pl, "applied"),
			Answer:                         plString(pl, "answer"),
			Error:                          plString(pl, "error"),
			NextEligibleMS:                 e.TSUnixMS + cooldown.Milliseconds(),
			Resolution:                     plString(pl, "resolution"),
			ResolutionSummary:              plString(pl, "resolution_summary"),
			DelegateTo:                     plString(pl, "delegate_to"),
			DelegatedBy:                    plString(pl, "delegated_by"),
			RootAgent:                      plString(pl, "root_agent"),
			ChainDepth:                     intNumber(pl["chain_depth"]),
			IncidentID:                     plString(pl, "incident_id"),
			RootIncidentID:                 plString(pl, "root_incident_id"),
			ParentIncidentID:               plString(pl, "parent_incident_id"),
			RoutingTaskType:                plString(pl, "routing_task_type"),
			RoutingTaskModelChain:          plStrings(pl, "routing_task_model_chain"),
			PreviousRoutingTaskModelChain:  plStrings(pl, "previous_routing_task_model_chain"),
			RoutingForceGeneration:         intNumber(pl["routing_force_generation"]),
			PreviousRoutingForceGeneration: intNumber(pl["previous_routing_force_generation"]),
		}
		rows = append(rows, row)
		if row.Fingerprint != "" {
			latestByFingerprint[row.Fingerprint] = row
		}
		return nil
	})
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Seq > rows[j].Seq })
	total := len(rows)

	// Cursor pagination (M-pending follow-up): `cursor` is the opaque "<seq>"
	// boundary of the previous page; server skips rows with seq >= cursorSeq.
	// Note: the `latest`/`next_eligible_ms` fields below intentionally use the
	// FULL row list (rows[0]) — they reflect the agent's current state, not the
	// page being viewed, so they must not move with cursor pagination.
	cursorSeq, cursorOK := parseSeqCursor(stringArg(req.Args, "cursor"))
	history := rows
	if cursorOK {
		filtered := rows[:0]
		for _, r := range rows {
			if r.Seq >= cursorSeq {
				continue
			}
			filtered = append(filtered, r)
		}
		history = filtered
	}
	var nextCursor string
	if limit > 0 && len(history) > limit {
		history = history[:limit]
		nextCursor = strconv.FormatInt(history[limit-1].Seq, 10)
	}
	inflightRows := make([]agentRepairRow, 0, len(latestByFingerprint))
	for _, row := range latestByFingerprint {
		if row.Phase == "queued" || row.Phase == "routing_rollback_queued" {
			inflightRows = append(inflightRows, row)
		}
	}
	sort.SliceStable(inflightRows, func(i, j int) bool { return inflightRows[i].Seq > inflightRows[j].Seq })

	result := map[string]any{
		"slug":           p.Slug,
		"cooldown_sec":   int(cooldown / time.Second),
		"contract":       agentRepairContractView(p, cooldown),
		"history":        repairRowsView(history),
		"count":          len(history),
		"total":          total,
		"inflight":       repairRowsView(inflightRows),
		"inflight_count": len(inflightRows),
	}
	if nextCursor != "" {
		result["next_cursor"] = nextCursor
	}
	if len(rows) > 0 {
		result["latest"] = repairRowView(rows[0])
		result["next_eligible_ms"] = rows[0].NextEligibleMS
	}
	result["next_action"] = agentRepairNextActionView(p, rows, inflightRows, time.Now().UnixMilli())
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: result})
}

func agentRepairContractView(p roster.Profile, cooldown time.Duration) map[string]any {
	retryAttempts := 1
	retryBackoff := "none"
	retryOn := []string{"error", "timeout"}
	if p.RetryPolicy != nil {
		if p.RetryPolicy.MaxAttempts > 0 {
			retryAttempts = p.RetryPolicy.MaxAttempts
		}
		if strings.TrimSpace(p.RetryPolicy.Backoff) != "" {
			retryBackoff = strings.TrimSpace(p.RetryPolicy.Backoff)
		}
		if len(p.RetryPolicy.RetryOn) > 0 {
			retryOn = append([]string(nil), p.RetryPolicy.RetryOn...)
		}
	}
	selfRepairEnabled := p.SelfRepairPolicy != nil && p.SelfRepairPolicy.Enabled
	selfRepairMax := 0
	escalateTo := ""
	if p.SelfRepairPolicy != nil {
		selfRepairMax = p.SelfRepairPolicy.MaxAttempts
		escalateTo = strings.TrimSpace(p.SelfRepairPolicy.EscalateTo)
	}
	doctor := ""
	failureThreshold := 0
	if p.HealthPolicy != nil {
		doctor = strings.TrimSpace(p.HealthPolicy.DoctorAgent)
		failureThreshold = p.HealthPolicy.FailureThreshold
	}
	return map[string]any{
		"retry_attempts":       retryAttempts,
		"retry_backoff":        retryBackoff,
		"retry_on":             retryOn,
		"doctor_agent":         doctor,
		"failure_threshold":    failureThreshold,
		"self_repair_enabled":  selfRepairEnabled,
		"self_repair_attempts": selfRepairMax,
		"escalate_to":          escalateTo,
		"cooldown_sec":         int(cooldown / time.Second),
		"authority_boundary":   "agent identity owns retry, doctor, self-repair and escalation; schedules/workflows only wake this contract",
	}
}

func agentRepairNextActionView(p roster.Profile, rows, inflight []agentRepairRow, nowMS int64) map[string]any {
	action := "manual_repair"
	label := "manual repair"
	detail := "no autonomous repair is currently queued"
	tone := "muted"
	if p.Retired {
		return map[string]any{"action": "revive_required", "label": "revive required", "detail": "graveyard agent cannot repair until revived", "tone": "muted"}
	}
	if !p.Enabled {
		return map[string]any{"action": "resume_required", "label": "resume required", "detail": "paused agent cannot repair until resumed", "tone": "warn"}
	}
	if len(inflight) > 0 {
		row := inflight[0]
		return map[string]any{
			"action":         "wait_inflight",
			"label":          "repair in flight",
			"detail":         repairDecisionDetail(row, "doctor/self-repair run is already queued"),
			"tone":           "accent",
			"correlation_id": row.CorrelationID,
			"fingerprint":    row.Fingerprint,
			"phase":          row.Phase,
		}
	}
	var latest agentRepairRow
	if len(rows) > 0 {
		latest = rows[0]
		if latest.NextEligibleMS > nowMS {
			return map[string]any{
				"action":           "cooldown",
				"label":            "cooldown active",
				"detail":           repairDecisionDetail(latest, "wait before another autonomous repair attempt"),
				"tone":             "warn",
				"next_eligible_ms": latest.NextEligibleMS,
				"phase":            latest.Phase,
				"fingerprint":      latest.Fingerprint,
			}
		}
		switch strings.TrimSpace(latest.Phase) {
		case "attempts_exhausted", "resolution_failed", "routing_rollback_failed", "failed":
			target := firstNonEmpty(strings.TrimSpace(latest.DelegateTo), repairEscalationOwner(p))
			if target != "" {
				return map[string]any{
					"action":      "escalate_owner",
					"label":       "escalate owner",
					"detail":      repairDecisionDetail(latest, "self-repair failed; owner should take over"),
					"tone":        "bad",
					"delegate_to": target,
					"phase":       latest.Phase,
				}
			}
			return map[string]any{
				"action": "operator_resolution",
				"label":  "operator resolution",
				"detail": repairDecisionDetail(latest, "repair failed and no owner escalation target is configured"),
				"tone":   "bad",
				"phase":  latest.Phase,
			}
		}
	}
	if p.SelfRepairPolicy != nil && p.SelfRepairPolicy.Enabled {
		action = "run_self_repair"
		label = "self-repair eligible"
		detail = "next failure can trigger autonomous self-repair"
		tone = "good"
	} else if p.HealthPolicy != nil && strings.TrimSpace(p.HealthPolicy.DoctorAgent) != "" {
		action = "doctor_monitor"
		label = "doctor monitoring"
		detail = "doctor can queue repair after health threshold"
		tone = "good"
	}
	if latest.Phase != "" {
		detail = repairDecisionDetail(latest, detail)
	}
	return map[string]any{"action": action, "label": label, "detail": detail, "tone": tone}
}

func repairEscalationOwner(p roster.Profile) string {
	if p.SelfRepairPolicy != nil && strings.TrimSpace(p.SelfRepairPolicy.EscalateTo) != "" {
		return strings.TrimSpace(p.SelfRepairPolicy.EscalateTo)
	}
	return firstNonEmpty(strings.TrimSpace(p.ParentAgent), strings.TrimSpace(p.OwnerAgent))
}

func repairDecisionDetail(row agentRepairRow, fallback string) string {
	parts := []string{fallback}
	if row.Mode != "" {
		parts = append(parts, "mode "+row.Mode)
	}
	if row.Phase != "" {
		parts = append(parts, "phase "+row.Phase)
	}
	if row.Fingerprint != "" {
		parts = append(parts, "fingerprint "+row.Fingerprint)
	}
	if row.Reason != "" {
		parts = append(parts, row.Reason)
	} else if row.Error != "" {
		parts = append(parts, row.Error)
	}
	if row.SelfRepairAttempt > 0 && row.SelfRepairMaxAttempts > 0 {
		parts = append(parts, fmt.Sprintf("attempt %d/%d", row.SelfRepairAttempt, row.SelfRepairMaxAttempts))
	}
	return strings.Join(parts, " · ")
}

func (s *Server) handleAgentRepair(conn net.Conn, req Request) {
	ref, err := requiredArgString(req.Args, "ref")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	p, ok := s.k.Roster().Get(ref)
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown agent: " + ref})
		return
	}
	if p.Retired {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "agent " + p.Slug + " is retired — revive it first"})
		return
	}
	if !p.Enabled {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "agent " + p.Slug + " is paused"})
		return
	}
	if !p.AllowsDirectCall() {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: managedSubagentDirectCallError(p, "repaired")})
		return
	}
	corr := s.k.NewCorrelation()
	reason := strings.TrimSpace(stringArg(req.Args, "reason"))
	lineage := operatorIncidentLineage(req.Args)
	publishOperatorAction(s.k, "agent.repair", corr, map[string]any{
		"phase":              "requested",
		"agent":              p.Slug,
		"reason":             reason,
		"incident_id":        lineage.incidentID,
		"root_incident_id":   lineage.rootIncidentID,
		"parent_incident_id": lineage.parentIncidentID,
	})
	go func() {
		src := overseertool.NewKernelSource(s.k, s.baseDir)
		res, err := src.RepairAgent(p.Slug, reason)
		if err != nil {
			publishOperatorAction(s.k, "agent.repair", corr, map[string]any{
				"phase":              "failed",
				"agent":              p.Slug,
				"reason":             reason,
				"error":              err.Error(),
				"incident_id":        lineage.incidentID,
				"root_incident_id":   lineage.rootIncidentID,
				"parent_incident_id": lineage.parentIncidentID,
			})
			return
		}
		publishOperatorAction(s.k, "agent.repair", corr, map[string]any{
			"phase":                             "completed",
			"agent":                             p.Slug,
			"reason":                            reason,
			"applied":                           res.Applied,
			"routing_task_type":                 res.RoutingTaskType,
			"routing_task_model_chain":          res.RoutingTaskModelChain,
			"previous_routing_task_model_chain": res.PreviousRoutingTaskModelChain,
			"answer":                            truncate(res.Answer, 300),
			"incident_id":                       lineage.incidentID,
			"root_incident_id":                  lineage.rootIncidentID,
			"parent_incident_id":                lineage.parentIncidentID,
		})
	}()
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"accepted":       true,
		"agent":          p.Slug,
		"correlation_id": corr,
	}})
}

func (s *Server) handleAgentWake(conn net.Conn, req Request) {
	ref, err := requiredArgString(req.Args, "ref")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	p, ok := s.k.Roster().Get(ref)
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown agent: " + ref})
		return
	}
	if p.Retired {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "agent " + p.Slug + " is retired — revive it first"})
		return
	}
	if !p.Enabled {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "agent " + p.Slug + " is paused"})
		return
	}
	if !p.AllowsDirectCall() {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: managedSubagentDirectCallError(p, "called")})
		return
	}
	intent, _, ierr := argString(req.Args, "intent")
	if ierr != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: ierr.Error()})
		return
	}
	reason := strings.TrimSpace(stringArg(req.Args, "reason"))
	intent = buildOperatorWakeIntent(strings.TrimSpace(intent), p.Slug, reason, req.Args)
	if strings.TrimSpace(intent) == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "agent wake requires args.intent or args.reason"})
		return
	}
	corr := s.k.NewCorrelation()
	lineage := operatorIncidentLineage(req.Args)
	runbook := agentAutonomyRunbookPayload(p)
	publishOperatorAction(s.k, "agent.wake", corr, map[string]any{
		"phase":              "requested",
		"agent":              p.Slug,
		"reason":             reason,
		"intent":             truncate(intent, 240),
		"autonomy_runbook":   runbook,
		"incident_id":        lineage.incidentID,
		"root_incident_id":   lineage.rootIncidentID,
		"parent_incident_id": lineage.parentIncidentID,
	})
	go s.runAgentWake(corr, p, intent, reason, lineage)
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"accepted":       true,
		"agent":          p.Slug,
		"correlation_id": corr,
	}})
}

func (s *Server) handleAgentResolve(conn net.Conn, req Request) {
	ref, err := requiredArgString(req.Args, "ref")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	p, ok := s.k.Roster().Get(ref)
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown agent: " + ref})
		return
	}
	resolution := strings.TrimSpace(stringArg(req.Args, "resolution"))
	switch resolution {
	case "paused", "retired", "delegated", "force_chain":
	default:
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.resolution must be paused, retired, delegated, or force_chain"})
		return
	}
	summary := strings.TrimSpace(stringArg(req.Args, "summary"))
	lineage := operatorIncidentLineage(req.Args)
	corr := s.k.NewCorrelation()
	requested := map[string]any{
		"phase":              "requested",
		"agent":              p.Slug,
		"resolution":         resolution,
		"resolution_summary": summary,
		"incident_id":        lineage.incidentID,
		"root_incident_id":   lineage.rootIncidentID,
		"parent_incident_id": lineage.parentIncidentID,
	}
	if delegateTo := strings.TrimSpace(stringArg(req.Args, "delegate_to")); delegateTo != "" {
		requested["delegate_to"] = delegateTo
	}
	if taskType := strings.TrimSpace(stringArg(req.Args, "task_type")); taskType != "" {
		requested["routing_task_type"] = taskType
	}
	if chain, ok := req.Args["task_model_chain"].([]any); ok && len(chain) > 0 {
		requested["routing_task_model_chain"] = normalizeTaskModelChain(chain)
	}
	publishOperatorAction(s.k, "agent.resolve", corr, requested)

	result, err := s.applyAgentResolution(p, resolution, summary, req.Args)
	if err != nil {
		fail := map[string]any{
			"phase":              "failed",
			"agent":              p.Slug,
			"resolution":         resolution,
			"resolution_summary": summary,
			"reason":             err.Error(),
			"incident_id":        lineage.incidentID,
			"root_incident_id":   lineage.rootIncidentID,
			"parent_incident_id": lineage.parentIncidentID,
		}
		if result.delegateTo != "" {
			fail["delegate_to"] = result.delegateTo
		}
		if result.taskType != "" {
			fail["routing_task_type"] = result.taskType
		}
		if len(result.taskModelChain) > 0 {
			fail["routing_task_model_chain"] = result.taskModelChain
		}
		publishOperatorAction(s.k, "agent.resolve", corr, fail)
		s.fail(conn, req, err)
		return
	}
	completed := map[string]any{
		"phase":              "completed",
		"agent":              p.Slug,
		"resolution":         resolution,
		"resolution_summary": summary,
		"incident_id":        lineage.incidentID,
		"root_incident_id":   lineage.rootIncidentID,
		"parent_incident_id": lineage.parentIncidentID,
	}
	if result.delegateTo != "" {
		completed["delegate_to"] = result.delegateTo
	}
	if result.messageID != "" {
		completed["message_id"] = result.messageID
	}
	if result.taskType != "" {
		completed["routing_task_type"] = result.taskType
	}
	if len(result.taskModelChain) > 0 {
		completed["routing_task_model_chain"] = result.taskModelChain
	}
	if len(result.previousTaskModelChain) > 0 {
		completed["previous_routing_task_model_chain"] = result.previousTaskModelChain
	}
	if result.routingForceGeneration > 0 {
		completed["routing_force_generation"] = result.routingForceGeneration
	}
	if result.previousRoutingForceGeneration > 0 {
		completed["previous_routing_force_generation"] = result.previousRoutingForceGeneration
	}
	publishOperatorAction(s.k, "agent.resolve", corr, completed)
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"applied":        true,
		"agent":          p.Slug,
		"resolution":     resolution,
		"correlation_id": corr,
	}})
}

type appliedAgentResolution struct {
	delegateTo                     string
	messageID                      string
	taskType                       string
	taskModelChain                 []string
	previousTaskModelChain         []string
	routingForceGeneration         int
	previousRoutingForceGeneration int
}

type routingChainApplier interface {
	ApplyRoutingChain(ref, taskType string, targetChain []string, reason string) (overseertool.RepairResult, error)
}

func (s *Server) applyAgentResolution(p roster.Profile, resolution, summary string, args map[string]any) (appliedAgentResolution, error) {
	switch resolution {
	case "paused":
		if p.Retired {
			return appliedAgentResolution{}, fmt.Errorf("agent %s is retired — revive it first", p.Slug)
		}
		_, err := s.k.SetProfileEnabled(p.Slug, false)
		return appliedAgentResolution{}, err
	case "retired":
		reason := summary
		if reason == "" {
			reason = "retired by operator incident resolution"
		}
		_, err := s.k.SetProfileRetired(p.Slug, true, reason)
		return appliedAgentResolution{}, err
	case "delegated":
		target := strings.TrimSpace(stringArg(args, "delegate_to"))
		if err := s.validateOperatorDelegateTarget(p, target); err != nil {
			return appliedAgentResolution{}, err
		}
		st, ok := s.boardWriter()
		if !ok {
			return appliedAgentResolution{}, fmt.Errorf("the board is not available on this daemon")
		}
		text := strings.TrimSpace(summary)
		if text == "" {
			text = "Operator delegated this incident for ownership review."
		}
		msg, err := st.HelpRequest("operator", target, text, time.Now().UnixMilli())
		if err != nil {
			return appliedAgentResolution{}, err
		}
		if s.boardNotify != nil {
			s.boardNotify(msg, "")
		}
		return appliedAgentResolution{delegateTo: target, messageID: strings.TrimSpace(msg.ID)}, nil
	case "force_chain":
		taskType := strings.TrimSpace(stringArg(args, "task_type"))
		chain := normalizeTaskModelChain(argListAny(args["task_model_chain"]))
		if taskType == "" || len(chain) == 0 {
			return appliedAgentResolution{}, fmt.Errorf("force_chain resolution requires task_type and task_model_chain")
		}
		if exhausted := latestExhaustedRoutingChain(s.k, p.Slug, operatorIncidentLineage(args), taskType); len(exhausted) > 0 && equalStringSlices(exhausted, chain) {
			return appliedAgentResolution{}, fmt.Errorf("force_chain resolution must choose a new chain for exhausted routing policy")
		}
		src, ok := overseertool.NewKernelSource(s.k, s.baseDir).(routingChainApplier)
		if !ok {
			return appliedAgentResolution{}, fmt.Errorf("force_chain resolution is not supported by the active repair source")
		}
		prevGen := latestOperatorForceGeneration(s.k, p.Slug, taskType)
		res, err := src.ApplyRoutingChain(p.Slug, taskType, chain, summary)
		if err != nil {
			return appliedAgentResolution{}, err
		}
		return appliedAgentResolution{
			taskType:                       firstNonEmpty(res.RoutingTaskType, taskType),
			taskModelChain:                 append([]string(nil), firstNonEmptyStrings(res.RoutingTaskModelChain, chain)...),
			previousTaskModelChain:         append([]string(nil), res.PreviousRoutingTaskModelChain...),
			routingForceGeneration:         prevGen + 1,
			previousRoutingForceGeneration: prevGen,
		}, nil
	default:
		return appliedAgentResolution{}, nil
	}
}

func latestOperatorForceGeneration(k *runtime.Kernel, slug, taskType string) int {
	if k == nil || strings.TrimSpace(slug) == "" || strings.TrimSpace(taskType) == "" {
		return 0
	}
	best := 0
	_ = k.Journal().Range(func(e *event.Event) error {
		if e.Kind != event.KindInfo || (e.Subject != "doctor.auto_repair" && e.Subject != "agent.resolve") {
			return nil
		}
		var pl map[string]any
		if json.Unmarshal(e.Payload, &pl) != nil {
			return nil
		}
		if strings.TrimSpace(plString(pl, "agent")) != slug || strings.TrimSpace(plString(pl, "resolution")) != "force_chain" {
			return nil
		}
		phase := strings.TrimSpace(plString(pl, "phase"))
		if phase != "resolution_applied" && phase != "completed" {
			return nil
		}
		if strings.TrimSpace(plString(pl, "routing_task_type")) != taskType {
			return nil
		}
		if gen := intNumber(pl["routing_force_generation"]); gen > best {
			best = gen
		}
		return nil
	})
	return best
}

func (s *Server) validateOperatorDelegateTarget(p roster.Profile, target string) error {
	target = strings.TrimSpace(target)
	if target == "" {
		return fmt.Errorf("delegated resolution requires delegate_to")
	}
	if strings.EqualFold(target, strings.TrimSpace(p.Slug)) {
		return fmt.Errorf("delegated resolution points back to the root agent %s", p.Slug)
	}
	if owner := firstNonEmpty(p.ParentAgent, p.OwnerAgent); owner != "" && strings.EqualFold(target, owner) {
		return fmt.Errorf("delegated resolution points back to the current owner %s", owner)
	}
	dst, ok := s.k.Roster().Get(target)
	if !ok {
		return fmt.Errorf("delegated resolution target %s does not exist", target)
	}
	if dst.Retired {
		return fmt.Errorf("delegated resolution target %s is retired", dst.Slug)
	}
	if !dst.AllowsDirectCall() {
		return fmt.Errorf("delegated resolution target %s is a managed sub-agent", dst.Slug)
	}
	return nil
}

func latestExhaustedRoutingChain(k *runtime.Kernel, slug string, lineage operatorWakeLineage, taskType string) []string {
	if k == nil || strings.TrimSpace(slug) == "" || strings.TrimSpace(taskType) == "" || !lineage.hasAny() {
		return nil
	}
	var bestChain []string
	var bestSeq int64
	_ = k.Journal().Range(func(e *event.Event) error {
		if e.Kind != event.KindInfo || e.Subject != "doctor.auto_repair" {
			return nil
		}
		var pl map[string]any
		if json.Unmarshal(e.Payload, &pl) != nil {
			return nil
		}
		if !strings.EqualFold(strings.TrimSpace(plString(pl, "agent")), slug) {
			return nil
		}
		if strings.TrimSpace(plString(pl, "phase")) != "routing_force_exhausted_detected" {
			return nil
		}
		if !strings.EqualFold(strings.TrimSpace(plString(pl, "routing_task_type")), taskType) {
			return nil
		}
		if !incidentLineageMatchesPayload(lineage, pl) || e.Seq <= bestSeq {
			return nil
		}
		bestSeq = e.Seq
		bestChain = plStrings(pl, "routing_task_model_chain")
		return nil
	})
	return append([]string(nil), bestChain...)
}

func incidentLineageMatchesPayload(lineage operatorWakeLineage, pl map[string]any) bool {
	if !lineage.hasAny() {
		return false
	}
	payloadIDs := []string{
		strings.TrimSpace(plString(pl, "incident_id")),
		strings.TrimSpace(plString(pl, "root_incident_id")),
		strings.TrimSpace(plString(pl, "parent_incident_id")),
	}
	return incidentIDInSlice(lineage.incidentID, payloadIDs) ||
		incidentIDInSlice(lineage.rootIncidentID, payloadIDs) ||
		incidentIDInSlice(lineage.parentIncidentID, payloadIDs)
}

func incidentIDInSlice(id string, items []string) bool {
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	for _, item := range items {
		if strings.EqualFold(id, strings.TrimSpace(item)) {
			return true
		}
	}
	return false
}

func (l operatorWakeLineage) hasAny() bool {
	return strings.TrimSpace(l.incidentID) != "" ||
		strings.TrimSpace(l.rootIncidentID) != "" ||
		strings.TrimSpace(l.parentIncidentID) != ""
}

func argListAny(v any) []any {
	if raw, ok := v.([]any); ok {
		return raw
	}
	return nil
}

func normalizeTaskModelChain(raw []any) []string {
	if len(raw) == 0 {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		switch v := item.(type) {
		case string:
			if v = strings.TrimSpace(v); v != "" {
				out = append(out, v)
			}
		}
	}
	return out
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !strings.EqualFold(strings.TrimSpace(a[i]), strings.TrimSpace(b[i])) {
			return false
		}
	}
	return true
}

func (s *Server) handleAgentEscalations(conn net.Conn, req Request) {
	ref, err := requiredArgString(req.Args, "ref")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	p, ok := s.k.Roster().Get(ref)
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown agent: " + ref})
		return
	}
	limit, err := argLimit(req.Args, 20, 100)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	st, err := s.boardReader()
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	// Cursor pagination (M-pending follow-up): `cursor` is the opaque
	// "<ts_unix_ms>:<message_id>" boundary of the previous page; server skips
	// entries strictly newer-or-equal. ts can collide across messages, so the
	// message_id is the tie-break.
	var cursorTS int64
	var cursorID string
	cursorOK := false
	if raw, _, cerr := argString(req.Args, "cursor"); cerr != nil {
		s.fail(conn, req, cerr)
		return
	} else if raw != "" {
		tsStr, id, _ := strings.Cut(raw, ":")
		if ts, perr := strconv.ParseInt(tsStr, 10, 64); perr == nil {
			cursorTS, cursorID, cursorOK = ts, id, true
		}
	}
	if !cursorOK {
		cursorTS, cursorID = 0, ""
	}
	rows, nextCursor := s.agentEscalationRows(st, p.Slug, limit, cursorTS, cursorID)
	openCount := 0
	for _, row := range rows {
		if row.Status == "open" {
			openCount++
		}
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, map[string]any{
			"message_id":          row.MessageID,
			"from":                row.From,
			"to":                  row.To,
			"text":                row.Text,
			"ts_unix_ms":          row.TSUnixMS,
			"status":              row.Status,
			"reply_count":         row.ReplyCount,
			"acked":               row.Acked,
			"source_agent":        row.SourceAgent,
			"mode":                row.Mode,
			"wake_phase":          row.WakePhase,
			"wake_reason":         row.WakeReason,
			"wake_error":          row.WakeError,
			"wake_correlation_id": row.WakeCorrelationID,
			"fingerprint":         row.Fingerprint,
			"resolution":          row.Resolution,
			"resolution_summary":  row.ResolutionSummary,
			"delegate_to":         row.DelegateTo,
			"origin_kind":         row.OriginKind,
			"origin_agent":        row.OriginAgent,
			"root_agent":          row.RootAgent,
			"chain_depth":         row.ChainDepth,
			"incident_id":         row.IncidentID,
			"root_incident_id":    row.RootIncidentID,
			"parent_incident_id":  row.ParentIncidentID,
		})
	}
	result := map[string]any{
		"slug":        p.Slug,
		"escalations": out,
		"count":       len(out),
		"open_count":  openCount,
	}
	if nextCursor != "" {
		result["next_cursor"] = nextCursor
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: result})
}

type operatorWakeLineage struct {
	incidentID       string
	rootIncidentID   string
	parentIncidentID string
}

func operatorIncidentLineage(args map[string]any) operatorWakeLineage {
	return operatorWakeLineage{
		incidentID:       strings.TrimSpace(stringArg(args, "incident_id")),
		rootIncidentID:   strings.TrimSpace(stringArg(args, "root_incident_id")),
		parentIncidentID: strings.TrimSpace(stringArg(args, "parent_incident_id")),
	}
}

// agentAutonomyRunbookPayload delegates to the canonical roster builder so manual
// operator wakes share the exact runbook shape as schedule/standing/delegated wakes.
func agentAutonomyRunbookPayload(p roster.Profile) map[string]any {
	return roster.AutonomyRunbook(p)
}

func publishOperatorAction(k *runtime.Kernel, subject, corr string, payload map[string]any) {
	if k == nil || k.Bus() == nil {
		return
	}
	_, _ = k.Bus().Publish(event.Spec{
		Subject:       subject,
		Kind:          event.KindInfo,
		Actor:         "controlplane",
		CorrelationID: corr,
		Payload:       payload,
	})
}

func buildOperatorWakeIntent(explicit, slug, reason string, args map[string]any) string {
	if text := strings.TrimSpace(explicit); text != "" {
		return text
	}
	var b strings.Builder
	b.WriteString("Manual wake-up.\n")
	b.WriteString("You are agent ")
	b.WriteString(slug)
	b.WriteString(". You were explicitly woken by the operator/control plane.\n")
	if reason = strings.TrimSpace(reason); reason != "" {
		b.WriteString("Reason: ")
		b.WriteString(reason)
		b.WriteString("\n")
	}
	if root := strings.TrimSpace(stringArg(args, "root_incident_id")); root != "" {
		b.WriteString("Incident root: ")
		b.WriteString(root)
		b.WriteString("\n")
	}
	if incident := strings.TrimSpace(stringArg(args, "incident_id")); incident != "" {
		b.WriteString("Incident hop: ")
		b.WriteString(incident)
		b.WriteString("\n")
	}
	b.WriteString("Inspect your durable instructions, memory, mailbox, tasklist, and current health context. Do the next concrete recovery step and then stop.")
	return b.String()
}

func (s *Server) runAgentWake(corr string, p roster.Profile, intent, reason string, lineage operatorWakeLineage) {
	runbook := agentAutonomyRunbookPayload(p)
	ctx := runtime.WithAgentProfile(context.Background(), p)
	ctx = runtime.WithWakeContext(ctx, runtime.WakeContext{
		Source: "operator",
		Reason: reason,
	})
	if p.MaxCostMc > 0 {
		ctx = runtime.WithMaxCost(ctx, p.MaxCostMc)
	}
	var (
		answer string
		err    error
	)
	if p.RetryPolicy != nil && p.RetryPolicy.MaxAttempts > 1 {
		answer, err = s.k.RunWithRetry(ctx, corr, intent, *p.RetryPolicy)
	} else {
		answer, err = s.k.RunWith(ctx, corr, intent)
	}
	if err != nil {
		publishOperatorAction(s.k, "agent.wake", corr, map[string]any{
			"phase":              "failed",
			"agent":              p.Slug,
			"reason":             reason,
			"error":              err.Error(),
			"autonomy_runbook":   runbook,
			"incident_id":        lineage.incidentID,
			"root_incident_id":   lineage.rootIncidentID,
			"parent_incident_id": lineage.parentIncidentID,
		})
		return
	}
	publishOperatorAction(s.k, "agent.wake", corr, map[string]any{
		"phase":              "completed",
		"agent":              p.Slug,
		"reason":             reason,
		"answer":             truncate(answer, 300),
		"autonomy_runbook":   runbook,
		"incident_id":        lineage.incidentID,
		"root_incident_id":   lineage.rootIncidentID,
		"parent_incident_id": lineage.parentIncidentID,
	})
}

// agentActivitySummary decides whether one event belongs in an agent's timeline
// and renders a one-line summary. Attribution is by the slug fields the events
// already carry, plus the agent's own run correlations for run-scoped events.
func agentAutoRepairCooldown() time.Duration {
	raw := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "AUTO_REPAIR_COOLDOWN"))
	if raw == "" {
		return 30 * time.Minute
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return 30 * time.Minute
	}
	return d
}

func (s *Server) agentRepairSummaries() map[string]agentRepairSummary {
	cooldown := agentAutoRepairCooldown()
	latestBySlug := map[string]agentRepairRow{}
	latestBySlugFingerprint := map[string]map[string]agentRepairRow{}
	_ = s.k.Journal().Range(func(e *event.Event) error {
		if e.Subject != "doctor.auto_repair" || e.Kind != event.KindInfo {
			return nil
		}
		var pl map[string]any
		if json.Unmarshal(e.Payload, &pl) != nil {
			return nil
		}
		slug := plString(pl, "agent")
		if strings.TrimSpace(slug) == "" {
			return nil
		}
		row := agentRepairRow{
			Seq:                            e.Seq,
			TSUnixMS:                       e.TSUnixMS,
			CorrelationID:                  e.CorrelationID,
			Mode:                           plString(pl, "mode"),
			Phase:                          plString(pl, "phase"),
			Reason:                         plString(pl, "reason"),
			Fingerprint:                    plString(pl, "fingerprint"),
			SelfRepairAttempt:              plInt(pl, "self_repair_attempt"),
			SelfRepairMaxAttempts:          plInt(pl, "self_repair_max_attempts"),
			Issues:                         plStrings(pl, "issues"),
			Applied:                        plStrings(pl, "applied"),
			Answer:                         plString(pl, "answer"),
			Error:                          plString(pl, "error"),
			TargetAgent:                    plString(pl, "target_agent"),
			TargetCorr:                     plString(pl, "target_correlation"),
			MailboxMessage:                 plString(pl, "mailbox_message_id"),
			Resolution:                     plString(pl, "resolution"),
			ResolutionSummary:              plString(pl, "resolution_summary"),
			DelegateTo:                     plString(pl, "delegate_to"),
			DelegatedBy:                    plString(pl, "delegated_by"),
			RootAgent:                      plString(pl, "root_agent"),
			ChainDepth:                     intNumber(pl["chain_depth"]),
			IncidentID:                     plString(pl, "incident_id"),
			RootIncidentID:                 plString(pl, "root_incident_id"),
			ParentIncidentID:               plString(pl, "parent_incident_id"),
			NextEligibleMS:                 e.TSUnixMS + cooldown.Milliseconds(),
			RoutingTaskType:                plString(pl, "routing_task_type"),
			RoutingTaskModelChain:          plStrings(pl, "routing_task_model_chain"),
			PreviousRoutingTaskModelChain:  plStrings(pl, "previous_routing_task_model_chain"),
			RoutingForceGeneration:         intNumber(pl["routing_force_generation"]),
			PreviousRoutingForceGeneration: intNumber(pl["previous_routing_force_generation"]),
		}
		if cur, ok := latestBySlug[slug]; !ok || row.Seq > cur.Seq {
			latestBySlug[slug] = row
		}
		if row.Fingerprint != "" {
			if latestBySlugFingerprint[slug] == nil {
				latestBySlugFingerprint[slug] = map[string]agentRepairRow{}
			}
			if cur, ok := latestBySlugFingerprint[slug][row.Fingerprint]; !ok || row.Seq > cur.Seq {
				latestBySlugFingerprint[slug][row.Fingerprint] = row
			}
		}
		return nil
	})
	out := map[string]agentRepairSummary{}
	for slug, latest := range latestBySlug {
		sum := agentRepairSummary{Latest: latest, HasLatest: true}
		for _, row := range latestBySlugFingerprint[slug] {
			if row.Phase == "queued" || row.Phase == "routing_rollback_queued" {
				sum.InflightCount++
			}
		}
		out[slug] = sum
	}
	return out
}

func repairPhaseLabel(mode, phase string) string {
	mode = strings.TrimSpace(mode)
	switch strings.TrimSpace(phase) {
	case "routing_forced_failed_detected":
		return "forced chain failed"
	case "routing_force_exhausted_detected":
		return "forced chain exhausted"
	case "routing_unstable_detected":
		return "unstable routing"
	case "attempts_exhausted":
		return "repair exhausted"
	case "queued":
		if mode == "routing_unstable" {
			return "unstable routing"
		}
		if mode == "degraded" {
			return "doctor queued"
		}
		if mode == "routing" {
			return "routing queued"
		}
		return "repair queued"
	case "routing_rollback_queued":
		return "rollback queued"
	case "completed":
		if mode == "degraded" {
			return "doctor repaired"
		}
		if mode == "routing" {
			return "routing stabilized"
		}
		return "repaired"
	case "routing_rollback_completed":
		return "rolled back"
	case "failed":
		if mode == "degraded" {
			return "doctor failed"
		}
		if mode == "routing" {
			return "routing failed"
		}
		return "repair failed"
	case "routing_rollback_failed":
		return "rollback failed"
	case "escalation_answered":
		return "manager answered"
	case "resolution_applied":
		return "manager applied"
	case "escalation_woke":
		return "manager woke"
	case "escalation_skipped":
		return "wake skipped"
	case "escalation_failed":
		return "wake failed"
	case "resolution_failed":
		return "resolution failed"
	case "delegation_queued":
		return "delegation queued"
	case "delegation_woke":
		return "delegation woke"
	case "delegation_failed":
		return "delegation failed"
	default:
		if strings.TrimSpace(phase) == "" {
			return "idle"
		}
		return phase
	}
}

func repairRowsView(rows []agentRepairRow) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, repairRowView(row))
	}
	return out
}

func repairRowView(row agentRepairRow) map[string]any {
	return map[string]any{
		"seq":                               row.Seq,
		"ts_unix_ms":                        row.TSUnixMS,
		"correlation_id":                    row.CorrelationID,
		"mode":                              row.Mode,
		"phase":                             row.Phase,
		"reason":                            row.Reason,
		"fingerprint":                       row.Fingerprint,
		"self_repair_attempt":               row.SelfRepairAttempt,
		"self_repair_max_attempts":          row.SelfRepairMaxAttempts,
		"issues":                            row.Issues,
		"applied":                           row.Applied,
		"answer":                            row.Answer,
		"error":                             row.Error,
		"target_agent":                      row.TargetAgent,
		"target_correlation":                row.TargetCorr,
		"mailbox_message_id":                row.MailboxMessage,
		"resolution":                        row.Resolution,
		"resolution_summary":                row.ResolutionSummary,
		"delegate_to":                       row.DelegateTo,
		"delegated_by":                      row.DelegatedBy,
		"root_agent":                        row.RootAgent,
		"chain_depth":                       row.ChainDepth,
		"incident_id":                       row.IncidentID,
		"root_incident_id":                  row.RootIncidentID,
		"parent_incident_id":                row.ParentIncidentID,
		"next_eligible_ms":                  row.NextEligibleMS,
		"routing_task_type":                 row.RoutingTaskType,
		"routing_task_model_chain":          row.RoutingTaskModelChain,
		"previous_routing_task_model_chain": row.PreviousRoutingTaskModelChain,
		"routing_force_generation":          row.RoutingForceGeneration,
		"previous_routing_force_generation": row.PreviousRoutingForceGeneration,
	}
}

func (s *Server) agentEscalationRows(st *board.Store, slug string, limit int, cursorTS int64, cursorID string) ([]agentEscalationRow, string) {
	msgs := st.Read("help", boardReadMaxLimit)
	metaByMessage := map[string]agentRepairRow{}
	_ = s.k.Journal().Range(func(e *event.Event) error {
		if e.Subject != "doctor.auto_repair" || e.Kind != event.KindInfo {
			return nil
		}
		var pl map[string]any
		if json.Unmarshal(e.Payload, &pl) != nil {
			return nil
		}
		if plString(pl, "target_agent") != slug {
			return nil
		}
		msgID := plString(pl, "mailbox_message_id")
		if strings.TrimSpace(msgID) == "" {
			return nil
		}
		row := agentRepairRow{
			Seq:                            e.Seq,
			TSUnixMS:                       e.TSUnixMS,
			Agent:                          plString(pl, "agent"),
			CorrelationID:                  e.CorrelationID,
			Mode:                           plString(pl, "mode"),
			Phase:                          plString(pl, "phase"),
			Reason:                         plString(pl, "reason"),
			Error:                          plString(pl, "error"),
			Fingerprint:                    plString(pl, "fingerprint"),
			SelfRepairAttempt:              plInt(pl, "self_repair_attempt"),
			SelfRepairMaxAttempts:          plInt(pl, "self_repair_max_attempts"),
			TargetAgent:                    plString(pl, "target_agent"),
			TargetCorr:                     plString(pl, "target_correlation"),
			Resolution:                     plString(pl, "resolution"),
			ResolutionSummary:              plString(pl, "resolution_summary"),
			DelegateTo:                     plString(pl, "delegate_to"),
			DelegatedBy:                    plString(pl, "delegated_by"),
			RootAgent:                      plString(pl, "root_agent"),
			ChainDepth:                     intNumber(pl["chain_depth"]),
			IncidentID:                     plString(pl, "incident_id"),
			RootIncidentID:                 plString(pl, "root_incident_id"),
			ParentIncidentID:               plString(pl, "parent_incident_id"),
			RoutingForceGeneration:         intNumber(pl["routing_force_generation"]),
			PreviousRoutingForceGeneration: intNumber(pl["previous_routing_force_generation"]),
		}
		if cur, ok := metaByMessage[msgID]; !ok || row.Seq > cur.Seq {
			metaByMessage[msgID] = row
		}
		return nil
	})
	out := make([]agentEscalationRow, 0, len(msgs))
	for _, msg := range msgs {
		if !msg.Help {
			continue
		}
		if msg.To != slug && msg.To != board.Everyone {
			continue
		}
		replies := st.Replies(msg.ID, boardReadMaxLimit)
		acked := boardMessageAckedBy(msg, slug)
		status := "open"
		if len(replies) > 0 {
			status = "answered"
		} else if acked {
			status = "acked"
		}
		row := agentEscalationRow{
			MessageID:  msg.ID,
			From:       msg.From,
			To:         msg.To,
			Text:       msg.Text,
			TSUnixMS:   msg.TSMS,
			Status:     status,
			ReplyCount: len(replies),
			Acked:      acked,
		}
		if meta, ok := metaByMessage[msg.ID]; ok {
			row.SourceAgent = meta.Agent
			row.Mode = meta.Mode
			row.WakePhase = meta.Phase
			row.WakeReason = meta.Reason
			row.WakeError = meta.Error
			row.WakeCorrelationID = meta.TargetCorr
			row.Fingerprint = meta.Fingerprint
			row.Resolution = meta.Resolution
			row.ResolutionSummary = meta.ResolutionSummary
			row.DelegateTo = meta.DelegateTo
			row.RootAgent = meta.RootAgent
			row.ChainDepth = meta.ChainDepth
			row.IncidentID = meta.IncidentID
			row.RootIncidentID = meta.RootIncidentID
			row.ParentIncidentID = meta.ParentIncidentID
			if strings.HasPrefix(meta.Phase, "delegation_") {
				row.OriginKind = "delegated"
				row.OriginAgent = firstNonEmpty(meta.DelegatedBy, msg.From)
			} else {
				row.OriginKind = "doctor"
				row.OriginAgent = firstNonEmpty(msg.From, meta.DelegatedBy)
			}
		}
		// Prefer parsing the broken/source agent out of the message text only when
		// the event envelope didn't carry one (older history).
		if row.SourceAgent == "" {
			row.SourceAgent = escalationSourceFromText(msg.Text)
		}
		if row.RootAgent == "" {
			row.RootAgent = row.SourceAgent
		}
		if row.OriginKind == "" {
			row.OriginKind = "doctor"
			row.OriginAgent = msg.From
		}
		out = append(out, row)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].TSUnixMS > out[j].TSUnixMS })
	// Cursor pagination: cursor encodes (TSUnixMS, MessageID) of the LAST
	// entry on the previous page; server skips entries strictly newer-or-equal.
	if cursorTS > 0 || cursorID != "" {
		filtered := out[:0]
		for _, r := range out {
			if r.TSUnixMS > cursorTS {
				continue
			}
			if r.TSUnixMS == cursorTS && r.MessageID >= cursorID {
				continue
			}
			filtered = append(filtered, r)
		}
		out = filtered
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
		last := out[limit-1]
		return out, strconv.FormatInt(last.TSUnixMS, 10) + ":" + last.MessageID
	}
	return out, ""
}

func boardMessageAckedBy(m board.Message, slug string) bool {
	slug = strings.ToLower(strings.TrimSpace(slug))
	for _, by := range m.AckedBy {
		if strings.ToLower(strings.TrimSpace(by)) == slug {
			return true
		}
	}
	return false
}

func escalationSourceFromText(text string) string {
	text = strings.TrimSpace(text)
	const prefix = "Doctor "
	if !strings.HasPrefix(text, prefix) {
		return ""
	}
	if i := strings.Index(text, " for agent "); i > 0 {
		// The agent named after "for agent" is the broken agent, not the owner.
		start := i + len(" for agent ")
		if end := strings.Index(text[start:], "."); end > 0 {
			return strings.TrimSpace(text[start : start+end])
		}
	}
	return ""
}

func joinActivityParts(parts ...string) string {
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return strings.Join(out, " · ")
}

func wakeRunbookActivitySuffix(pl map[string]any) string {
	raw, _ := pl["autonomy_runbook"].(map[string]any)
	if len(raw) == 0 {
		return ""
	}
	parts := []string{
		plString(raw, "trigger_contract"),
		plString(raw, "route_contract"),
		plString(raw, "recovery_contract"),
		plString(raw, "sleep_contract"),
	}
	clean := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			clean = append(clean, part)
		}
	}
	if len(clean) == 0 {
		return ""
	}
	return "contract " + strings.Join(clean, "/")
}

func plString(pl map[string]any, key string) string {
	s, _ := pl[key].(string)
	return s
}

func plInt(pl map[string]any, key string) int {
	switch n := pl[key].(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return 0
	}
}

func plStrings(pl map[string]any, key string) []string {
	raw, ok := pl[key].([]any)
	if !ok || len(raw) == 0 {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func truncate(s string, n int) string {
	return strutil.Ellipsis(strings.TrimSpace(s), n, "…")
}

func firstNonEmpty(items ...string) string { return strutil.FirstNonEmpty(items...) }

func firstNonEmptyStrings(primary, fallback []string) []string {
	return strutil.FirstNonEmptySlice(primary, fallback)
}

func intNumber(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return 0
	}
}

func (s *Server) handleAgentRetire(conn net.Conn, req Request) {
	s.handleAgentSetRetired(conn, req, true)
}

func (s *Server) handleAgentRevive(conn net.Conn, req Request) {
	s.handleAgentSetRetired(conn, req, false)
}

func (s *Server) handleAgentSetRetired(conn net.Conn, req Request, retired bool) {
	ref, err := requiredArgString(req.Args, "ref")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	reason := stringArg(req.Args, "reason")
	// Compute impact BEFORE the state change so a retire reports what it affected.
	var impact []string
	var impactSummary map[string]any
	if retired {
		if p, ok := s.k.Roster().Get(ref); ok {
			impact = s.k.AgentImpact(p.Slug)
			impactSummary = s.agentImpactResult(p)
		}
	} else if p, ok := s.k.Roster().Get(ref); ok {
		if err := s.validateAgentHierarchyRefs(p); err != nil {
			s.fail(conn, req, err)
			return
		}
	}
	p, err := s.k.SetProfileRetired(ref, retired, reason)
	if err != nil {
		if errors.Is(err, roster.ErrNotFound) {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown agent: " + ref})
			return
		}
		s.fail(conn, req, err)
		return
	}
	res := map[string]any{"profile": profileView(p)}
	if retired {
		pausedStanding, err := s.pauseAgentStanding(p.Slug)
		if err != nil {
			s.fail(conn, req, err)
			return
		}
		pausedSchedules, err := s.pauseAgentSchedules(p.Slug)
		if err != nil {
			s.fail(conn, req, err)
			return
		}
		res["impact"] = impact
		res["impact_summary"] = impactSummary
		res["standing_paused"] = pausedStanding
		res["schedules_paused"] = pausedSchedules
		if impactSummary != nil {
			impactSummary["standing_paused"] = pausedStanding
			impactSummary["schedules_paused"] = pausedSchedules
		}
		publishOperatorAction(s.k, "agent.retire", s.k.NewCorrelation(), map[string]any{
			"agent":            p.Slug,
			"reason":           p.RetiredReason,
			"retired_ms":       p.RetiredMS,
			"standing_paused":  pausedStanding,
			"schedules_paused": pausedSchedules,
			"impact_summary":   impactSummary,
		})
	} else {
		pausedStanding := s.countAgentPausedStanding(p.Slug)
		pausedSchedules := s.countAgentPausedSchedules(p.Slug)
		res["standing_paused"] = pausedStanding
		res["schedules_paused"] = pausedSchedules
		publishOperatorAction(s.k, "agent.revive", s.k.NewCorrelation(), map[string]any{
			"agent":            p.Slug,
			"standing_paused":  pausedStanding,
			"schedules_paused": pausedSchedules,
		})
	}
	s.invalidateAgentListCache()
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: res})
}

func (s *Server) handleAgentRemove(conn net.Conn, req Request) {
	ref, err := requiredArgString(req.Args, "ref")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	p, found := s.k.Roster().Get(ref)
	if !found {
		s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"removed": false}})
		return
	}
	if p.System {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "system agent " + p.Slug + " cannot be removed; retire or pause it instead"})
		return
	}
	cascade := parseAgentRemoveCascade(req.Args["cascade"])
	subagents := s.agentSubagents(p.Slug)
	if len(subagents) > 0 && !cascade.Subagents {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: fmt.Sprintf("agent %s has %d dependent sub-agent(s); set cascade.subagents=true to retire them before removal", p.Slug, len(subagents))})
		return
	}
	retainedMailboxMessageLabels := s.agentRemovalMailboxImpact(p.Slug, subagents, cascade.Subagents)
	retainedWorkflowRefLabels := s.agentWorkflowImpact(p)
	retainedSubagentWorkflowRefLabels := []string(nil)
	if cascade.Subagents {
		retainedSubagentWorkflowRefLabels = s.subagentImpact(subagents, (*Server).agentWorkflowImpact)
	}
	retainedMailboxMessages := len(retainedMailboxMessageLabels)
	retainedWorkflowRefs := len(retainedWorkflowRefLabels)
	retainedSubagentWorkflowRefs := len(retainedSubagentWorkflowRefLabels)
	retiredSubagents, retiredSubagentSlugs, err := s.retireAgentSubagents(p.Slug, subagents, cascade.Subagents)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	removedStanding, err := s.removeAgentStanding(p.Slug, cascade.Standing)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	removedSchedules, err := s.removeAgentSchedules(p.Slug, cascade.Schedules)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	if cascade.Subagents {
		for _, child := range subagents {
			n, err := s.removeAgentStanding(child.Slug, cascade.Standing)
			if err != nil {
				s.fail(conn, req, err)
				return
			}
			removedStanding += n
			n, err = s.removeAgentSchedules(child.Slug, cascade.Schedules)
			if err != nil {
				s.fail(conn, req, err)
				return
			}
			removedSchedules += n
		}
	}
	forgotMemory, err := s.forgetAgentMemory(p, cascade.Memory)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	forgotAuthoredMemory, err := s.forgetAgentAuthoredSharedMemory(p.Slug, cascade.AuthoredMemory)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	archivedSkills, err := s.archiveAgentSkills(p.Slug, cascade.Skills)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	deletedConfig, prunedConfigAccess, err := s.deleteAgentConfigEntries(p.Slug, cascade.Config)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	deletedWorkspaces, err := s.deleteAgentWorkspace(p, cascade.Workspace)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	if cascade.Subagents {
		for _, child := range subagents {
			n, err := s.forgetAgentMemory(child, cascade.Memory)
			if err != nil {
				s.fail(conn, req, err)
				return
			}
			forgotMemory += n
			n, err = s.forgetAgentAuthoredSharedMemory(child.Slug, cascade.AuthoredMemory)
			if err != nil {
				s.fail(conn, req, err)
				return
			}
			forgotAuthoredMemory += n
			n, err = s.archiveAgentSkills(child.Slug, cascade.Skills)
			if err != nil {
				s.fail(conn, req, err)
				return
			}
			archivedSkills += n
			var pruned int
			n, pruned, err = s.deleteAgentConfigEntries(child.Slug, cascade.Config)
			if err != nil {
				s.fail(conn, req, err)
				return
			}
			deletedConfig += n
			prunedConfigAccess += pruned
			n, err = s.deleteAgentWorkspace(child, cascade.Workspace)
			if err != nil {
				s.fail(conn, req, err)
				return
			}
			deletedWorkspaces += n
		}
	}
	ok, err := s.k.RemoveProfile(ref)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	if ok {
		publishOperatorAction(s.k, "agent.remove", s.k.NewCorrelation(), map[string]any{
			"agent":                                  p.Slug,
			"removed":                                true,
			"cascade":                                agentRemoveCascadeView(cascade),
			"standing_removed":                       removedStanding,
			"schedules_removed":                      removedSchedules,
			"memories_forgotten":                     forgotMemory,
			"authored_memories_forgotten":            forgotAuthoredMemory,
			"skills_archived":                        archivedSkills,
			"configs_deleted":                        deletedConfig,
			"configs_access_pruned":                  prunedConfigAccess,
			"workspaces_deleted":                     deletedWorkspaces,
			"subagents_retired":                      retiredSubagents,
			"subagents_retired_slugs":                retiredSubagentSlugs,
			"mailbox_messages_retained":              retainedMailboxMessages,
			"mailbox_messages_retained_refs":         retainedMailboxMessageLabels,
			"workflow_refs_retained":                 retainedWorkflowRefs,
			"workflow_refs_retained_labels":          retainedWorkflowRefLabels,
			"subagent_workflow_refs_retained":        retainedSubagentWorkflowRefs,
			"subagent_workflow_refs_retained_labels": retainedSubagentWorkflowRefLabels,
		})
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"removed":                                ok,
		"standing_removed":                       removedStanding,
		"schedules_removed":                      removedSchedules,
		"memories_forgotten":                     forgotMemory,
		"authored_memories_forgotten":            forgotAuthoredMemory,
		"skills_archived":                        archivedSkills,
		"configs_deleted":                        deletedConfig,
		"configs_access_pruned":                  prunedConfigAccess,
		"workspaces_deleted":                     deletedWorkspaces,
		"subagents_retired":                      retiredSubagents,
		"subagents_retired_slugs":                retiredSubagentSlugs,
		"mailbox_messages_retained":              retainedMailboxMessages,
		"mailbox_messages_retained_refs":         retainedMailboxMessageLabels,
		"workflow_refs_retained":                 retainedWorkflowRefs,
		"workflow_refs_retained_labels":          retainedWorkflowRefLabels,
		"subagent_workflow_refs_retained":        retainedSubagentWorkflowRefs,
		"subagent_workflow_refs_retained_labels": retainedSubagentWorkflowRefLabels,
	}})
	s.invalidateAgentListCache()
}

type agentRemoveCascade struct {
	Standing       bool
	Schedules      bool
	Memory         bool
	AuthoredMemory bool
	Skills         bool
	Config         bool
	Workspace      bool
	Subagents      bool
}

func agentRemoveCascadeView(c agentRemoveCascade) map[string]any {
	return map[string]any{
		"standing":        c.Standing,
		"schedules":       c.Schedules,
		"memory":          c.Memory,
		"authored_memory": c.AuthoredMemory,
		"skills":          c.Skills,
		"config":          c.Config,
		"workspace":       c.Workspace,
		"subagents":       c.Subagents,
	}
}

func parseAgentRemoveCascade(raw any) agentRemoveCascade {
	var c agentRemoveCascade
	m, ok := raw.(map[string]any)
	if !ok {
		return c
	}
	c.Standing = boolish(m["standing"])
	c.Schedules = boolish(m["schedules"])
	c.Memory = boolish(m["memory"])
	c.AuthoredMemory = boolish(m["authored_memory"]) || boolish(m["authored_shared_memory"])
	c.Skills = boolish(m["skills"])
	c.Config = boolish(m["config"])
	c.Workspace = boolish(m["workspace"]) || boolish(m["workdir"])
	c.Subagents = boolish(m["subagents"])
	return c
}

func boolish(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		x = strings.TrimSpace(strings.ToLower(x))
		return x == "1" || x == "true" || x == "yes" || x == "on"
	default:
		return false
	}
}

func agentMailboxImpactLabel(msg board.Message, slug string) (string, bool) {
	from := strings.ToLower(strings.TrimSpace(msg.From))
	to := strings.ToLower(strings.TrimSpace(msg.To))
	acked := boardMessageAckedBy(msg, slug)
	var direction string
	switch {
	case from == slug:
		direction = "sent"
	case to == slug:
		direction = "received"
	case msg.To == board.Everyone && from != slug:
		direction = "broadcast"
	case acked:
		direction = "acked"
	default:
		return "", false
	}
	topic := strings.TrimSpace(msg.Topic)
	if topic == "" {
		topic = "board"
	}
	id := strings.TrimSpace(msg.ID)
	if id == "" {
		id = strconv.FormatInt(msg.TSMS, 10)
	}
	return topic + " " + direction + " (" + id + ")", true
}

func agentSubagentImpact(slug string, children []roster.Profile) []string {
	out := make([]string, 0, len(children))
	for _, child := range children {
		roles := make([]string, 0, 2)
		if strings.EqualFold(strings.TrimSpace(child.OwnerAgent), slug) {
			roles = append(roles, "owner")
		}
		if strings.EqualFold(strings.TrimSpace(child.ParentAgent), slug) {
			roles = append(roles, "parent")
		}
		if len(roles) == 0 {
			roles = append(roles, "descendant")
		}
		label := child.Slug
		if strings.TrimSpace(child.Name) != "" && strings.TrimSpace(child.Name) != child.Slug {
			label = strings.TrimSpace(child.Name) + " (" + child.Slug + ")"
		}
		if len(roles) > 0 {
			label += " [" + strings.Join(roles, ", ") + "]"
		}
		if child.Retired {
			label += " [retired]"
		}
		out = append(out, label)
	}
	sort.Strings(out)
	return out
}

func workflowNodeConfigReferencesAgent(raw json.RawMessage, slug string) bool {
	if len(raw) == 0 {
		return false
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return false
	}
	return jsonValueReferencesAgent(v, strings.ToLower(strings.TrimSpace(slug)), "")
}

func jsonValueReferencesAgent(v any, slug, key string) bool {
	switch x := v.(type) {
	case map[string]any:
		for k, value := range x {
			if jsonValueReferencesAgent(value, slug, strings.ToLower(strings.TrimSpace(k))) {
				return true
			}
		}
	case []any:
		for _, value := range x {
			if jsonValueReferencesAgent(value, slug, key) {
				return true
			}
		}
	case string:
		if !agentReferenceConfigKey(key) {
			return false
		}
		return strings.EqualFold(strings.TrimSpace(x), slug)
	}
	return false
}

func agentReferenceConfigKey(key string) bool {
	switch key {
	case "agent", "agent_slug", "target_agent", "owner_agent", "parent_agent", "delegate_to", "source_agent", "root_agent":
		return true
	default:
		return false
	}
}

func (s *Server) agentSubagents(slug string) []roster.Profile {
	root := strings.TrimSpace(slug)
	if root == "" {
		return nil
	}
	byManager := map[string][]roster.Profile{}
	for _, p := range s.k.Roster().List() {
		childSlug := strings.TrimSpace(p.Slug)
		if childSlug == "" || strings.EqualFold(childSlug, root) {
			continue
		}
		seenManager := map[string]bool{}
		for _, manager := range []string{strings.TrimSpace(p.OwnerAgent), strings.TrimSpace(p.ParentAgent)} {
			if manager == "" || strings.EqualFold(manager, childSlug) || seenManager[strings.ToLower(manager)] {
				continue
			}
			seenManager[strings.ToLower(manager)] = true
			byManager[strings.ToLower(manager)] = append(byManager[strings.ToLower(manager)], p)
		}
	}
	var out []roster.Profile
	seen := map[string]bool{strings.ToLower(root): true}
	var walk func(string)
	walk = func(parent string) {
		for _, child := range byManager[strings.ToLower(strings.TrimSpace(parent))] {
			key := strings.ToLower(strings.TrimSpace(child.Slug))
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, child)
			walk(child.Slug)
		}
	}
	walk(root)
	sort.Slice(out, func(i, j int) bool {
		return strings.Compare(out[i].Slug, out[j].Slug) < 0
	})
	return out
}

func (s *Server) agentWorkspaceInfo(p roster.Profile) (string, bool) {
	workdir := strings.TrimSpace(p.Workdir)
	if workdir == "" {
		return "", false
	}
	root := s.agentWorkspaceRoot()
	dir, ok := confineUnder(root, workdir)
	if !ok || filepath.Clean(dir) == filepath.Clean(root) {
		return "", false
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return "", false
	}
	files, bytes := countTreeFiles(dir)
	return filepath.ToSlash(workdir) + fmt.Sprintf(" (%d file(s), %d bytes)", files, bytes), true
}

func (s *Server) agentWorkspaceRoot() string {
	if ws := os.Getenv(brand.EnvPrefix + "WORKSPACE"); strings.TrimSpace(ws) != "" {
		return ws
	}
	return filepath.Join(s.k.BaseDir(), "workspace")
}

func countTreeFiles(root string) (int, int64) {
	var files int
	var bytes int64
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		info, ierr := d.Info()
		if ierr != nil {
			return nil
		}
		files++
		bytes += info.Size()
		return nil
	})
	return files, bytes
}

func registerRosterCommands() {
	register(
		commandSpec{Cmd: CmdAgentList, Handler: func(dc *DispatchCtx) { dc.S.handleAgentList(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdAgentAdd, Handler: func(dc *DispatchCtx) { dc.S.handleAgentAdd(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdAgentEdit, Handler: func(dc *DispatchCtx) { dc.S.handleAgentEdit(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdAgentSetEnabled, Handler: func(dc *DispatchCtx) { dc.S.handleAgentSetEnabled(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdAgentRemove, Handler: func(dc *DispatchCtx) { dc.S.handleAgentRemove(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdAgentTaskUpdate, Handler: func(dc *DispatchCtx) { dc.S.handleAgentTaskUpdate(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdAgentImpact, Handler: func(dc *DispatchCtx) { dc.S.handleAgentImpact(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdAgentTombstone, Handler: func(dc *DispatchCtx) { dc.S.handleAgentTombstone(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdAgentGraveyard, Handler: func(dc *DispatchCtx) { dc.S.handleAgentGraveyard(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdAgentActivity, Handler: func(dc *DispatchCtx) { dc.S.handleAgentActivity(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdAgentRepairStatus, Handler: func(dc *DispatchCtx) { dc.S.handleAgentRepairStatus(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdAgentRepair, Handler: func(dc *DispatchCtx) { dc.S.handleAgentRepair(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdAgentEscalations, Handler: func(dc *DispatchCtx) { dc.S.handleAgentEscalations(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdAgentWake, Handler: func(dc *DispatchCtx) { dc.S.handleAgentWake(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdAgentResolve, Handler: func(dc *DispatchCtx) { dc.S.handleAgentResolve(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdAgentRetire, Handler: func(dc *DispatchCtx) { dc.S.handleAgentRetire(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdAgentRevive, Handler: func(dc *DispatchCtx) { dc.S.handleAgentRevive(dc.Conn, dc.Req) }},
	)
}

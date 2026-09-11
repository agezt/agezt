// SPDX-License-Identifier: MIT

package governor

// Governor usage-tracking + snapshot helpers: recordUsage +
// indexUsageTokens + UsageFor + admitRate + SpentByTaskMicrocents +
// SpentByAgentMicrocents + Snapshot + rolloverIfNeededLocked + publish +
// shouldFallback + providerNames. Carved out of governor.go during the
// Day 29 god file split #1.

import (
	"context"
	"errors"
	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/event"
)

func (g *Governor) recordUsage(p *ProviderInfo, req agent.CompletionRequest, resp *agent.CompletionResponse) {
	model := resp.Usage.Model
	if model == "" {
		model = req.Model
	}
	// Sanitize provider-reported token counts (M191): clamp negatives to
	// 0 so a buggy/hostile usage response can't charge a NEGATIVE cost
	// (which would credit the ledger and eventually disable the daily
	// ceiling). Using the clamped values for BOTH the cost and the audit
	// event keeps the journal honest too. The overflow case is handled by
	// costMicrocents' saturating math.
	inTok := resp.Usage.InputTokens
	if inTok < 0 {
		inTok = 0
	}
	outTok := resp.Usage.OutputTokens
	if outTok < 0 {
		outTok = 0
	}
	// Prompt-cache tokens bill at the cache-read (M289) / cache-write (M291)
	// rates. Clamp to [0, inTok]; cached+write are subsets of the prompt, so a
	// buggy endpoint claiming more must not credit the ledger (costMicrocentsCached
	// re-clamps defensively too).
	cachedTok := resp.Usage.CachedInputTokens
	if cachedTok < 0 {
		cachedTok = 0
	}
	if cachedTok > inTok {
		cachedTok = inTok
	}
	writeTok := resp.Usage.CacheWriteInputTokens
	if writeTok < 0 {
		writeTok = 0
	}
	if cachedTok+writeTok > inTok {
		writeTok = inTok - cachedTok
	}
	cost := costMicrocentsCached(model, inTok, cachedTok, writeTok, outTok)

	g.mu.Lock()
	g.rolloverIfNeededLocked()
	g.spentToday.Add(cost)
	if req.TaskType != "" {
		g.spentByTaskToday[req.TaskType] += cost
	}
	if req.Agent != "" {
		g.spentByAgentToday[req.Agent] += cost // per-identity ledger (M793)
	}
	spent := g.spentToday.Load()
	ceiling := g.effectiveCeilingLocked()
	g.mu.Unlock()

	// Record into the best-effort usage index (reporting fast path) with the SAME
	// token counts that go into budget.consumed below, so a fast-path hit equals
	// the journal sum exactly.
	g.indexUsageTokens(req.CorrelationID, inTok, outTok)

	// An unpriced model was just billed at the fallback rate (BIZ-001). Journal
	// it on EVERY such call, not only under strict pricing: the operator's
	// ledger is now moving on an estimate rather than a real price, and the only
	// way to see that — and to fix it with `agt catalog sync` or a table entry —
	// is for the daemon to say so each time. Emitted before budget.consumed so a
	// journal fold reads the explanation ahead of the charge.
	if model != "" && !modelIsPriced(model) {
		g.publish(event.Spec{
			Subject:       "governor.budget",
			Kind:          event.KindBudgetUnpriced,
			Actor:         "governor",
			CorrelationID: req.CorrelationID,
			Payload: map[string]any{
				"model":                   model,
				"charged_microcents":      cost,
				"fallback_input_mc_mtok":  unpricedFallbackPrice.InputMicrocentsPerMTok,
				"fallback_output_mc_mtok": unpricedFallbackPrice.OutputMicrocentsPerMTok,
				"reason":                  "no catalog or fallback-table price; charged at the conservative fallback rate so the model still consumes budget",
			},
		})
	}

	g.publish(event.Spec{
		Subject: "governor.budget",
		Kind:    event.KindBudgetConsumed,
		Actor:   "governor",
		// Stamp the spending run's correlation (M47) so spend can be
		// attributed per run / per delegation by a journal fold — the same
		// way every other event ties to its run. Empty when the caller set
		// no CorrelationID (e.g. an out-of-run governor call).
		CorrelationID: req.CorrelationID,
		Payload: map[string]any{
			"provider":                 p.Name,
			"model":                    model,
			"input_tokens":             inTok,
			"cached_input_tokens":      cachedTok,
			"cache_write_input_tokens": writeTok,
			"output_tokens":            outTok,
			"cost_microcents":          cost,
			"spent_today_mc":           spent,
			"ceiling_mc":               ceiling,
			"correlation_id":           req.CorrelationID,
		},
	})
}

// indexUsageTokens adds one call's token usage to the bounded best-effort
// per-correlation index that backs UsageFor. Summed across a run's calls, exactly
// as the journal fold does. Guarded by its own lock (never the spend hot path).
// Empty correlations are ignored.
//
// Memory is bounded by a two-generation rotation: writes land in the live map;
// when it fills (usageIndexCap), it becomes the previous generation and a fresh
// live map starts (total ≤ 2×cap). The critical property is that a still-running
// correlation's partial sum is NEVER served as authoritative: a write for a corr
// already in the previous generation MIGRATES that accumulated entry into the live
// map before adding, so a hit always reflects the COMPLETE running sum. A corr is
// dropped only when it ages out of BOTH generations untouched — then UsageFor
// cleanly misses and the caller falls back to the authoritative journal scan.
// (The earlier wholesale-drop could leave an in-flight run with a fresh zero entry
// and then serve that PARTIAL sum with ok=true — a silent under-count on the API
// usage field; this rotation removes that hazard.)
func (g *Governor) indexUsageTokens(corr string, in, out int) {
	if corr == "" {
		return
	}
	g.usageMu.Lock()
	defer g.usageMu.Unlock()
	if g.usage == nil {
		g.usage = make(map[string]usageTokens, 64)
	}
	e, live := g.usage[corr]
	if !live {
		// Consolidate any prior-generation accumulation so the live entry holds the
		// complete running sum, never a partial.
		if prev, hadPrev := g.usagePrev[corr]; hadPrev {
			e = prev
			delete(g.usagePrev, corr)
		}
	}
	e.in += in
	e.out += out
	g.usage[corr] = e
	if len(g.usage) >= usageIndexCap {
		g.usagePrev = g.usage
		g.usage = make(map[string]usageTokens, 64)
	}
}

// UsageFor returns the summed input/output tokens recorded for corr from the
// bounded in-memory index, or ok=false when the correlation isn't present (the
// caller then falls back to the authoritative journal scan). Best-effort fast
// path for the API `usage` reporting field; never used for billing or ceilings.
func (g *Governor) UsageFor(corr string) (in, out int, ok bool) {
	g.usageMu.Lock()
	defer g.usageMu.Unlock()
	// Live generation first, then the previous one. Migrate-on-write guarantees a
	// correlation is never split across both, so the first hit is the complete sum.
	if e, hit := g.usage[corr]; hit {
		return e.in, e.out, true
	}
	if e, hit := g.usagePrev[corr]; hit {
		return e.in, e.out, true
	}
	return 0, 0, false
}

// admitRate applies the per-minute fixed-window rate limit. It returns
// (admitted, callsUsedThisWindow, limit). When unlimited (RateLimitPerMin <= 0)
// it always admits without counting. On admission it increments the window
// counter; the window resets when the UTC clock-minute changes.
func (g *Governor) admitRate() (bool, int, int) {
	if g.cfg.RateLimitPerMin <= 0 {
		return true, 0, 0
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	win := g.cfg.Now().UTC().Format("2006-01-02T15:04")
	if win != g.rateWindow {
		g.rateWindow = win
		g.callsThisWindow = 0
	}
	if g.callsThisWindow >= g.cfg.RateLimitPerMin {
		return false, g.callsThisWindow, g.cfg.RateLimitPerMin
	}
	g.callsThisWindow++
	return true, g.callsThisWindow, g.cfg.RateLimitPerMin
}

// SpentByTaskMicrocents returns the current-day spend for taskType
// in microcents. Useful for the operator-facing `agt budget` view
// and for tests. Returns 0 for unknown / unspent task types.
func (g *Governor) SpentByTaskMicrocents(taskType string) int64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.rolloverIfNeededLocked()
	return g.spentByTaskToday[taskType]
}

// SpentByAgentMicrocents returns the current-day spend attributed to a named
// agent (M793) in microcents. 0 for unknown / unspent agents.
func (g *Governor) SpentByAgentMicrocents(slug string) int64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.rolloverIfNeededLocked()
	return g.spentByAgentToday[slug]
}

// TaskBudgetSnapshot is one row of the per-task budget view
// returned by Snapshot. SpentMicrocents is the current spend;
// CeilingMicrocents is the configured cap (always > 0 since
// zero-cap entries are filtered at parse time).
type TaskBudgetSnapshot struct {
	TaskType        string
	SpentMicrocents int64
	CapMicrocents   int64
}

// BudgetSnapshot is the read-only view powering `agt budget`
// (and any future operator-facing budget UI). All counters are
// for the current UTC day; UTCDate names that day so callers
// can render "as of 2026-05-29" without a separate field.
type BudgetSnapshot struct {
	UTCDate           string
	SpentMicrocents   int64
	CeilingMicrocents int64
	PerTask           []TaskBudgetSnapshot
	// StrictPricing reflects whether unpriced models are refused (M193/M194)
	// rather than silently charged $0 — part of the operator's spend-protection
	// posture surfaced by `agt budget`.
	StrictPricing bool
}

// Snapshot returns a point-in-time copy of the governor's budget
// state. Holds the mutex for the duration; callers should treat
// the returned struct as immutable. Per-task entries are returned
// for every type with a configured cap (NOT only ones with spend
// > 0) so the operator sees "I configured a cap but nothing has
// hit it" as a separate state from "no cap configured."
func (g *Governor) Snapshot() BudgetSnapshot {
	g.mu.Lock()
	g.rolloverIfNeededLocked()
	snap := BudgetSnapshot{
		UTCDate:           g.today,
		SpentMicrocents:   g.spentToday.Load(),
		CeilingMicrocents: g.effectiveCeilingLocked(),
		StrictPricing:     g.cfg.StrictPricing,
	}
	g.mu.Unlock()
	if len(g.cfg.TaskBudgets) > 0 {
		snap.PerTask = make([]TaskBudgetSnapshot, 0, len(g.cfg.TaskBudgets))
		for taskType, cap := range g.cfg.TaskBudgets {
			snap.PerTask = append(snap.PerTask, TaskBudgetSnapshot{
				TaskType:        taskType,
				SpentMicrocents: g.spentByTaskToday[taskType],
				CapMicrocents:   cap,
			})
		}
	}
	return snap
}

// rolloverIfNeededLocked resets the daily counter at UTC midnight.
// Caller holds g.mu.
func (g *Governor) rolloverIfNeededLocked() {
	today := g.cfg.Now().UTC().Format("2006-01-02")
	if today != g.today {
		g.today = today
		g.spentToday.Store(0)
		// Per-task counters also roll over (M1.zz). Clear the map
		// rather than allocating a fresh one to keep the same
		// underlying memory hot.
		for k := range g.spentByTaskToday {
			delete(g.spentByTaskToday, k)
		}
		// Per-agent counters too (M793).
		for k := range g.spentByAgentToday {
			delete(g.spentByAgentToday, k)
		}
	}
}

func (g *Governor) publish(spec event.Spec) {
	b := g.bus.Load()
	if b == nil {
		return
	}
	_, _ = b.Publish(spec)
}

// shouldFallback decides whether to walk further down the provider chain.
// Cancellation and the budget error are terminal; everything else (rate
// limits, transient HTTP errors, parse errors) is fall-back-able in M1.b.
// A richer classification (DECISIONS C3 borderline-escalation, transient
// vs. terminal) lands with the catalog sync.
func shouldFallback(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	if errors.Is(err, ErrBudgetExceeded) {
		return false
	}
	if errors.Is(err, ErrStreamInterrupted) {
		// Output already reached the consumer (M882): a retry/fallback would
		// duplicate the stream, so the failure is terminal.
		return false
	}
	return true
}

func providerNames(ps []*ProviderInfo) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.Name
	}
	return out
}


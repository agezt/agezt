// SPDX-License-Identifier: MIT

package selfrepair

// Self-repair coordinator claim logic (M846): the reaper-driven claim
// pipeline that converts per-agent reaper reports into autoRepairCandidate
// entries (claim + claimOne). Carved out of selfrepair.go during the
// Day 25 god file split #7 so the main file can focus on wire-up +
// handleTick + dispatch.

import (
	"fmt"
	"sort"
	"time"
	"github.com/agezt/agezt/kernel/roster"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
)

func (c *autoRepairCoordinator) claim(k *kernelruntime.Kernel, rep kernelruntime.ReaperReport, profiles []roster.Profile) []autoRepairCandidate {
	bySlug := make(map[string]roster.Profile, len(profiles))
	for _, p := range profiles {
		bySlug[p.Slug] = p
	}
	now := c.now()
	c.mu.Lock()
	defer c.mu.Unlock()
	var out []autoRepairCandidate
	claimed := map[string]bool{}
	var claimOK bool
	for _, row := range rep.MisconfiguredAgents {
		p, ok := bySlug[row.Slug]
		if !ok || !autoRepairEligible(p) {
			continue
		}
		fp := autoRepairFingerprint(append([]string{"misconfigured"}, row.Issues...)...)
		incidentID := autoRepairIncidentID(p.Slug, fp, now)
		cand := autoRepairCandidate{
			Slug:         p.Slug,
			Mode:         "misconfigured",
			Issues:       append([]string(nil), row.Issues...),
			Fingerprint:  fp,
			Reason:       autoRepairReason(row.Issues),
			EscalateTo:   autoRepairEscalationTarget(p),
			EscalateFrom: autoRepairEscalationFrom(p),
			RootAgent:    p.Slug,
			ChainDepth:   0,
			IncidentID:   incidentID,
			RootChainID:  incidentID,
		}
		if cand, claimOK = c.claimOne(k, now, cand, p); !claimOK {
			continue
		}
		claimed[p.Slug] = true
		out = append(out, cand)
	}
	for _, row := range rep.RoutingUnstable {
		if claimed[row.Slug] {
			continue
		}
		p, ok := bySlug[row.Slug]
		if !ok || !autoRepairEligible(p) {
			continue
		}
		fp := autoRepairRoutingUnstableFingerprint(row)
		incidentID := autoRepairIncidentID(p.Slug, fp, now)
		cand := autoRepairCandidate{
			Slug:         p.Slug,
			Mode:         "routing_unstable",
			Fingerprint:  fp,
			Reason:       autoRepairRoutingUnstableReason(row),
			EscalateTo:   autoRepairEscalationTarget(p),
			EscalateFrom: autoRepairEscalationFrom(p),
			RootAgent:    p.Slug,
			ChainDepth:   0,
			IncidentID:   incidentID,
			RootChainID:  incidentID,
		}
		if cand, claimOK = c.claimOne(k, now, cand, p); !claimOK {
			continue
		}
		claimed[p.Slug] = true
		out = append(out, cand)
	}
	for _, row := range rep.RoutingForcedExhausted {
		if claimed[row.Slug] {
			continue
		}
		p, ok := bySlug[row.Slug]
		if !ok || !autoRepairEligible(p) {
			continue
		}
		fp := autoRepairRoutingForcedExhaustedFingerprint(row)
		incidentID := autoRepairIncidentID(p.Slug, fp, now)
		cand := autoRepairCandidate{
			Slug:                    p.Slug,
			Mode:                    "routing_forced_exhausted",
			Fingerprint:             fp,
			Reason:                  autoRepairRoutingForcedExhaustedReason(row),
			EscalateTo:              autoRepairEscalationTarget(p),
			EscalateFrom:            autoRepairEscalationFrom(p),
			RootAgent:               p.Slug,
			ChainDepth:              0,
			IncidentID:              incidentID,
			RootChainID:             incidentID,
			RoutingRollbackTaskType: row.TaskType,
			RoutingRollbackToChain:  append([]string(nil), row.ForcedChain...),
		}
		if cand, claimOK = c.claimOne(k, now, cand, p); !claimOK {
			continue
		}
		claimed[p.Slug] = true
		out = append(out, cand)
	}
	for _, row := range rep.RoutingForcedFailed {
		if claimed[row.Slug] {
			continue
		}
		p, ok := bySlug[row.Slug]
		if !ok || !autoRepairEligible(p) {
			continue
		}
		fp := autoRepairRoutingForcedFailedFingerprint(row)
		incidentID := autoRepairIncidentID(p.Slug, fp, now)
		cand := autoRepairCandidate{
			Slug:                    p.Slug,
			Mode:                    "routing_forced_failed",
			Fingerprint:             fp,
			Reason:                  autoRepairRoutingForcedFailedReason(row),
			EscalateTo:              autoRepairEscalationTarget(p),
			EscalateFrom:            autoRepairEscalationFrom(p),
			RootAgent:               p.Slug,
			ChainDepth:              0,
			IncidentID:              incidentID,
			RootChainID:             incidentID,
			RoutingRollbackTaskType: row.TaskType,
			RoutingRollbackToChain:  append([]string(nil), row.ForcedChain...),
		}
		if cand, claimOK = c.claimOne(k, now, cand, p); !claimOK {
			continue
		}
		claimed[p.Slug] = true
		out = append(out, cand)
	}
	for _, row := range rep.RoutingPressure {
		if claimed[row.Slug] {
			continue
		}
		p, ok := bySlug[row.Slug]
		if !ok || !autoRepairEligible(p) {
			continue
		}
		fp := autoRepairRoutingFingerprint(row)
		incidentID := autoRepairIncidentID(p.Slug, fp, now)
		cand := autoRepairCandidate{
			Slug:         p.Slug,
			Mode:         "routing",
			Fingerprint:  fp,
			Reason:       autoRepairRoutingReason(row),
			EscalateTo:   autoRepairEscalationTarget(p),
			EscalateFrom: autoRepairEscalationFrom(p),
			RootAgent:    p.Slug,
			ChainDepth:   0,
			IncidentID:   incidentID,
			RootChainID:  incidentID,
		}
		if plan := autoRepairRoutingRollbackPlan(k, p, row, now); plan != nil {
			cand.RoutingRollbackTaskType = plan.TaskType
			cand.RoutingRollbackFromChain = append([]string(nil), plan.FromChain...)
			cand.RoutingRollbackToChain = append([]string(nil), plan.ToChain...)
			cand.Reason = plan.Reason
		}
		if cand, claimOK = c.claimOne(k, now, cand, p); !claimOK {
			continue
		}
		claimed[p.Slug] = true
		out = append(out, cand)
	}
	for _, row := range rep.RetryPressure {
		if claimed[row.Slug] {
			continue // config/routing signals take priority when both exist
		}
		p, ok := bySlug[row.Slug]
		if !ok || !autoRepairEligible(p) {
			continue
		}
		fp := autoRepairRetryFingerprint(row)
		incidentID := autoRepairIncidentID(p.Slug, fp, now)
		cand := autoRepairCandidate{
			Slug:         p.Slug,
			Mode:         "retry_pressure",
			Fingerprint:  fp,
			Reason:       autoRepairRetryReason(row),
			EscalateTo:   autoRepairEscalationTarget(p),
			EscalateFrom: autoRepairEscalationFrom(p),
			RootAgent:    p.Slug,
			ChainDepth:   0,
			IncidentID:   incidentID,
			RootChainID:  incidentID,
		}
		if cand, claimOK = c.claimOne(k, now, cand, p); !claimOK {
			continue
		}
		claimed[p.Slug] = true
		out = append(out, cand)
	}
	for _, row := range rep.DegradedAgents {
		if claimed[row.Slug] {
			continue // config/routing/retry signals take priority when both exist
		}
		p, ok := bySlug[row.Slug]
		if !ok || !autoRepairEligible(p) {
			continue
		}
		fp := autoRepairDegradedFingerprint(row)
		incidentID := autoRepairIncidentID(p.Slug, fp, now)
		cand := autoRepairCandidate{
			Slug:         p.Slug,
			Mode:         "degraded",
			Fingerprint:  fp,
			Reason:       autoRepairDegradedReason(row),
			EscalateTo:   autoRepairEscalationTarget(p),
			EscalateFrom: autoRepairEscalationFrom(p),
			RootAgent:    p.Slug,
			ChainDepth:   0,
			IncidentID:   incidentID,
			RootChainID:  incidentID,
		}
		if cand, claimOK = c.claimOne(k, now, cand, p); !claimOK {
			continue
		}
		claimed[p.Slug] = true
		out = append(out, cand)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Slug < out[j].Slug })
	return out
}

func (c *autoRepairCoordinator) claimOne(k *kernelruntime.Kernel, now time.Time, cand autoRepairCandidate, p roster.Profile) (autoRepairCandidate, bool) {
	if _, busy := c.inflight[cand.Slug]; busy {
		return cand, false
	}
	if prev, ok := c.last[cand.Slug]; ok && prev.fingerprint == cand.Fingerprint && now.Sub(prev.at) < c.cooldown {
		return cand, false
	}
	max := autoRepairMaxAttempts(p)
	attempts := previousAutoRepairAttempts(k, cand.Slug, cand.Fingerprint)
	cand.SelfRepairMaxAttempts = max
	cand.SelfRepairAttempt = attempts + 1
	if max > 0 && attempts >= max {
		cand.SelfRepairAttempt = attempts
		cand.SelfRepairExhausted = true
		if cand.Reason != "" {
			cand.Reason += "; "
		}
		cand.Reason += fmt.Sprintf("self-repair attempts exhausted (%d/%d)", attempts, max)
	}
	c.inflight[cand.Slug] = struct{}{}
	c.last[cand.Slug] = autoRepairStamp{fingerprint: cand.Fingerprint, at: now}
	return cand, true
}


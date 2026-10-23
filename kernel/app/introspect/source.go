// SPDX-License-Identifier: MIT

package introspect

import (
	"encoding/json"
	"sort"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/internal/strutil"
	"github.com/agezt/agezt/kernel/contract/cadenceapi"
	"github.com/agezt/agezt/kernel/contract/introspectapi"
	"github.com/agezt/agezt/kernel/contract/standingapi"
	"github.com/agezt/agezt/kernel/event"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
)

// fallbackWindow bounds the journal scan for provider.fallback events — recent
// history is what a health check cares about, and it keeps the cost flat on a
// long-lived daemon (mirrors runstool's window).
const fallbackWindow = 5000
const reaperOverviewWindow = 30 * 24 * time.Hour

// kernelSource adapts the live *runtime.Kernel to the tool's narrow Source. The
// kernel owns every slice the overview reports, so this is a thin gather — the
// same data `agt status` (handleStatus) assembles, minus the server-level
// HTTP/channel extras the kernel doesn't own.
type kernelSource struct {
	k *kernelruntime.Kernel
}

// NewKernelSource binds the introspect tool to the live kernel. Wired in
// cmd/agezt once the kernel opens, exactly like the other self-knowledge tools.
func NewKernelSource(k *kernelruntime.Kernel) introspectapi.Source { return &kernelSource{k: k} }

func (s *kernelSource) Overview() introspectapi.Overview {
	k := s.k

	headSeq, _ := k.Journal().Head() // (seq, hash); hash unused here
	if headSeq < 0 {
		headSeq = 0 // empty journal returns -1; render 0 for a fresh install
	}

	// Floor uptime to whole seconds — sub-second precision is noise.
	uptimeSecs := int64(time.Since(k.StartTime()) / time.Second)

	schedTotal, schedEnabled := 0, 0
	if sched := k.Schedules(); sched != nil {
		for _, e := range sched.List() {
			schedTotal++
			if e.Enabled {
				schedEnabled++
			}
		}
	}

	pendingApprovals := 0
	if ap := k.Approvals(); ap != nil {
		pendingApprovals = ap.PendingCount()
	}

	fbCount, fbReason := s.providerFallbacks()
	reaper := s.reaperOverview()

	dl := k.SubAgentLimits()

	// Registered tool names, sorted, so the agent sees its own capability surface
	// (and the list is stable across calls).
	names := make([]string, 0, len(k.Tools()))
	for name := range k.Tools() {
		names = append(names, name)
	}
	sort.Strings(names)

	return introspectapi.Overview{
		Daemon:                 brand.Version,
		Protocol:               brand.ProtocolVersion,
		Model:                  k.Model(),
		UptimeSeconds:          uptimeSecs,
		Halted:                 k.IsHalted(),
		ActiveRuns:             k.ActiveRuns(),
		Tools:                  names,
		MemoryRecords:          k.Memory().Count(),
		WorldEntities:          k.World().Count(),
		ActiveSkills:           k.Forge().Count(),
		JournalHead:            headSeq,
		SchedulesTotal:         schedTotal,
		SchedulesEnabled:       schedEnabled,
		PendingApprovals:       pendingApprovals,
		ProviderFallbacks:      fbCount,
		ProviderFallbackReason: fbReason,
		Delegation:             dl,
		Reaper:                 reaper,
	}
}

func (s *kernelSource) Schedules() []cadenceapi.Entry {
	if sched := s.k.Schedules(); sched != nil {
		return sched.List()
	}
	return nil
}

func (s *kernelSource) Reaper() introspectapi.ReaperReport {
	return reaperReport(s.scan())
}

// scan runs the reaper over the window the health report cares about.
func (s *kernelSource) scan() kernelruntime.ReaperReport {
	cutoff := time.Now().Add(-reaperOverviewWindow).UnixMilli()
	return s.k.ReaperScan(cutoff, cutoff)
}

// reaperReport projects the reaper's own scan row onto the contract's row.
// The projection is a hand-written field-for-field copy, so it is pinned by a
// test: a field dropped here would be silent — the agent would read a fleet
// scan that is missing a bucket without ever being told.
func reaperReport(rep kernelruntime.ReaperReport) introspectapi.ReaperReport {
	out := introspectapi.ReaperReport{StaleArtifacts: rep.StaleArtifacts, StaleBytes: rep.StaleBytes}
	for _, a := range rep.DeadAgents {
		out.DeadAgents = append(out.DeadAgents, introspectapi.ReaperAgent{Slug: a.Slug, Name: a.Name, LastActiveMS: a.LastActiveMS})
	}
	for _, a := range rep.DegradedAgents {
		out.DegradedAgents = append(out.DegradedAgents, introspectapi.ReaperDegradedAgent{
			Slug: a.Slug, Name: a.Name, Failures: a.Failures, Window: a.Window, Threshold: a.Threshold,
			DoctorAgent: a.DoctorAgent, SelfRepairEnabled: a.SelfRepairEnabled, EscalateTo: a.EscalateTo,
			LastFailureMS: a.LastFailureMS, LastReason: a.LastReason,
		})
	}
	for _, a := range rep.MisconfiguredAgents {
		out.MisconfiguredAgents = append(out.MisconfiguredAgents, introspectapi.ReaperMisconfiguredAgent{
			Slug: a.Slug, Name: a.Name, Issues: append([]string(nil), a.Issues...),
			DoctorAgent: a.DoctorAgent, SelfRepairEnabled: a.SelfRepairEnabled, EscalateTo: a.EscalateTo,
		})
	}
	return out
}

// reaperOverview projects the same scan onto the overview's fleet snapshot:
// the counts and the slugs, without the rows themselves.
func reaperOverview(rep kernelruntime.ReaperReport) introspectapi.ReaperOverview {
	out := introspectapi.ReaperOverview{
		DeadAgents:          len(rep.DeadAgents),
		DegradedAgents:      len(rep.DegradedAgents),
		MisconfiguredAgents: len(rep.MisconfiguredAgents),
	}
	for _, a := range rep.DeadAgents {
		out.DeadSlugs = append(out.DeadSlugs, a.Slug)
	}
	for _, a := range rep.DegradedAgents {
		out.DegradedSlugs = append(out.DegradedSlugs, a.Slug)
	}
	for _, a := range rep.MisconfiguredAgents {
		out.MisconfiguredSlugs = append(out.MisconfiguredSlugs, a.Slug)
	}
	return out
}

func (s *kernelSource) Standing() []standingapi.Order {
	if st := s.k.Standing(); st != nil {
		return st.List()
	}
	return nil
}

// providerFallbacks folds recent journal events for provider.fallback (M280):
// how many times the governor fell back from a primary provider to a backup,
// plus the most recent reason. A silent primary that errors every request — and
// gets masked by the always-on mock fallback — is otherwise invisible in a
// health report.
func (s *kernelSource) providerFallbacks() (int, string) {
	j := s.k.Journal()
	if j == nil {
		return 0, ""
	}
	evs, err := j.Tail(fallbackWindow)
	if err != nil {
		return 0, ""
	}
	count := 0
	last := ""
	for _, e := range evs {
		if e.Kind != event.KindProviderFallback {
			continue
		}
		count++
		var p struct {
			Reason string `json:"reason"`
		}
		if e.Payload != nil {
			_ = json.Unmarshal(e.Payload, &p)
			if p.Reason != "" {
				last = strutil.Ellipsis(p.Reason, 160, "…")
			}
		}
	}
	return count, last
}

func (s *kernelSource) reaperOverview() introspectapi.ReaperOverview {
	return reaperOverview(s.scan())
}

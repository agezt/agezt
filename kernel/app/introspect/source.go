// SPDX-License-Identifier: MIT

package introspect

import (
	"encoding/json"
	"sort"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/internal/strutil"
	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/contract/cadenceapi"
	"github.com/agezt/agezt/kernel/contract/introspectapi"
	"github.com/agezt/agezt/kernel/contract/standingapi"
	"github.com/agezt/agezt/kernel/event"
)

// fallbackWindow bounds the journal scan for provider.fallback events — recent
// history is what a health check cares about, and it keeps the cost flat on a
// long-lived daemon (mirrors runstool's window).
const fallbackWindow = 5000
const reaperOverviewWindow = 30 * 24 * time.Hour

// kernelSource adapts the introspector's ports to the tool's narrow Source. The
// daemon owns every slice the overview reports, so this is a thin gather — the
// same data `agt status` (handleStatus) assembles, minus the server-level
// HTTP/channel extras the kernel doesn't own.
//
// The source holds the PORTS and not the handle they were filled from: a file
// that names the runtime is the port's business, and the adapter that answers
// the contract is not.
type kernelSource struct {
	ports Ports
}

// NewKernelSource binds the introspect tool to the ports. Wired in cmd/agezt
// once the kernel opens, exactly like the other self-knowledge tools.
//
// The source stays bound even when the ports hold no daemon: the tool keeps
// its shape, and the GATHER answers with nothing rather than panicking.
func NewKernelSource(ports Ports) introspectapi.Source { return &kernelSource{ports: ports} }

func (s *kernelSource) Overview() introspectapi.Overview {
	headSeq, _ := s.ports.JournalHead() // (seq, hash); hash unused here
	if headSeq < 0 {
		headSeq = 0 // empty journal returns -1; render 0 for a fresh install
	}

	// Floor uptime to whole seconds — sub-second precision is noise.
	//
	// A host with NO daemon has no uptime: measuring from the zero time would
	// claim a daemon has been running since the year one.
	var uptimeSecs int64
	if s.ports.Ready() {
		uptimeSecs = int64(time.Since(s.ports.Started()) / time.Second)
	}

	schedTotal, schedEnabled := 0, 0
	for _, e := range s.ports.ScheduleEntries() {
		schedTotal++
		if e.Enabled {
			schedEnabled++
		}
	}

	fbCount, fbReason := s.providerFallbacks()
	reaper := s.reaperOverview()

	// Registered tool names, sorted, so the agent sees its own capability surface
	// (and the list is stable across calls).
	names := s.ports.ToolNames()
	sort.Strings(names)

	return introspectapi.Overview{
		Daemon:                 brand.Version,
		Protocol:               brand.ProtocolVersion,
		Model:                  s.ports.DefaultModel(),
		UptimeSeconds:          uptimeSecs,
		Halted:                 s.ports.Stopped(),
		ActiveRuns:             s.ports.ActiveRunCount(),
		Tools:                  names,
		MemoryRecords:          s.ports.MemoryRecordCount(),
		WorldEntities:          s.ports.WorldEntities(),
		ActiveSkills:           s.ports.ActiveSkills(),
		JournalHead:            headSeq,
		SchedulesTotal:         schedTotal,
		SchedulesEnabled:       schedEnabled,
		PendingApprovals:       s.ports.PendingApprovals(),
		ProviderFallbacks:      fbCount,
		ProviderFallbackReason: fbReason,
		Delegation:             s.ports.Delegation(),
		Reaper:                 reaper,
	}
}

func (s *kernelSource) Schedules() []cadenceapi.Entry {
	return s.ports.ScheduleEntries()
}

func (s *kernelSource) Reaper() introspectapi.ReaperReport {
	return reaperReport(s.scan())
}

// scan runs the reaper over the window the health report cares about.
func (s *kernelSource) scan() agentapi.ReaperReport {
	cutoff := time.Now().Add(-reaperOverviewWindow).UnixMilli()
	return s.ports.FleetScan(cutoff)
}

// reaperReport projects the reaper's own scan row onto the contract's row.
// The projection is a hand-written field-for-field copy, so it is pinned by a
// test: a field dropped here would be silent — the agent would read a fleet
// scan that is missing a bucket without ever being told.
func reaperReport(rep agentapi.ReaperReport) introspectapi.ReaperReport {
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
func reaperOverview(rep agentapi.ReaperReport) introspectapi.ReaperOverview {
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
	return s.ports.StandingOrders()
}

// providerFallbacks folds recent journal events for provider.fallback (M280):
// how many times the governor fell back from a primary provider to a backup,
// plus the most recent reason. A silent primary that errors every request — and
// gets masked by the always-on mock fallback — is otherwise invisible in a
// health report.
func (s *kernelSource) providerFallbacks() (int, string) {
	evs, err := s.ports.JournalTail(fallbackWindow)
	if err != nil {
		return 0, ""
	}
	count := 0
	last := ""
	for _, e := range evs {
		if e == nil || e.Kind != event.KindProviderFallback {
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

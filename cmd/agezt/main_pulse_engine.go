// SPDX-License-Identifier: MIT

// Pulse engine construction + observer admin + health stat (bootStep +
// pulseObserverAdmin + AddDiskObserver + AddProbeObserver + buildPulse +
// healthStatFromJournal). Extracted from main_pulse.go during Day 211
// god-file refactor (#57). Public API unchanged.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/pulse"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/state"
	"github.com/agezt/agezt/kernel/warden"
)

type bootStep struct {
	name  string
	run   func() (desc string, err error)
	fatal bool
}
type pulseObserverAdmin struct {
	eng  *pulse.Engine
	ward warden.Engine
	st   *state.FileStore
}
func (a pulseObserverAdmin) AddDiskObserver(path string, minPct float64) (string, bool) {
	return a.eng.AddObserver(pulse.NewDiskObserver(path, minPct, pulse.DiskUsage)), true
}
func (a pulseObserverAdmin) AddProbeObserver(name string, argv []string) (string, bool) {
	return a.eng.AddObserver(pulse.NewProbeObserver(name, argv, a.ward, a.st)), true
}
func buildPulse(k *kernelruntime.Kernel, ward warden.Engine, model string, stdout io.Writer, extraSink pulse.BriefSink) (*pulse.Engine, string) {
	if strings.EqualFold(os.Getenv(brand.EnvPrefix+"PULSE"), "off") {
		return nil, ""
	}
	cadence := 60 * time.Second
	if v := os.Getenv(brand.EnvPrefix + "PULSE_CADENCE"); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			cadence = d
		}
	}
	dial := pulse.ParseDial(os.Getenv(brand.EnvPrefix + "PULSE_DIAL"))
	qh := pulse.ParseQuietHours(os.Getenv(brand.EnvPrefix + "PULSE_QUIET_HOURS"))

	var obs []pulse.Observer
	var parts []string
	if spec := os.Getenv(brand.EnvPrefix + "PULSE_PROBE"); spec != "" {
		if name, argv, ok := pulse.ParseProbeSpec(spec); ok {
			obs = append(obs, pulse.NewProbeObserver(name, argv, ward, k.State()))
			parts = append(parts, "probe:"+name)
		}
	}
	if spec := os.Getenv(brand.EnvPrefix + "PULSE_DISK"); spec != "" {
		if path, pctStr, ok := strings.Cut(spec, ":"); ok {
			if pct, err := strconv.ParseFloat(pctStr, 64); err == nil && pct > 0 {
				obs = append(obs, pulse.NewDiskObserver(path, pct, pulse.DiskUsage))
				parts = append(parts, "disk:"+path)
			}
		}
	}
	// Self-health observer (M628): the daemon watches its OWN run/tool
	// reliability and briefs the operator when its health transitions
	// (healthy↔degraded↔critical) — proactive self-monitoring, not just the
	// reactive Analyst. On by default (the whole point is to watch unprompted);
	// AGEZT_PULSE_HEALTH=off disables it, =<float> overrides the tool-error-rate
	// degrade threshold (default 0.30).
	if hv := os.Getenv(brand.EnvPrefix + "PULSE_HEALTH"); !strings.EqualFold(hv, "off") {
		degradeAt := 0.0 // observer falls back to its default
		if f, err := strconv.ParseFloat(hv, 64); err == nil && f > 0 {
			degradeAt = f
		}
		obs = append(obs, pulse.NewHealthObserver(healthStatFromJournal(k), degradeAt, 0))
		parts = append(parts, "self:health")
	}
	useLLM := strings.EqualFold(os.Getenv(brand.EnvPrefix+"PULSE_LLM"), "on")
	// Autonomy level (M999): off|ask|act, default act. `act` makes Pulse EMIT a
	// pulse.initiative.act event on actionable observations; an autonomous run still
	// requires an ENABLED standing order bound to it (the seeded responder ships
	// disabled), so a fresh install is bold-by-default yet dormant until opt-in.
	initiative := pulse.ParseInitiative(os.Getenv(brand.EnvPrefix + "PULSE_INITIATIVE"))

	eng := pulse.New(pulse.Config{
		Bus:        k.Bus(),
		State:      k.State(),
		Warden:     ward,
		Provider:   k.Provider(),
		Model:      model,
		Relevance:  k.World(), // world-model relevance signal (SPEC-05 §3.4)
		Observers:  obs,
		Dial:       dial,
		Initiative: initiative,
		Cadence:    cadence,
		QuietHours: qh,
		UseLLM:     useLLM,
		Sink:       briefSink(stdout, extraSink),
	})
	observers := "no observers configured"
	if len(parts) > 0 {
		observers = strings.Join(parts, ",")
	}
	return eng, fmt.Sprintf("dial=%s initiative=%s cadence=%s observers=[%s]", dial, initiative, cadence, observers)
}
func healthStatFromJournal(k *kernelruntime.Kernel) pulse.HealthStatFunc {
	const healthWindow = 2000
	return func(context.Context) (pulse.HealthStat, error) {
		j := k.Journal()
		if j == nil {
			return pulse.HealthStat{}, nil
		}
		evs, err := j.Tail(healthWindow)
		if err != nil {
			return pulse.HealthStat{}, err
		}
		var st pulse.HealthStat
		for _, e := range evs {
			switch e.Kind {
			case event.KindToolInvoked:
				st.ToolCalls++
			case event.KindToolResult:
				var p struct {
					Error bool `json:"error"`
				}
				_ = json.Unmarshal(e.Payload, &p)
				if p.Error {
					st.ToolErrors++
				}
			case event.KindTaskCompleted:
				st.Runs++
			case event.KindTaskFailed:
				st.Runs++
				st.FailedRuns++
			}
		}
		return st, nil
	}
}

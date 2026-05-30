// SPDX-License-Identifier: MIT

package pulse

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/agezt/agezt/kernel/state"
	"github.com/agezt/agezt/kernel/warden"
)

// probeNS is the state namespace where the probe observer remembers each
// command's last exit code, so it can detect green↔red transitions across
// beats (and across daemon restarts).
const probeNS = "pulse_probe"

// ProbeObserver runs an operator-configured command on each beat and emits a
// delta only on a green↔red transition — it reports "CI went red", never the
// full output every beat (SPEC-03 §3.1). Dependency-free: it covers the
// "broken CI" demo by running e.g. `make test` or `gh run list` through the
// same Warden every tool uses, with no GitHub/network coupling in Pulse.
type ProbeObserver struct {
	name   string
	argv   []string
	warden warden.Engine
	state  *state.FileStore
}

// NewProbeObserver constructs a probe. name labels the source ("probe:<name>");
// argv is the command (argv[0] is the binary — no shell unless you pass one).
func NewProbeObserver(name string, argv []string, w warden.Engine, st *state.FileStore) *ProbeObserver {
	return &ProbeObserver{name: name, argv: argv, warden: w, state: st}
}

// Name implements Observer.
func (p *ProbeObserver) Name() string { return "probe:" + p.name }

type probeState struct {
	LastExit int  `json:"last_exit"`
	Known    bool `json:"known"`
}

// Poll implements Observer: run the command, compare its exit code to the last
// known one, and emit a delta on transition.
func (p *ProbeObserver) Poll(ctx context.Context) ([]Delta, error) {
	if len(p.argv) == 0 || p.warden == nil {
		return nil, nil
	}
	res, err := p.warden.Run(ctx, warden.Spec{
		Profile: warden.ProfileNone,
		Argv:    p.argv,
		Env:     nil,
		Actor:   "pulse",
	})
	if err != nil {
		return nil, fmt.Errorf("probe %s: %w", p.name, err)
	}
	exit := res.ExitCode

	prev := p.load()
	p.save(probeState{LastExit: exit, Known: true})

	// First observation establishes a baseline without alerting (avoids a
	// spurious "failed" the moment Pulse starts on an already-red probe —
	// that's noise, not a change).
	if !prev.Known {
		return nil, nil
	}
	prevFailed := prev.LastExit != 0
	nowFailed := exit != 0
	if prevFailed == nowFailed {
		return nil, nil // no transition
	}

	src := "probe:" + p.name
	if nowFailed {
		return []Delta{{
			Source:  src,
			Kind:    "probe_failed",
			Summary: fmt.Sprintf("%s probe failed (exit %d)", p.name, exit),
			Before:  "ok",
			After:   fmt.Sprintf("exit %d", exit),
			Hints:   map[string]string{"severity": string(SevHigh)},
		}}, nil
	}
	return []Delta{{
		Source:  src,
		Kind:    "probe_recovered",
		Summary: fmt.Sprintf("%s probe recovered (exit 0)", p.name),
		Before:  fmt.Sprintf("exit %d", prev.LastExit),
		After:   "ok",
		Hints:   map[string]string{"severity": string(SevMedium)},
	}}, nil
}

func (p *ProbeObserver) load() probeState {
	if p.state == nil {
		return probeState{}
	}
	raw, ok, err := p.state.Get(probeNS, p.name)
	if err != nil || !ok {
		return probeState{}
	}
	var s probeState
	if json.Unmarshal(raw, &s) != nil {
		return probeState{}
	}
	return s
}

func (p *ProbeObserver) save(s probeState) {
	if p.state != nil {
		_ = p.state.Set(probeNS, p.name, s)
	}
}

// DiskFreeFunc returns the free and total bytes for a path. Injectable so the
// observer is testable without touching the real filesystem; the daemon wires
// the platform implementation (diskUsage).
type DiskFreeFunc func(path string) (free, total uint64, err error)

// DiskObserver watches free space on a path and emits a delta when usage
// crosses the configured minimum-free-percent threshold (SPEC-03 §3.2 system
// health). Transition-based like the probe: it fires on crossing, not every
// beat while low.
type DiskObserver struct {
	path     string
	minPct   float64 // alert when free% drops below this
	usage    DiskFreeFunc
	wasLow   bool
	hasState bool
}

// NewDiskObserver constructs a disk observer. minPct is the free-percent floor
// (e.g. 10 → alert when less than 10% free).
func NewDiskObserver(path string, minPct float64, usage DiskFreeFunc) *DiskObserver {
	return &DiskObserver{path: path, minPct: minPct, usage: usage}
}

// Name implements Observer.
func (o *DiskObserver) Name() string { return "system:disk" }

// Poll implements Observer.
func (o *DiskObserver) Poll(_ context.Context) ([]Delta, error) {
	if o.usage == nil {
		return nil, nil
	}
	free, total, err := o.usage(o.path)
	if err != nil {
		return nil, fmt.Errorf("disk %s: %w", o.path, err)
	}
	if total == 0 {
		return nil, nil
	}
	freePct := 100 * float64(free) / float64(total)
	low := freePct < o.minPct

	prevLow := o.wasLow
	prevKnown := o.hasState
	o.wasLow = low
	o.hasState = true

	if !prevKnown || prevLow == low {
		return nil, nil // baseline or no transition
	}

	if low {
		sev := SevHigh
		if freePct < o.minPct/2 {
			sev = SevCritical
		}
		return []Delta{{
			Source:  "system:disk",
			Kind:    "disk_low",
			Summary: fmt.Sprintf("disk %s low: %.1f%% free (< %.0f%%)", o.path, freePct, o.minPct),
			Before:  "ok",
			After:   fmt.Sprintf("%.1f%% free", freePct),
			Hints:   map[string]string{"severity": string(sev)},
		}}, nil
	}
	return []Delta{{
		Source:  "system:disk",
		Kind:    "disk_recovered",
		Summary: fmt.Sprintf("disk %s recovered: %.1f%% free", o.path, freePct),
		Before:  "low",
		After:   fmt.Sprintf("%.1f%% free", freePct),
		Hints:   map[string]string{"severity": string(SevLow)},
	}}, nil
}

// ParseProbeSpec parses an `AGEZT_PULSE_PROBE` entry of the form
// "name=<label>;argv=<command>". The command is split on spaces (simple; for
// quoting, pass `sh -c "..."`). Returns ok=false on a malformed spec.
func ParseProbeSpec(s string) (name string, argv []string, ok bool) {
	for part := range strings.SplitSeq(s, ";") {
		k, v, found := strings.Cut(strings.TrimSpace(part), "=")
		if !found {
			continue
		}
		switch strings.TrimSpace(k) {
		case "name":
			name = strings.TrimSpace(v)
		case "argv", "cmd":
			argv = strings.Fields(v)
		}
	}
	if name == "" || len(argv) == 0 {
		return "", nil, false
	}
	return name, argv, true
}

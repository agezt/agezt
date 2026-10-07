// SPDX-License-Identifier: MIT
package pulse

import (
	"context"
	"errors"
	"strings"
	"time"
)

type Controller interface {
	StatusMap() map[string]any
	Pause()
	Resume()
	Beat()
	SetCadence(time.Duration) time.Duration
	SetDial(string) string
	SetQuietHours(string) string
	FlushDigest() int
	RemoveObserver(string) int
	PendingAsks() []map[string]any
	ResolveAsk(string, bool) (bool, bool)
}
type Observers interface {
	AddDiskObserver(string, float64) (string, bool)
	AddProbeObserver(string, []string) (string, bool)
}

var ErrDisabled = errors.New("pulse is disabled (AGEZT_PULSE=off)")
var ErrWatchesUnavailable = errors.New("watches are unavailable (pulse is disabled)")
var ErrPendingAskMissing = errors.New("no pending ask with that issue_key (already resolved?)")

type Controls struct {
	controller Controller
	observers  Observers
	persist    func(string, string)
}

func NewControls(controller Controller, observers Observers, persist func(string, string)) *Controls {
	return &Controls{controller: controller, observers: observers, persist: persist}
}
func (s *Controls) Available() bool        { return s.controller != nil }
func (s *Controls) WatchesAvailable() bool { return s.observers != nil }
func (s *Controls) setting(name, value string) {
	if s.persist != nil {
		s.persist(name, value)
	}
}

type StatusInput struct{}
type StatusOutput map[string]any

func (s *Controls) Status(_ context.Context, _ StatusInput) (StatusOutput, error) {
	if !s.Available() {
		return StatusOutput{"enabled": false}, nil
	}
	out := s.controller.StatusMap()
	out["enabled"] = true
	return StatusOutput(out), nil
}

type AsksInput struct{}
type AsksOutput struct {
	Asks []map[string]any `json:"asks"`
}

func (s *Controls) Asks(_ context.Context, _ AsksInput) (AsksOutput, error) {
	if !s.Available() {
		return AsksOutput{Asks: []map[string]any{}}, nil
	}
	return AsksOutput{Asks: s.controller.PendingAsks()}, nil
}

type PauseInput struct{}
type PauseOutput struct {
	Paused bool `json:"paused"`
}

func (s *Controls) Pause(_ context.Context, _ PauseInput) (PauseOutput, error) {
	if !s.Available() {
		return PauseOutput{}, ErrDisabled
	}
	s.controller.Pause()
	return PauseOutput{Paused: true}, nil
}
func (s *Controls) Resume(_ context.Context, _ PauseInput) (PauseOutput, error) {
	if !s.Available() {
		return PauseOutput{}, ErrDisabled
	}
	s.controller.Resume()
	return PauseOutput{Paused: false}, nil
}

type BeatInput struct{}
type BeatOutput struct {
	Triggered bool `json:"triggered"`
}

func (s *Controls) Beat(_ context.Context, _ BeatInput) (BeatOutput, error) {
	if !s.Available() {
		return BeatOutput{}, ErrDisabled
	}
	s.controller.Beat()
	return BeatOutput{Triggered: true}, nil
}

type ResolveInput struct {
	IssueKey string
	Approve  bool
}
type ResolveOutput struct {
	Resolved bool `json:"resolved"`
	Approved bool `json:"approved"`
	Acted    bool `json:"acted"`
}

func (s *Controls) Resolve(_ context.Context, in ResolveInput) (ResolveOutput, error) {
	if !s.Available() {
		return ResolveOutput{}, ErrDisabled
	}
	found, acted := s.controller.ResolveAsk(in.IssueKey, in.Approve)
	if !found {
		return ResolveOutput{}, ErrPendingAskMissing
	}
	return ResolveOutput{Resolved: true, Approved: in.Approve, Acted: acted}, nil
}

type CadenceInput struct{ Seconds float64 }
type CadenceOutput struct {
	CadenceMS int64 `json:"cadence_ms"`
}

func (s *Controls) Cadence(_ context.Context, in CadenceInput) (CadenceOutput, error) {
	if !s.Available() {
		return CadenceOutput{}, ErrDisabled
	}
	if in.Seconds <= 0 {
		return CadenceOutput{}, errors.New("args.seconds must be > 0")
	}
	applied := s.controller.SetCadence(time.Duration(in.Seconds * float64(time.Second)))
	s.setting("AGEZT_PULSE_CADENCE", applied.String())
	return CadenceOutput{CadenceMS: applied.Milliseconds()}, nil
}

type DialInput struct{ Dial string }
type DialOutput struct {
	Dial string `json:"dial"`
}

func (s *Controls) Dial(_ context.Context, in DialInput) (DialOutput, error) {
	if !s.Available() {
		return DialOutput{}, ErrDisabled
	}
	applied := s.controller.SetDial(in.Dial)
	s.setting("AGEZT_PULSE_DIAL", applied)
	return DialOutput{Dial: applied}, nil
}

type QuietInput struct{ Hours string }
type QuietOutput struct {
	Quiet string `json:"quiet"`
}

func (s *Controls) Quiet(_ context.Context, in QuietInput) (QuietOutput, error) {
	if !s.Available() {
		return QuietOutput{}, ErrDisabled
	}
	applied := s.controller.SetQuietHours(in.Hours)
	s.setting("AGEZT_PULSE_QUIET_HOURS", applied)
	return QuietOutput{Quiet: applied}, nil
}

type FlushInput struct{}
type FlushOutput struct {
	Flushed int `json:"flushed"`
}

func (s *Controls) Flush(_ context.Context, _ FlushInput) (FlushOutput, error) {
	if !s.Available() {
		return FlushOutput{}, ErrDisabled
	}
	return FlushOutput{Flushed: s.controller.FlushDigest()}, nil
}

type UnwatchInput struct{ Name string }
type UnwatchOutput struct {
	Removed int `json:"removed"`
}

func (s *Controls) Unwatch(_ context.Context, in UnwatchInput) (UnwatchOutput, error) {
	if !s.Available() {
		return UnwatchOutput{}, ErrDisabled
	}
	return UnwatchOutput{Removed: s.controller.RemoveObserver(in.Name)}, nil
}

type WatchInput struct {
	Path   string
	MinPct float64
}
type ObserverOutput struct {
	Added    bool   `json:"added"`
	Observer string `json:"observer"`
}

func (s *Controls) Watch(_ context.Context, in WatchInput) (ObserverOutput, error) {
	if !s.WatchesAvailable() {
		return ObserverOutput{}, ErrWatchesUnavailable
	}
	if in.MinPct <= 0 || in.MinPct >= 100 {
		return ObserverOutput{}, errors.New("args.min_pct must be between 0 and 100")
	}
	name, ok := s.observers.AddDiskObserver(in.Path, in.MinPct)
	if !ok {
		return ObserverOutput{}, errors.New("could not add the watch")
	}
	return ObserverOutput{Added: true, Observer: name}, nil
}

type ProbeInput struct{ Name, Command string }

func (s *Controls) Probe(_ context.Context, in ProbeInput) (ObserverOutput, error) {
	if !s.WatchesAvailable() {
		return ObserverOutput{}, ErrWatchesUnavailable
	}
	argv := strings.Fields(in.Command)
	if in.Name == "" || len(argv) == 0 {
		return ObserverOutput{}, errors.New("args.name and args.command required")
	}
	name, ok := s.observers.AddProbeObserver(in.Name, argv)
	if !ok {
		return ObserverOutput{}, errors.New("could not add the probe")
	}
	return ObserverOutput{Added: true, Observer: name}, nil
}

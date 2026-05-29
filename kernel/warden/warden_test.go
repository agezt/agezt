// SPDX-License-Identifier: MIT

package warden_test

import (
	"context"
	"encoding/json"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/journal"
	"github.com/agezt/agezt/kernel/warden"
)

// ---- helpers ----

func newBus(t *testing.T) (*bus.Bus, *journal.Journal) {
	t.Helper()
	j, err := journal.Open(t.TempDir(), journal.Options{})
	if err != nil {
		t.Fatalf("journal.Open: %v", err)
	}
	t.Cleanup(func() { j.Close() })
	b := bus.New(j)
	t.Cleanup(b.Close)
	return b, j
}

// echoArgv returns an argv that prints msg to stdout and exits 0,
// portable across Linux/macOS/Windows.
func echoArgv(msg string) []string {
	if runtime.GOOS == "windows" {
		return []string{"cmd", "/C", "echo " + msg}
	}
	return []string{"sh", "-c", "echo " + msg}
}

// failArgv returns an argv that exits with code 7.
func failArgv() []string {
	if runtime.GOOS == "windows" {
		return []string{"cmd", "/C", "exit 7"}
	}
	return []string{"sh", "-c", "exit 7"}
}

// busyArgv returns an argv that loops in-process so a kill from
// Warden's timeout actually has to reap it. Avoids ping/sleep which
// behave inconsistently across Windows shells.
func busyArgv() []string {
	if runtime.GOOS == "windows" {
		return []string{"cmd", "/C", "for /L %i in (1,0,2) do @ver >NUL"}
	}
	return []string{"sh", "-c", "while :; do :; done"}
}

func countEvents(t *testing.T, j *journal.Journal, kind event.Kind) int {
	t.Helper()
	var n int
	_ = j.Range(func(e *event.Event) error {
		if e.Kind == kind {
			n++
		}
		return nil
	})
	return n
}

// ---- basic exec ----

func TestRun_ExecutesAndCapturesStdout(t *testing.T) {
	b, j := newBus(t)
	e := warden.New(b)
	res, err := e.Run(context.Background(), warden.Spec{
		Profile: warden.ProfileNone,
		Argv:    echoArgv("hello-warden"),
		Actor:   "test",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.ExitCode != 0 {
		t.Errorf("exit=%d want 0; stderr=%q", res.ExitCode, string(res.Stderr))
	}
	if !strings.Contains(string(res.Stdout), "hello-warden") {
		t.Errorf("stdout=%q missing greeting", string(res.Stdout))
	}
	if res.TimedOut || res.Truncated {
		t.Errorf("unexpected flags: timedOut=%v truncated=%v", res.TimedOut, res.Truncated)
	}
	// warden.executed must be published.
	if got := countEvents(t, j, event.KindWardenExecuted); got != 1 {
		t.Errorf("warden.executed count=%d want 1", got)
	}
}

func TestRun_PropagatesNonZeroExit(t *testing.T) {
	b, _ := newBus(t)
	e := warden.New(b)
	res, err := e.Run(context.Background(), warden.Spec{
		Profile: warden.ProfileNone,
		Argv:    failArgv(),
	})
	if err != nil {
		t.Fatalf("Run returned engine err for plain non-zero exit: %v", err)
	}
	if res.ExitCode != 7 {
		t.Errorf("exit=%d want 7", res.ExitCode)
	}
}

func TestRun_RejectsEmptyArgv(t *testing.T) {
	e := warden.New(nil)
	_, err := e.Run(context.Background(), warden.Spec{Profile: warden.ProfileNone})
	if err == nil {
		t.Fatal("expected ErrBadSpec for empty Argv")
	}
}

// ---- timeout ----

func TestRun_TimeoutKillsAndFlagsTimedOut(t *testing.T) {
	b, j := newBus(t)
	e := warden.New(b)

	start := time.Now()
	res, err := e.Run(context.Background(), warden.Spec{
		Profile: warden.ProfileNone,
		Argv:    busyArgv(),
		Limits:  warden.Limits{Timeout: 200 * time.Millisecond, WaitDelay: 500 * time.Millisecond},
	})
	dur := time.Since(start)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !res.TimedOut {
		t.Error("expected TimedOut=true")
	}
	// 200ms timeout + 500ms WaitDelay = strict upper bound ~1.5s.
	if dur > 2*time.Second {
		t.Errorf("Run took %s; expected <2s (timeout + WaitDelay)", dur)
	}
	// warden.limit_exceeded for the timeout.
	if got := countEvents(t, j, event.KindWardenLimitExceeded); got < 1 {
		t.Errorf("limit_exceeded count=%d want >=1", got)
	}
}

// ---- output truncation ----

func TestRun_OutputTruncated(t *testing.T) {
	b, j := newBus(t)
	e := warden.New(b)

	// Emit ~10 KiB of output; cap at 1 KiB so truncation flags fire.
	var argv []string
	if runtime.GOOS == "windows" {
		// PowerShell would be ideal but cmd is universal. Repeat a
		// short line many times.
		argv = []string{"cmd", "/C", "for /L %i in (1,1,200) do @echo AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}
	} else {
		argv = []string{"sh", "-c", "for i in $(seq 1 200); do echo AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA; done"}
	}
	res, err := e.Run(context.Background(), warden.Spec{
		Profile: warden.ProfileNone,
		Argv:    argv,
		Limits:  warden.Limits{MaxOutputBytes: 1024},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !res.Truncated {
		t.Errorf("expected Truncated=true, stdout=%d bytes", len(res.Stdout))
	}
	if len(res.Stdout) > 1024 {
		t.Errorf("stdout=%d bytes; cap=1024", len(res.Stdout))
	}
	if got := countEvents(t, j, event.KindWardenLimitExceeded); got < 1 {
		t.Errorf("limit_exceeded count=%d want >=1", got)
	}
}

// ---- profile downgrade ----

func TestRun_DowngradesNamespaceToNone(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("on linux ProfileNamespace stays as Namespace (M1.d); this test covers the non-linux downgrade path")
	}
	b, j := newBus(t)
	e := warden.New(b)
	res, err := e.Run(context.Background(), warden.Spec{
		Profile: warden.ProfileNamespace,
		Argv:    echoArgv("ok"),
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !res.Downgraded {
		t.Error("expected Downgraded=true on a non-namespace-capable build")
	}
	if res.RequestedProfile != warden.ProfileNamespace {
		t.Errorf("requested=%s want namespace", res.RequestedProfile)
	}
	if res.EffectiveProfile != warden.ProfileNone {
		t.Errorf("effective=%s want none (M1.c)", res.EffectiveProfile)
	}
	// warden.profile_downgraded must be published.
	if got := countEvents(t, j, event.KindWardenProfileDowngraded); got != 1 {
		t.Errorf("profile_downgraded count=%d want 1", got)
	}

	// Second call with same requested profile must NOT publish again
	// (once-per-profile dedup); execution still proceeds.
	_, err = e.Run(context.Background(), warden.Spec{
		Profile: warden.ProfileNamespace,
		Argv:    echoArgv("ok2"),
	})
	if err != nil {
		t.Fatalf("Run #2: %v", err)
	}
	if got := countEvents(t, j, event.KindWardenProfileDowngraded); got != 1 {
		t.Errorf("profile_downgraded count after 2 runs=%d want 1 (deduped)", got)
	}
}

func TestEffectiveProfile_AllRequestsDowngradeInM1c(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("M1.d on linux upgrades ProfileNamespace handling; covered by TestEffectiveProfile_LinuxKeepsNamespace")
	}
	e := warden.New(nil)
	cases := []warden.Profile{warden.ProfileNamespace, warden.ProfileContainer, warden.ProfileMicroVM}
	for _, p := range cases {
		if got := e.EffectiveProfile(p); got != warden.ProfileNone {
			t.Errorf("EffectiveProfile(%s)=%s want none in M1.c", p, got)
		}
	}
	if got := e.EffectiveProfile(warden.ProfileNone); got != warden.ProfileNone {
		t.Errorf("EffectiveProfile(none)=%s want none", got)
	}
}

// TestEffectiveProfile_LinuxKeepsNamespace covers the M1.d
// behavior: on linux, ProfileNamespace stays at Namespace (engages
// setpgid + rlimit hardening) and Container/MicroVM downgrade to
// Namespace as next-best-available. Skipped on non-linux.
func TestEffectiveProfile_LinuxKeepsNamespace(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("M1.d hardening is linux-only")
	}
	e := warden.New(nil)
	if got := e.EffectiveProfile(warden.ProfileNamespace); got != warden.ProfileNamespace {
		t.Errorf("EffectiveProfile(namespace) on linux = %s, want namespace", got)
	}
	for _, p := range []warden.Profile{warden.ProfileContainer, warden.ProfileMicroVM} {
		if got := e.EffectiveProfile(p); got != warden.ProfileNamespace {
			t.Errorf("EffectiveProfile(%s) on linux = %s, want namespace (best available)", p, got)
		}
	}
	if got := e.EffectiveProfile(warden.ProfileNone); got != warden.ProfileNone {
		t.Errorf("EffectiveProfile(none) on linux = %s, want none", got)
	}
}

func TestEffectiveProfile_UnknownDowngradesToNone(t *testing.T) {
	e := warden.New(nil)
	if got := e.EffectiveProfile(warden.Profile("bogus")); got != warden.ProfileNone {
		t.Errorf("unknown profile resolves to %s, want none", got)
	}
}

// ---- event payload shape ----

func TestEvent_ExecutedPayloadShape(t *testing.T) {
	b, j := newBus(t)
	e := warden.New(b)
	_, err := e.Run(context.Background(), warden.Spec{
		Profile:       warden.ProfileNone,
		Argv:          echoArgv("payload-test"),
		Actor:         "test-actor",
		CorrelationID: "corr-xyz",
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	var executed *event.Event
	_ = j.Range(func(e *event.Event) error {
		if e.Kind == event.KindWardenExecuted {
			executed = e
		}
		return nil
	})
	if executed == nil {
		t.Fatal("warden.executed not in journal")
	}
	if executed.Actor != "test-actor" {
		t.Errorf("actor=%q want test-actor", executed.Actor)
	}
	if executed.CorrelationID != "corr-xyz" {
		t.Errorf("correlation=%q want corr-xyz", executed.CorrelationID)
	}
	var p struct {
		ProfileEffective string `json:"profile_effective"`
		ProfileRequested string `json:"profile_requested"`
		Downgraded       bool   `json:"downgraded"`
		ExitCode         int    `json:"exit_code"`
		DurationMS       int64  `json:"duration_ms"`
		HostOS           string `json:"host_os"`
	}
	if err := json.Unmarshal(executed.Payload, &p); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if p.ProfileEffective != "none" || p.ProfileRequested != "none" {
		t.Errorf("profiles: effective=%q requested=%q", p.ProfileEffective, p.ProfileRequested)
	}
	if p.Downgraded {
		t.Error("downgraded=true for explicit none request")
	}
	if p.ExitCode != 0 {
		t.Errorf("exit_code=%d want 0", p.ExitCode)
	}
	if p.HostOS != runtime.GOOS {
		t.Errorf("host_os=%q want %q", p.HostOS, runtime.GOOS)
	}
}

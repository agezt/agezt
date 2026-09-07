package shell

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/warden"
)

// stubEngine returns a canned Result without running anything, so tests can
// drive the Tool's real Invoke/renderResult path with exact stream bytes.
type stubEngine struct {
	res warden.Result
}

func (s *stubEngine) Run(_ context.Context, _ warden.Spec) (*warden.Result, error) {
	r := s.res
	return &r, nil
}

func (s *stubEngine) EffectiveProfile(p warden.Profile) warden.Profile { return p }

func (s *stubEngine) SetBus(_ *bus.Bus) {}

// The tool documents a 64 KiB model-facing budget ("Output is truncated to
// 64 KiB"), but warden caps stdout and stderr EACH at MaxOutputBytes and
// renderResult concatenates them — so a command with ~40 KiB on both streams
// shipped ~80 KiB to the model, silently (neither stream alone trips the cap,
// so warden's Truncated flag stays false and no marker was prepended).
func TestInvoke_EnforcesModelFacingOutputBudget(t *testing.T) {
	tl := &Tool{Warden: &stubEngine{res: warden.Result{
		Stdout:   bytes.Repeat([]byte("a"), 40*1024),
		Stderr:   bytes.Repeat([]byte("b"), 40*1024),
		ExitCode: 0,
	}}}
	raw, err := json.Marshal(map[string]any{"command": "x"})
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}
	r, err := tl.Invoke(context.Background(), raw)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if r.IsError {
		t.Fatalf("unexpected tool error: %s", r.Output)
	}
	if len(r.Output) > MaxOutputBytes {
		t.Fatalf("combined output is %d bytes, want <= %d: stdout and stderr are each capped at %d and then concatenated, doubling the documented model-facing budget",
			len(r.Output), MaxOutputBytes, MaxOutputBytes)
	}
	if !strings.Contains(r.Output, "[truncated") {
		t.Fatalf("combined output exceeded the budget without a truncation marker")
	}
}

// Under the budget the output must be passed through byte-for-byte: the
// enforcement must not eat legitimate small results.
func TestInvoke_UnderBudgetUntouched(t *testing.T) {
	stdout := bytes.Repeat([]byte("a"), 30*1024)
	stderr := bytes.Repeat([]byte("b"), 30*1024)
	tl := &Tool{Warden: &stubEngine{res: warden.Result{
		Stdout:   stdout,
		Stderr:   stderr,
		ExitCode: 0,
	}}}
	raw, err := json.Marshal(map[string]any{"command": "x"})
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}
	r, err := tl.Invoke(context.Background(), raw)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	want := string(stdout) + "\n" + string(stderr)
	if r.Output != want {
		t.Fatalf("under-budget output altered: got %d bytes, want %d (and no marker): %q...", len(r.Output), len(want), r.Output[:min(80, len(r.Output))])
	}
}

// Warden's own truncation flag must still produce the marker even when the
// combined output is small (a single stream was truncated by warden).
func TestInvoke_WardenTruncatedStillMarked(t *testing.T) {
	tl := &Tool{Warden: &stubEngine{res: warden.Result{
		Stdout:    bytes.Repeat([]byte("a"), 100),
		Truncated: true,
		ExitCode:  0,
	}}}
	raw, err := json.Marshal(map[string]any{"command": "x"})
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}
	r, err := tl.Invoke(context.Background(), raw)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !strings.HasPrefix(r.Output, "[truncated to last 64 KiB]\n") {
		t.Fatalf("warden-truncated output lost its marker: %q", r.Output[:min(60, len(r.Output))])
	}
}

// Real-path proof: an actual cmd run emitting ~42 KiB on each stream must not
// hand the model ~87 KiB.
func TestInvoke_RealWarden_CombinedBudgetHeld(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("cmd-specific output generator")
	}
	tl := NewWithWarden(warden.New(nil))
	tl.Profile = warden.ProfileNone
	line60 := strings.Repeat("A", 60)
	err60 := strings.Repeat("B", 60)
	command := fmt.Sprintf("for /l %%i in (1,1,700) do @echo %s & for /l %%i in (1,1,700) do @echo %s 1>&2", line60, err60)
	raw, err := json.Marshal(map[string]any{"command": command})
	if err != nil {
		t.Fatalf("marshal input: %v", err)
	}
	r, err := tl.Invoke(context.Background(), raw)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if len(r.Output) > MaxOutputBytes {
		t.Fatalf("real-path combined output is %d bytes, want <= %d (no truncation marker: %v)",
			len(r.Output), MaxOutputBytes, strings.Contains(r.Output, "[truncated"))
	}
}

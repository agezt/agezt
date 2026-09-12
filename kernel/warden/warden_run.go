// SPDX-License-Identifier: MIT

// Warden engine: New + NewWithOptions + SetBus + EffectiveProfile + Run + classifyWaitErr + publishExecuted + publishDowngradeOnce + publishLimitExceeded.
// Code extracted from warden.go during the Day-72 god-file split. Public API unchanged.
package warden


import (
	"context"
	"errors"
	"fmt"
	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/event"
	"os/exec"
	"runtime"
	"time"
)


func New(b *bus.Bus) Engine {
	return NewWithOptions(b, Options{})
}

// NewWithOptions constructs the default engine with optional external
// backends. The zero Options value is identical to New.
func NewWithOptions(b *bus.Bus, opts Options) Engine {
	return &engine{
		bus:             b,
		container:       normalizeContainerOptions(opts.Container),
		downgradeWarned: map[Profile]struct{}{},
	}
}

// SetBus implements Engine. See interface docs.
func (e *engine) SetBus(b *bus.Bus) { e.bus = b }

// EffectiveProfile reports what a request for p resolves to *today*.
//
//   - On non-Linux hosts: everything downgrades to ProfileNone (M1.c).
//   - On Linux (M1.d): ProfileNamespace stays as-is and engages the
//     rlimit + Setpgid hardening in warden_linux.go. Container and
//     MicroVM still downgrade to Namespace (next-best available).
//
// The platform split is in resolveEffectiveProfile (defined per-OS).
func (e *engine) EffectiveProfile(p Profile) Profile {
	if !p.IsKnown() {
		return ProfileNone
	}
	if p == ProfileContainer && e.container.active() {
		return ProfileContainer
	}
	return resolveEffectiveProfile(p)
}

// ErrBadSpec is returned by Run for a malformed Spec.
var ErrBadSpec = errors.New("warden: bad spec")

// Run executes the spec and returns the result. See package docs for
// what's enforced.
func (e *engine) Run(ctx context.Context, spec Spec) (*Result, error) {
	if len(spec.Argv) == 0 || spec.Argv[0] == "" {
		return nil, fmt.Errorf("%w: empty Argv", ErrBadSpec)
	}
	requested := spec.Profile
	if !requested.IsKnown() {
		requested = ProfileNone
	}
	effective := e.EffectiveProfile(requested)
	downgraded := effective != requested

	if downgraded {
		e.publishDowngradeOnce(spec, requested, effective)
	}

	execSpec := spec
	if effective == ProfileContainer {
		argv, err := buildContainerArgv(spec, e.container)
		if err != nil {
			return nil, fmt.Errorf("warden: container: %w", err)
		}
		execSpec.Argv = argv
		execSpec.WorkDir = ""
		execSpec.Env = []string{}
	}

	timeout := spec.Limits.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	maxOut := spec.Limits.MaxOutputBytes
	if maxOut <= 0 {
		maxOut = DefaultMaxOutputBytes
	}
	waitDelay := spec.Limits.WaitDelay
	if waitDelay <= 0 {
		waitDelay = DefaultWaitDelay
	}

	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(runCtx, execSpec.Argv[0], execSpec.Argv[1:]...)
	cmd.Dir = execSpec.WorkDir
	// Honor the documented contract that a nil Env means an EMPTY
	// environment (most restrictive), NOT inheritance (M186). Go's
	// os/exec treats cmd.Env == nil as "inherit the parent's
	// environment", which would leak the daemon's secrets (API keys,
	// tokens, AWS_*, …) into an untrusted child — the exact opposite of
	// what Spec.Env documents and what callers like pulse's probe runner
	// (Env: nil) rely on. Translate nil to an explicit empty slice so the
	// documented default is also the safe one. A caller that genuinely
	// wants inheritance must pass os.Environ() explicitly.
	if execSpec.Env == nil {
		cmd.Env = []string{}
	} else {
		cmd.Env = execSpec.Env
	}
	cmd.WaitDelay = waitDelay

	// M1.d: platform-specific pre-Start setup (sets SysProcAttr on
	// Linux for Setpgid so kill-on-timeout sweeps grandchildren;
	// no-op on non-Linux).
	configurePlatformAttrs(cmd, effective)
	// M958: on Windows, hand `cmd /C <command>` to cmd.exe verbatim (cmd /S /C
	// "<command>") so a quoted command isn't mangled by os/exec's MSVC-style
	// escaping. No-op off Windows and for non-cmd invocations.
	fixupWindowsCmd(cmd)

	stdoutBuf := newCapBuffer(maxOut)
	stderrBuf := newCapBuffer(maxOut)
	cmd.Stdout = stdoutBuf
	cmd.Stderr = stderrBuf

	start := time.Now()
	if err := cmd.Start(); err != nil {
		end := time.Now()
		res := &Result{
			EffectiveProfile: effective,
			RequestedProfile: requested,
			Downgraded:       downgraded,
			ExitCode:         -1,
			Stdout:           stdoutBuf.Bytes(),
			Stderr:           stderrBuf.Bytes(),
			Started:          start,
			Ended:            end,
			Duration:         end.Sub(start),
		}
		e.publishExecuted(spec, res)
		return res, fmt.Errorf("warden: start %q: %w", execSpec.Argv[0], err)
	}

	// M1.d: platform-specific post-Start hardening (best-effort
	// Prlimit on Linux for CPU/AS/NOFILE/FSIZE; no-op on non-Linux).
	// Errors are surfaced via a warden.limit event but do NOT abort
	// the run — the existing wall-clock timeout still bounds the
	// worst case if rlimits can't be applied.
	applyPlatformLimits(cmd, execSpec, effective, e)

	err := cmd.Wait()
	end := time.Now()

	res := &Result{
		EffectiveProfile: effective,
		RequestedProfile: requested,
		Downgraded:       downgraded,
		ExitCode:         -1,
		Stdout:           stdoutBuf.Bytes(),
		Stderr:           stderrBuf.Bytes(),
		Truncated:        stdoutBuf.Truncated() || stderrBuf.Truncated(),
		TimedOut:         runCtx.Err() == context.DeadlineExceeded,
		Duration:         end.Sub(start),
		Started:          start,
		Ended:            end,
	}

	if cmd.ProcessState != nil {
		res.ExitCode = cmd.ProcessState.ExitCode()
	}
	if res.TimedOut {
		e.publishLimitExceeded(spec, "timeout", timeout)
	}
	if res.Truncated {
		e.publishLimitExceeded(spec, "output_bytes", maxOut)
	}

	e.publishExecuted(spec, res)

	if cerr := classifyWaitErr(err, res.TimedOut, execSpec.Argv[0]); cerr != nil {
		return res, cerr
	}
	return res, nil
}

// classifyWaitErr decides whether a cmd.Wait error is an engine-level failure to
// surface, or a normal process outcome to absorb into Result. A nil error or a
// timed-out run is normal (the timeout is reported via Result.TimedOut). An
// *exec.ExitError means the process ran — a non-zero exit is the caller's to
// interpret via Result.ExitCode — so it is absorbed. Anything else (failed launch,
// I/O error, WaitDelay abandonment after a kill) is a genuine engine failure and is
// returned. The earlier guard also required Result.ExitCode == 0, which wrongly
// SWALLOWED a non-ExitError failure whenever it coincided with a non-zero exit code
// (the common case for a killed/abandoned process), hiding it from the caller. (M475)
func classifyWaitErr(err error, timedOut bool, argv0 string) error {
	if err == nil || timedOut {
		return nil
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return nil
	}
	return fmt.Errorf("warden: exec %q: %w", argv0, err)
}

func (e *engine) publishExecuted(spec Spec, res *Result) {
	if e.bus == nil {
		return
	}
	_, _ = e.bus.Publish(event.Spec{
		Subject:       "warden.exec",
		Kind:          event.KindWardenExecuted,
		Actor:         actorOrDefault(spec.Actor),
		CorrelationID: spec.CorrelationID,
		Payload: map[string]any{
			"profile_effective": string(res.EffectiveProfile),
			"profile_requested": string(res.RequestedProfile),
			"downgraded":        res.Downgraded,
			"argv0":             spec.Argv[0],
			"exit_code":         res.ExitCode,
			"duration_ms":       res.Duration.Milliseconds(),
			"stdout_bytes":      len(res.Stdout),
			"stderr_bytes":      len(res.Stderr),
			"truncated":         res.Truncated,
			"timed_out":         res.TimedOut,
			"workdir":           spec.WorkDir,
			"host_os":           runtime.GOOS,
		},
	})
}

func (e *engine) publishDowngradeOnce(spec Spec, req, eff Profile) {
	e.mu.Lock()
	if _, seen := e.downgradeWarned[req]; seen {
		e.mu.Unlock()
		return
	}
	e.downgradeWarned[req] = struct{}{}
	e.mu.Unlock()
	if e.bus == nil {
		return
	}
	_, _ = e.bus.Publish(event.Spec{
		Subject:       "warden.profile",
		Kind:          event.KindWardenProfileDowngraded,
		Actor:         actorOrDefault(spec.Actor),
		CorrelationID: spec.CorrelationID,
		Payload: map[string]any{
			"requested": string(req),
			"effective": string(eff),
			"host_os":   runtime.GOOS,
			"reason":    downgradeReason(req),
		},
	})
}

func (e *engine) publishLimitExceeded(spec Spec, limit string, value any) {
	if e.bus == nil {
		return
	}
	_, _ = e.bus.Publish(event.Spec{
		Subject:       "warden.limit",
		Kind:          event.KindWardenLimitExceeded,
		Actor:         actorOrDefault(spec.Actor),
		CorrelationID: spec.CorrelationID,
		Payload: map[string]any{
			"limit": limit,
			"value": value,
			"argv0": spec.Argv[0],
		},
	})
}
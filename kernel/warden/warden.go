// SPDX-License-Identifier: MIT

// Package warden is the process-isolation layer (SPEC-06 §2; TASKS
// P1-WARD-01). It defines four named isolation profiles and a single
// Engine interface that tools (shell, browser, third-party plugins) use
// to run external work without caring about the underlying OS
// mechanism.
//
// Profiles (SPEC-06 §2):
//
//	ProfileNone        in-process / direct exec, no isolation
//	ProfileNamespace   Linux namespaces + cgroups + seccomp
//	ProfileContainer   OCI container (Docker / Podman)
//	ProfileMicroVM     lightweight VM (firecracker-class)
//
// **M1.c ships ProfileNone universally on every platform; stronger
// profiles are documented and *requested*, but transparently downgraded
// to ProfileNone with a `warden.profile_downgraded` event so the operator
// knows the actual isolation level.** Real Linux namespace + cgroups
// implementation lands in M1.d behind a `//go:build linux` partition;
// container/microvm backends are M2+ optional plugins (SPEC-06 §2.2).
//
// What the cross-platform Engine already enforces today:
//
//   - **Timeout**: ctx-with-deadline + cmd.WaitDelay to bound orphaned
//     child IO.
//   - **Output truncation**: stdout/stderr capped at MaxOutputBytes.
//   - **Working directory** scoping.
//   - **Environment scrubbing**: child inherits only an allowlist.
//   - **Audit**: every Run emits a `warden.executed` event with
//     {profile_effective, profile_requested, exit_code, durations, bytes,
//     truncated, timed_out}.
package warden

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"
	"sync"
	"time"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/event"
)

// Profile is one of the four named isolation modes from SPEC-06 §2.
type Profile string

const (
	ProfileNone      Profile = "none"
	ProfileNamespace Profile = "namespace"
	ProfileContainer Profile = "container"
	ProfileMicroVM   Profile = "microvm"
)

// IsKnown reports whether p is one of the four canonical profiles.
func (p Profile) IsKnown() bool {
	switch p {
	case ProfileNone, ProfileNamespace, ProfileContainer, ProfileMicroVM:
		return true
	}
	return false
}

// DefaultMaxOutputBytes caps captured stdout+stderr per Run. 256 KiB is
// large enough to carry a tool's full output even after a verbose run
// and small enough to keep one runaway invocation from blowing the
// journal budget. SPEC-02 §5 lists 256 KiB as the inline-attachment
// ceiling — we match it.
const DefaultMaxOutputBytes = 256 * 1024

// DefaultTimeout caps a single Run's wall time when no explicit
// timeout is provided.
const DefaultTimeout = 30 * time.Second

// DefaultWaitDelay bounds how long Wait() blocks for orphaned-child IO
// after the wrapper process is killed. Windows in particular needs this
// because killing cmd.exe does not reap its children.
const DefaultWaitDelay = 500 * time.Millisecond

// Limits caps a single Run. Zero-value means "use the package default".
type Limits struct {
	// Timeout is the wall-clock cap for the entire Run.
	Timeout time.Duration
	// MaxOutputBytes caps stdout+stderr combined; excess is dropped from
	// the head so the model still sees the most-recent output.
	MaxOutputBytes int
	// WaitDelay bounds Wait() after context cancel; see DefaultWaitDelay.
	WaitDelay time.Duration

	// ---- M1.d Linux-only resource limits ----
	//
	// All four fields are honored ONLY when the host is Linux AND
	// the request asked for ProfileNamespace (or stronger). On
	// non-Linux hosts and on Linux with ProfileNone, these fields
	// are silently ignored. Zero = "no extra limit beyond the OS
	// default" — leaving them zero is the right thing for tools
	// that genuinely need lots of CPU/memory.
	//
	// **Why best-effort.** Without unprivileged user-namespaces
	// (which need root or a sysctl twiddle most operators don't
	// set), we can't apply rlimits BEFORE the child execs. We
	// call Prlimit on the child PID right after Start; there is a
	// small window where the child can allocate before the limit
	// applies. Operators who need hard guarantees should use
	// ProfileContainer (out of M1 scope) — these limits are
	// hardening against accidental runaway, not malicious escape.

	// CPUSeconds caps total CPU time the child can accumulate
	// (RLIMIT_CPU). Hitting it triggers SIGXCPU; the kernel then
	// gives the child 1s grace before SIGKILL.
	CPUSeconds int

	// AddressSpaceBytes caps virtual memory (RLIMIT_AS). Go-runtime
	// children reserve a LOT of virtual address space upfront —
	// set this to at least ~1 GiB for Go binaries or they will
	// fail to start. For shell tools (bash/curl/jq), 256 MiB is
	// generous. Zero disables the cap.
	AddressSpaceBytes uint64

	// MaxOpenFiles caps file descriptors (RLIMIT_NOFILE). Hitting
	// it makes open(2) return EMFILE. Reasonable default for shell
	// tools: 256. Zero disables the cap.
	MaxOpenFiles uint64

	// MaxFileSizeBytes caps the size of any single file the child
	// writes (RLIMIT_FSIZE). Hitting it triggers SIGXFSZ. Useful
	// to prevent runaway writes from filling the disk. Zero
	// disables the cap.
	MaxFileSizeBytes uint64
}

// Spec is one isolated execution request.
type Spec struct {
	// Profile is the *requested* isolation level. If the engine can't
	// satisfy it on this host, it transparently downgrades to the
	// strongest available and emits warden.profile_downgraded.
	Profile Profile
	// Argv is the program plus arguments. Argv[0] is the binary; the
	// engine does NOT spawn a shell. Callers that want shell expansion
	// must pass {"sh", "-c", cmd} or {"cmd", "/C", cmd} themselves.
	Argv []string
	// WorkDir is the child's working directory. Empty = inherit.
	WorkDir string
	// Env is the *exact* environment the child sees. Nil = empty
	// environment (most restrictive); pass os.Environ() to inherit
	// everything (least restrictive).
	Env []string
	// Limits override the engine defaults; zero fields use defaults.
	Limits Limits
	// Actor is published in the warden.executed event so operators can
	// see *which* tool invoked the run.
	Actor string
	// CorrelationID is propagated to events so `agt why <id>` walks the
	// chain back to the originating task.
	CorrelationID string
}

// Result is what a single Run produced.
type Result struct {
	// EffectiveProfile is what actually ran (may differ from requested
	// after downgrade).
	EffectiveProfile Profile
	// RequestedProfile is what the caller asked for.
	RequestedProfile Profile
	// Downgraded reports whether Effective < Requested.
	Downgraded bool
	// ExitCode of the process. -1 for "did not run" or "killed before
	// completion".
	ExitCode int
	// Stdout / Stderr captured up to Limits.MaxOutputBytes.
	Stdout []byte
	Stderr []byte
	// Truncated reports whether either stream was tail-truncated.
	Truncated bool
	// TimedOut reports whether the deadline killed the process.
	TimedOut bool
	// Duration is the actual wall time the process consumed.
	Duration time.Duration
	// Started/Ended timestamps for the event payload.
	Started time.Time
	Ended   time.Time
}

// Engine runs Specs and emits audit events to the bus.
type Engine interface {
	// Run executes one Spec to completion. Returns a Result even on
	// non-zero exit; an error is returned only for issues that
	// prevented running at all (bad spec, exec lookup failure).
	Run(ctx context.Context, spec Spec) (*Result, error)
	// EffectiveProfile reports what a request for p would actually run
	// as on this host. Useful for the daemon banner.
	EffectiveProfile(p Profile) Profile
	// SetBus attaches a bus post-construction. The daemon builds the
	// Warden before runtime.Open creates the kernel bus, so this lets
	// the wiring close the loop without circular-init gymnastics.
	// MUST be called before the first Run; the bus pointer is read
	// without a lock on the hot path.
	SetBus(b *bus.Bus)
}

// engine is the default cross-platform implementation. The Linux
// build-tag partition will eventually attach extra fields and override
// EffectiveProfile/Run for namespace+cgroups support.
type engine struct {
	bus *bus.Bus

	// mu guards downgradeWarned so we only journal one downgrade event
	// per (requested-profile) per process lifetime — repeated tool
	// calls don't spam the journal with identical warnings.
	mu              sync.Mutex
	downgradeWarned map[Profile]struct{}
}

// New constructs the default engine. b may be nil for tests; events are
// silently dropped in that case.
func New(b *bus.Bus) Engine {
	return &engine{
		bus:             b,
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

	cmd := exec.CommandContext(runCtx, spec.Argv[0], spec.Argv[1:]...)
	cmd.Dir = spec.WorkDir
	// Honor the documented contract that a nil Env means an EMPTY
	// environment (most restrictive), NOT inheritance (M186). Go's
	// os/exec treats cmd.Env == nil as "inherit the parent's
	// environment", which would leak the daemon's secrets (API keys,
	// tokens, AWS_*, …) into an untrusted child — the exact opposite of
	// what Spec.Env documents and what callers like pulse's probe runner
	// (Env: nil) rely on. Translate nil to an explicit empty slice so the
	// documented default is also the safe one. A caller that genuinely
	// wants inheritance must pass os.Environ() explicitly.
	if spec.Env == nil {
		cmd.Env = []string{}
	} else {
		cmd.Env = spec.Env
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
		return res, fmt.Errorf("warden: start %q: %w", spec.Argv[0], err)
	}

	// M1.d: platform-specific post-Start hardening (best-effort
	// Prlimit on Linux for CPU/AS/NOFILE/FSIZE; no-op on non-Linux).
	// Errors are surfaced via a warden.limit event but do NOT abort
	// the run — the existing wall-clock timeout still bounds the
	// worst case if rlimits can't be applied.
	applyPlatformLimits(cmd, spec, effective, e)

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

	if cerr := classifyWaitErr(err, res.TimedOut, spec.Argv[0]); cerr != nil {
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

func actorOrDefault(a string) string {
	if a == "" {
		return "warden"
	}
	return a
}

func downgradeReason(req Profile) string {
	switch req {
	case ProfileNamespace:
		return "linux full-namespace backend (CLONE_NEWUSER + cgroups + seccomp) not built; M1.d ships only setpgid + rlimit hardening on linux, full downgrade elsewhere"
	case ProfileContainer:
		return "OCI container backend is an M2+ optional plugin (SPEC-06 §2.2)"
	case ProfileMicroVM:
		return "microVM backend is an M2+ optional plugin (SPEC-06 §2.2)"
	}
	return "unknown profile"
}

type ctxKey int

const ctxKeyCorrelation ctxKey = iota

// WithCorrelation returns a child context carrying corr so a tool that runs
// commands through the warden (e.g. the shell tool) can stamp it onto Spec.
// CorrelationID without threading the run id by hand. The runtime sets this on
// every run's context; the resulting warden.executed / warden.profile_downgraded
// events then carry the run correlation, so they appear in the run-detail
// timeline and `agt why <id>` walks back through them. Without it the warden
// events still record (just unlinked from a run).
func WithCorrelation(ctx context.Context, corr string) context.Context {
	return context.WithValue(ctx, ctxKeyCorrelation, corr)
}

// CorrelationFrom extracts the correlation id set by WithCorrelation, or "" if
// none was set.
func CorrelationFrom(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeyCorrelation).(string); ok {
		return v
	}
	return ""
}

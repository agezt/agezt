// SPDX-License-Identifier: MIT

// Package wardenapi is the contract layer's home for the sandbox rows: the
// isolation profile a run asks for, the limits a run is capped by, the
// request that describes a run, the result a run produced, and the port
// through which a caller drives an isolated execution.
//
// The rows live here because the tools that route their process launches
// through the warden are PLUGINS, and a plugin may reach the contract and the
// platform layers and nothing else. The implementation that enforces them —
// the engine that spawns the child, downgrades the profile it cannot honor
// and emits the audit event — stays in the platform (kernel/warden); the
// platform keeps the names its own callers use as aliases, so there is one
// row and one spelling of it.
//
// The port is deliberately NARROWER than the platform's engine: the platform's
// engine also carries SetBus, which names the bus — a row the contract layer
// may not import (archcheck's principle P4). A caller of the contract's port
// only ever runs a spec and asks what profile a request would really get, so
// the port carries exactly that.
//
// The rules of the road:
//
//   - Rows, constants and pure functions only. No process, no file, no clock.
//   - Pure. It may import the standard library and other contract packages
//     only (archcheck's principle P4).
package wardenapi

import (
	"context"
	"time"
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

// Engine runs Specs and emits audit events.
//
// The port is the narrow slice of the platform's engine: it runs a spec and
// reports what a requested profile would really become on this host. The
// platform's engine adds SetBus (the audit event's sink) — that row names
// the bus, which the contract layer may not import, so it stays in the
// platform.
type Engine interface {
	// Run executes one Spec to completion. Returns a Result even on
	// non-zero exit; an error is returned only for issues that
	// prevented running at all (bad spec, exec lookup failure).
	Run(ctx context.Context, spec Spec) (*Result, error)
	// EffectiveProfile reports what a request for p would actually run
	// as on this host. Useful for the daemon banner.
	EffectiveProfile(p Profile) Profile
}

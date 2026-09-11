// SPDX-License-Identifier: MIT

package plugin

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"

	"github.com/agezt/agezt/kernel/agent"
)

// DefaultInitTimeout caps the initialize round-trip. Plugins that
// take longer to start (Python imports, large model loads) should
// either (a) defer expensive work until the first tool call, or
// (b) bump this via Host.Config.InitTimeout.
const DefaultInitTimeout = 10 * time.Second

// DefaultInvokeTimeout caps a single tool/invoke round-trip.
const DefaultInvokeTimeout = 2 * time.Minute

// DefaultShutdownGrace is how long Host.Close waits for the plugin
// to exit after sending shutdown before sending a kill signal.
const DefaultShutdownGrace = 5 * time.Second

// DefaultMaxFrameBytes bounds a single newline-delimited frame read
// off a plugin's stdout (M177). The stream comes from an untrusted
// child: without a cap, a plugin that writes bytes but never emits a
// '\n' (or emits one pathologically large line) drives the host's
// bufio reader to allocate without limit, OOM-killing the whole
// daemon — one buggy/hostile plugin taking down every other plugin
// and the kernel. 16 MiB is generous for legitimate JSON tool
// results while still bounding the blast radius; a plugin that
// exceeds it is torn down (markDead) rather than the daemon.
const DefaultMaxFrameBytes = 16 << 20

// errFrameTooLarge is the cause recorded when a plugin's stdout frame
// exceeds Config.MaxFrameBytes. The plugin is marked dead; in-flight
// callers fail fast instead of the daemon dying under memory pressure.
var errFrameTooLarge = errors.New("plugin: stdout frame exceeds max size")

// DefaultMaxConcurrentCallbacks bounds how many plugin→host callbacks
// (host/invoke) run at once for a single plugin (M181). The plugin's
// stdout is untrusted: without a cap, a plugin that streams host/invoke
// frames as fast as the host reads them spawns an unbounded number of
// goroutines, each running a host tool with up to InvokeTimeout —
// goroutine/memory exhaustion plus amplification of whatever those tools
// touch. Excess callbacks past the cap are rejected with
// ErrTooManyCallbacks rather than queued, keeping the read loop
// responsive and goroutines bounded.
const DefaultMaxConcurrentCallbacks = 16

// ErrTooManyCallbacks is returned to a plugin when it has too many
// host/invoke callbacks already in flight (M181).
var ErrTooManyCallbacks = errors.New("plugin: too many concurrent callbacks")

// DefaultMaxAdvertisedTools caps how many tools a plugin may advertise
// in its initialize result (M182). The frame-size bound (M177) limits
// the raw initialize bytes, but ~1M tiny tool defs still fit in 16 MiB,
// and each becomes a registry map entry + remoteTool wrapper at
// registration — a memory blow-up at spawn. Real plugins advertise a
// handful to a few dozen tools; 256 is generous while bounding the
// blast radius. A plugin past the cap fails to spawn.
const DefaultMaxAdvertisedTools = 256

// ErrTooManyTools is returned by Spawn/Reload when a plugin advertises
// more tools than Config.MaxAdvertisedTools (M182).
var ErrTooManyTools = errors.New("plugin: advertised tool count exceeds max")

// Config tunes a Plugin.
type Config struct {
	// Path to the plugin executable. Required.
	Path string
	// Args passed after Path. Optional.
	Args []string
	// Env is the child's environment. Nil inherits the parent's.
	Env []string
	// Dir is the child's working directory. Empty inherits.
	Dir string
	// InitTimeout overrides DefaultInitTimeout.
	InitTimeout time.Duration
	// InvokeTimeout overrides DefaultInvokeTimeout.
	InvokeTimeout time.Duration
	// MaxFrameBytes overrides DefaultMaxFrameBytes — the hard cap on a
	// single newline-delimited stdout frame from the plugin (M177).
	// A frame larger than this tears the plugin down rather than
	// letting an untrusted child drive the host to OOM.
	MaxFrameBytes int
	// MaxConcurrentCallbacks overrides DefaultMaxConcurrentCallbacks —
	// the cap on simultaneous plugin→host callbacks (M181). Excess
	// host/invoke requests are rejected with ErrTooManyCallbacks
	// instead of spawning unbounded goroutines.
	MaxConcurrentCallbacks int
	// MaxAdvertisedTools overrides DefaultMaxAdvertisedTools — the cap
	// on how many tools a plugin may advertise at initialize (M182).
	// A plugin exceeding it fails to spawn with ErrTooManyTools.
	MaxAdvertisedTools int
	// Logger receives stderr from the child (one line per call).
	// Nil discards.
	Logger func(line string)
	// PinnedHash, when non-empty, is the expected BLAKE3-256 digest
	// of the plugin binary as a 64-char lowercase hex string (M1.ff).
	// Spawn computes the digest of the file at Path and refuses to
	// start the child if it doesn't match.
	//
	// Operators pin a plugin by recording its hash once (e.g. via
	// `b3sum` or `agt plugin hash <path>`) and feeding it back via
	// AGEZT_PLUGIN_PINS at daemon startup. A drift — whether
	// accidental (apt upgrade replaced the binary) or malicious
	// (supply-chain compromise swapped it) — surfaces as a clear
	// "plugin pin mismatch" error rather than silent execution.
	//
	// Empty (the default) skips verification entirely — opt-in
	// security so adopting plugins doesn't require setting up the
	// pin infrastructure first.
	PinnedHash string
	// AllowedTools, when non-empty, restricts which of the plugin's
	// advertised tools the host will surface (M1.hh). Spawn returns
	// `ErrToolAllowlistMismatch` when the plugin advertises a tool
	// outside the allowlist (so silent capability expansion in a
	// future plugin release becomes a hard error operators must
	// re-approve, complementing M1.ff's binary-hash pinning).
	//
	// Empty allowlist disables the check — opt-in. Names are
	// compared against the un-prefixed tool name the plugin returns
	// (matches the `agt plugin hash` audit story: the operator sees
	// the same name in their config that the plugin emits).
	AllowedTools []string

	// HostTools (M1.cb) is the set of in-host tools the plugin is
	// allowed to invoke via `host/invoke` callbacks. Keys are the
	// names the plugin uses; values are the tools the host runs.
	// Nil or empty disables callbacks entirely — `host/invoke`
	// requests from the plugin are rejected with
	// ErrCallbacksDisabled.
	//
	// **Why a separate map, not "share the daemon's tool registry".**
	// Plugins should not get a back-door to every tool the host has.
	// The operator wires HostTools explicitly to a curated subset
	// (typically: the basic read-only tools — file read, http get,
	// shell with strict warden caps — that a higher-level plugin
	// needs to gather context). The daemon's wiring code is the
	// audit point.
	//
	// **Loop hazard.** If HostTools contains a remoteTool wrapped
	// around the same plugin, a plugin→host→plugin→host… recursion
	// is possible. The host does NOT guard against this; the
	// invoke timeout caps the total damage. Operators wiring
	// HostTools must avoid the cycle (don't re-include the
	// plugin's own tools).
	HostTools map[string]agent.Tool
}

// Plugin manages one child process. Safe for concurrent calls.
type Plugin struct {
	cfg Config

	mu     sync.Mutex
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader

	// writeMu serializes writes to the child's stdin, separate from mu. The
	// blocking pipe write must NOT be held under mu: deliver (the read loop's
	// response router) takes mu, so holding mu across a stuck write would let a
	// plugin that floods stdout without draining its stdin wedge the read loop
	// against the blocked writer — a host-slot deadlock (M460). Frames still can't
	// interleave because writeMu serializes them; mu is taken only briefly to
	// snapshot stdin (which respawn swaps).
	writeMu sync.Mutex

	// waitDone is closed by a dedicated per-process waiter goroutine once
	// cmd.Wait() returns — i.e. the child has been reaped. It guarantees the
	// process is reaped on ANY death path (self-exit, crash, kill), not only via
	// Close (M422): markDead used to set dead without ever calling Wait, so a
	// plugin that exited on its own became a zombie and Close's dead-check
	// short-circuited the only Wait call site. The waiter is the single owner of
	// cmd.Wait() — nothing else may call it, or Wait would error/race. Replaced on
	// each (re)spawn; read under mu.
	waitDone chan struct{}

	// readDone is closed by readLoop when it returns. Reload waits on the OLD
	// child's readDone before respawn reuses this struct, so a late markDead from
	// the dying loop (e.g. "read stdout: file already closed" when Close shuts the
	// old pipe) can't clobber the freshly-reset liveness state of the NEW child,
	// and the old loop can't read p.stdout while respawn reassigns it (M560).
	// Each readLoop closes the channel it was started with, not the field, so a
	// respawn that replaces the field never makes a loop close the wrong one.
	// Replaced on each (re)spawn; read under mu.
	readDone chan struct{}

	// pending tracks in-flight requests by id → response channel.
	// Map access holds mu. The read loop's terminal send is done UNDER
	// mu and non-blocking (M179) so it can't race a teardown that
	// closes the channel; the buffer (cap 1, single-use) guarantees the
	// one legitimate response never blocks. The caller's receive is on
	// its own channel and never holds mu.
	pending map[string]chan *Response

	// progress tracks per-request callbacks for streaming
	// notifications (M1.ss). Populated by InvokeWithProgress;
	// cleared in the same defer that clears `pending`. nil entry
	// or missing key both mean "drop the progress line silently"
	// — keeps the protocol forward-compatible with plugins that
	// emit progress against hosts that don't consume it.
	progress map[string]func(string)

	// nextID is a monotonic counter used to mint correlation ids.
	nextID atomic.Int64

	// cbSem bounds concurrent plugin→host callbacks (M181). A buffered
	// channel used as a counting semaphore: the read loop acquires a
	// slot (non-blocking) before spawning handleCallback and the
	// goroutine releases it on exit; a full channel means the cap is hit
	// and the callback is rejected. Created once in Spawn and persists
	// across Reload, so it bounds the plugin's whole lifetime.
	cbSem chan struct{}

	// tools is the snapshot returned by the most recent initialize.
	tools []ToolDef

	// dead is set when the read loop sees EOF or a fatal error.
	// All subsequent operations fail fast with errors that name
	// the cause (rather than hanging).
	//
	// deathErr is the cause, written by the read-loop goroutine
	// (markDead) / Close and read by callers — so it MUST be accessed
	// atomically, not as a plain field (M178). The `dead` flag alone
	// does not publish a separate plain-error field under Go's memory
	// model; an atomic.Pointer makes the cause's publication safe.
	// Access via deathError(); store via setDeathErr().
	dead     atomic.Bool
	deathErr atomic.Pointer[error]
}

// deathError returns the recorded cause of the plugin's death, or nil
// if it has not been set. Safe to call from any goroutine (M178).
func (r *remoteTool) Definition() agent.ToolDef { return r.def }

func (r *remoteTool) Invoke(ctx context.Context, raw json.RawMessage) (agent.Result, error) {
	if !r.plugin.IsAlive() {
		return agent.Result{}, fmt.Errorf("plugin: tool %q unavailable (plugin process is dead: %v)",
			r.def.Name, r.plugin.deathError())
	}
	res, err := r.plugin.Invoke(ctx, r.remoteName, raw)
	if err != nil {
		return agent.Result{}, err
	}
	return agent.Result{
		Output:  res.Output,
		IsError: res.IsError,
	}, nil
}

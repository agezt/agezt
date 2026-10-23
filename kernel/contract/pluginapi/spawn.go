// SPDX-License-Identifier: MIT

package pluginapi

import (
	"context"
	"time"

	"github.com/agezt/agezt/kernel/contract/toolapi"
)

// SpawnRequest is what a caller asks the host to spawn: one out-of-process
// plugin, with the policy knobs the operator configured.
//
// It is the plugin half of the host's spawn configuration — the host's own
// configuration row carries more (the daemon's callback wiring), so the row
// is a REQUEST, not the whole configuration.
type SpawnRequest struct {
	// Path to the plugin executable. Required.
	Path string
	// Args are passed to the executable after Path.
	Args []string
	// Env is the child's environment. Nil inherits the parent's.
	Env []string
	// Dir is the child's working directory. Empty inherits.
	Dir string
	// InitTimeout bounds the initialize handshake.
	InitTimeout time.Duration
	// InvokeTimeout bounds a single tool invocation.
	InvokeTimeout time.Duration
	// MaxFrameBytes caps one newline-delimited stdout frame from the
	// plugin — a frame larger than this tears the plugin down rather than
	// letting an untrusted child drive the host to OOM.
	MaxFrameBytes int
	// MaxConcurrentCallbacks caps simultaneous plugin→host callbacks.
	MaxConcurrentCallbacks int
	// MaxAdvertisedTools caps how many tools a plugin may advertise.
	MaxAdvertisedTools int
	// Logger receives stderr from the child (one line per call).
	// Nil discards.
	Logger func(line string)
	// PinnedHash, when non-empty, is the expected BLAKE3-256 digest of the
	// plugin binary as a 64-char lowercase hex string. The host refuses to
	// start the child if the binary does not match it.
	PinnedHash string
	// AllowedTools, when non-empty, restricts which of the plugin's
	// advertised tools the host will surface. Empty disables the check.
	AllowedTools []string
	// HostTools is the set of in-host tools the plugin may call back into.
	// Nil or empty disables callbacks entirely.
	HostTools map[string]toolapi.Tool
}

// Spawned is a running plugin process.
//
// The port exposes only what a caller needs to register the plugin's tools
// and to shut it down: the tool definitions, the declared capability per
// tool, and the close. The host's own verbs (invoke, reload, liveness) stay
// with the host.
type Spawned interface {
	// Tools returns the plugin's tool definitions, with the optional prefix
	// prepended to each name.
	Tools(prefix string) map[string]toolapi.Tool
	// ToolCapabilities returns the declared capability per prefixed tool
	// name — only the tools whose manifest carried a non-empty capability.
	ToolCapabilities(prefix string) map[string]string
	// Close shuts the plugin down. Idempotent.
	Close() error
}

// Spawner launches an out-of-process plugin.
//
// The host implements it; the plugin side calls it. A plugin may not name
// the host's module, so the spawn goes through this port and the daemon
// hands the implementation over.
type Spawner interface {
	// Spawn launches the plugin process, completes the initialize
	// handshake and returns the ready plugin.
	Spawn(ctx context.Context, req SpawnRequest) (Spawned, error)
}

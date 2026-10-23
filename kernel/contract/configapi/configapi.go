// SPDX-License-Identifier: MIT

// Package configapi is the contract the config tool and the implementation
// that answers it agree on.
//
// The `config` tool is the agent's window into the Config Center: it reads
// and writes the settings the daemon, the `agt config` CLI and the HTTP
// routes share, and it can write a setting two ways — **live** (rebuilding
// the provider in place) or **agent-scoped** (on one agent's profile). Those
// two live paths need the running daemon: the kernel that owns the roster
// and the provider. The tool is a PLUGIN, and a plugin may only reach the
// contract and the platform layers (archcheck's plugin rule) — so the port
// through which the live daemon answers the tool has to live here.
//
// The implementation does NOT live here: answering the port needs the live
// kernel, and the app layer is the only layer that may see it.
//
// The rules of the road:
//
//   - Ports and the rows they carry only. No store, no kernel, no I/O.
//   - Pure. It may import the standard library and other contract packages
//     only (archcheck's principle P4).
package configapi

import (
	"context"

	"github.com/agezt/agezt/kernel/contract/agentapi"
)

// Kernel is the narrow slice of the live daemon the config tool needs: the
// two write paths that a setting may take, and the run's own overrides.
//
// A port, not the kernel itself: the tool is a plugin, and a plugin may not
// hold the daemon's implementation. The daemon hands this port over at the
// composition root, and a daemon with no kernel hands over a **nil** port —
// never a port wrapping a nil implementation.
type Kernel interface {
	// UpdateProfile edits the mutable fields of the agent named by ref —
	// the agent-scoped write path. It answers false and no error for an
	// unknown ref (the standing pattern).
	UpdateProfile(ref string, mutate func(*agentapi.Profile)) (agentapi.Profile, bool, error)
	// Reload rebuilds the provider in place — the live-apply path. It
	// answers the error only: the catalogue the reload produced is the
	// daemon's business, not the tool's, and the tool may not name it.
	Reload() error
	// AgentConfigOverrides returns the config overrides the run in this
	// context carries. The map is a copy and safe for the caller to
	// mutate; nil means the run carries no agent-specific overrides.
	AgentConfigOverrides(ctx context.Context) map[string]string
}

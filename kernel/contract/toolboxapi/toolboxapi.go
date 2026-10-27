// SPDX-License-Identifier: MIT

// Package toolboxapi is the contract the host-toolbox module and everyone who
// asks "which command-line tools does this machine have?" agree on.
//
// The toolbox answers three operator questions: what is installed, what can be
// upgraded and how do I install a missing tool. The detection and the install
// run at the HOST level (a package manager must be able to change the system),
// so the work belongs to a module — but the rows it answers with and the port
// it answers through belong here, for the same reason the board's row does
// (W3.59): the control plane may not name the module, and an operation's
// schema may not be a module's implementation detail.
//
// The rules of the road:
//
//   - Rows only. No store, no I/O.
//   - Pure. It may import the standard library and other contract packages
//     only (archcheck's principle P4).
//
// The toolbox module re-exports the rows under its own name, so the module,
// the daemon and the operations all speak the same type.
package toolboxapi

import (
	"context"
	"errors"
)

// ToolStatus is the per-tool detection result for the wire.
type ToolStatus struct {
	Name        string `json:"name"`
	Category    string `json:"category"`
	Description string `json:"description"`
	Installed   bool   `json:"installed"`
	Version     string `json:"version,omitempty"`
	Path        string `json:"path,omitempty"`
	Installable bool   `json:"installable"`
	Manager     string `json:"manager,omitempty"` // the manager that would install it
	Command     string `json:"command,omitempty"` // the install command, shown in the UI
}

// Inventory is the full detection snapshot.
type Inventory struct {
	OS             string       `json:"os"`
	Managers       []string     `json:"managers"`
	Tools          []ToolStatus `json:"tools"`
	InstalledCount int          `json:"installed_count"`
	MissingCount   int          `json:"missing_count"`
}

// InstallResult is one tool's install outcome (streamed per-tool by the caller).
type InstallResult struct {
	Tool       string `json:"tool"`
	OK         bool   `json:"ok"`
	Skipped    bool   `json:"skipped,omitempty"` // not installable on this host
	Manager    string `json:"manager,omitempty"`
	Command    string `json:"command,omitempty"`
	Version    string `json:"version,omitempty"` // version after a successful install
	OutputTail string `json:"output_tail,omitempty"`
	Error      string `json:"error,omitempty"`
}

// ErrUnavailable is what a daemon without a toolbox host owes its callers: the
// operations answer with it instead of pretending the machine was inspected.
//
// A nil port is a daemon with no toolbox — NOT a daemon whose toolbox found
// nothing. The two must not look alike, or "the daemon cannot install tools"
// would be indistinguishable from "this machine has no tools".
var ErrUnavailable = errors.New("toolbox is not available on this daemon")

// Host is the toolbox port: the surface the operations drive. It is a port and
// not the module's functions because the control plane is an ADAPTER, and an
// adapter reaches the app layer's operations, never a module (archcheck's
// principle P2). The toolbox module's *toolbox.Host implements it.
type Host interface {
	// Detect probes every catalog tool and returns the host's snapshot.
	Detect(context.Context) Inventory
	// Outdated returns the set of catalog tool names that appear upgradable.
	Outdated(context.Context) map[string]bool
	// Install runs the resolved package-manager command for one named tool.
	Install(context.Context, string) InstallResult
}

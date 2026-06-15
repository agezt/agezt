// SPDX-License-Identifier: MIT

// Package brand centralizes all user-visible name, path, and string constants
// in one place so the project name can be changed in a single edit
// (DECISIONS A1).
//
// Nothing else in the codebase MAY hardcode the project name, binary names,
// env prefix, or config dir. Always reference these constants.
package brand

import "runtime/debug"

// Frozen identity (DECISIONS A1).
const (
	// Name is the user-visible project name.
	Name = "Agezt"

	// Binary is the kernel/daemon binary name.
	Binary = "agezt"

	// CLI is the command-line client binary name.
	CLI = "agt"

	// EnvPrefix prefixes every environment variable Agezt reads.
	EnvPrefix = "AGEZT_"

	// ConfigDir is the per-user config directory name under $HOME.
	ConfigDir = ".agezt"

	// ProtocolVersion is the plugin/kernel contract major version
	// (DECISIONS B1). Plugins built for protocol N run with any kernel of
	// protocol N. Append-only fields; enums never renumbered.
	ProtocolVersion = 1
)

// Version is the kernel semver, updated per release. v1.0.0 is the Scale
// release (ROADMAP §3, M8): "One Agezt across many nodes" — the v0.1.0 MVP
// plus a federated mesh (peer discovery, capability-aware routing, failover,
// bounded delegation) and multi-tenant isolation, fused so each tenant
// federates to its own peer set. See CHANGELOG.md.
var Version = "1.0.0"

// BuildInfo returns the VCS revision, commit time, and dirty flag that `go
// build` embeds automatically when building from a git checkout (no ldflags
// needed). It lets operators confirm exactly which build a daemon is running —
// the semver above is bumped only per release, so it can't distinguish two
// dev builds. Empty revision means the binary was built without VCS info
// (e.g. `go build -buildvcs=false` or from a tarball).
func BuildInfo() (revision, committed string, modified bool) {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		return "", "", false
	}
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			revision = s.Value
		case "vcs.time":
			committed = s.Value
		case "vcs.modified":
			modified = s.Value == "true"
		}
	}
	return revision, committed, modified
}

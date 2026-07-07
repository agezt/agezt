// SPDX-License-Identifier: MIT

// Package brand centralizes all user-visible name, path, and string constants
// in one place so the project name can be changed in a single edit
// (DECISIONS A1).
//
// Nothing else in the codebase MAY hardcode the project name, binary names,
// env prefix, or config dir. Always reference these constants.
package brand

import "runtime/debug"

// debugReadBuildInfo is an indirection point for testing —
// tests can replace it to simulate ReadBuildInfo failure.
var debugReadBuildInfo = debug.ReadBuildInfo

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
//
// Overridable at build time via:
//
//	go build -ldflags '-X github.com/agezt/agezt/internal/brand.Version=1.2.3'
//
// The Makefile and scripts/build.sh do this automatically; the default
// below is used only when a developer runs `go build` without ldflags.
var Version = "1.0.0"

// BuildCommit is the short git SHA the binary was built from. Empty when
// not stamped — operators can detect "this build wasn't from CI" via
// BuildInfo() which surfaces the empty value alongside the runtime VCS
// info (debug.ReadBuildInfo). Stamped via:
//
//	go build -ldflags '-X github.com/agezt/agezt/internal/brand.BuildCommit=$(git rev-parse --short HEAD)'
var BuildCommit = ""

// BuildTime is the RFC3339 UTC timestamp of the build. Empty when not
// stamped; callers should fall back to debug.ReadBuildInfo().Stamped via:
//
//	go build -ldflags '-X github.com/agezt/agezt/internal/brand.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)'
var BuildTime = ""

// BuildInfo returns the VCS revision, commit time, and dirty flag that `go
// build` embeds automatically when building from a git checkout (no ldflags
// needed). It lets operators confirm exactly which build a daemon is running —
// the semver above is bumped only per release, so it can't distinguish two
// dev builds. Empty revision means the binary was built without VCS info
// (e.g. `go build -buildvcs=false` or from a tarball).
//
// BuildCommit and BuildTime are referenced here so that an `-X` ldflag
// stamp of those package-level vars actually reaches the binary: Go's
// linker applies dead-code elimination and an unreferenced `var` is
// stripped before the data-section patch has anything to overwrite.
// Calling these at runtime from any path that lives in `main` is enough
// to keep them in the data section.
func BuildInfo() (revision, committed string, modified bool) {
	bi, ok := debugReadBuildInfo()
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

// BuildStamp returns the three identity values stamped by the Makefile /
// scripts/build.sh ldflag injection: the version, the short git commit,
// and the RFC3339 UTC build timestamp. BuildCommit / BuildTime are
// empty when the build wasn't stamped (e.g. `go build` without ldflags
// or a tarball build) — callers should pair this with BuildInfo() to
// disambiguate "unstamped dev build" from "stamped dev build".
func BuildStamp() (version, commit, buildTime string) {
	return Version, BuildCommit, BuildTime
}

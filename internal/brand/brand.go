// SPDX-License-Identifier: MIT

// Package brand centralizes all user-visible name, path, and string constants
// in one place so the project name can be changed in a single edit
// (DECISIONS A1).
//
// Nothing else in the codebase MAY hardcode the project name, binary names,
// env prefix, or config dir. Always reference these constants.
package brand

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

// Version is the kernel semver, updated per release. v0.1.0 is the MVP
// (ROADMAP §2.2): a usable Jarvis — kernel + journal + agent loop + real
// providers + sandboxed tools + Telegram + Pulse + memory/world/skills/
// reflection + Web UI, all journaled and reversible. See CHANGELOG.md.
var Version = "0.1.0"

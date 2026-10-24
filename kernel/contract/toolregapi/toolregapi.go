// SPDX-License-Identifier: MIT

// Package toolregapi is the tool registry's contract: the rows every
// first-party tool declares itself with, and the registry that holds them.
//
// THE REGISTRY. Each built-in tool contributes a Spec — how to build it from
// boot-time deps, plus optional lifecycle hooks for the phases the daemon
// used to hand-wire with string-keyed downcasts in cmd/agezt/main.go:
//
//   - Build       pre-kernel construction (env-configured, may skip itself)
//   - PreOpen     mutate the daemon's configuration before the kernel opens
//   - Configure   post-Open injection (kernel, bus, artifact index, …)
//   - Late        wiring that needs live channels / the board
//
// Specs register into a package-level registry (mirroring builtinchannels);
// the daemon calls BuildAll once and drives the resulting Set through the
// phases. Netguard-capable tools implement NetguardAware and are wired
// generically by Configure — no hand-listed type switch — and NetguardGaps
// lets a test enumerate, FROM the registry, any Netguard spec whose built
// instances cannot receive the audit callback.
//
// The registry drives every first-party tool through four phases: build
// (pre-kernel), pre-Open (mutate the daemon's configuration before the
// kernel opens), configure (post-Open injection) and late (live channels and
// the board). A tool that takes part in a phase answers that phase's PORT —
// it may not name the daemon's runtime, which lives in a module.
//
// WHY IT LIVES HERE. The registry moved down to the contract layer in
// W3.39h: every row it carries is a contract row or a contract port, and the
// built-in tools are a PLUGIN — a plugin may name the contract layer and
// nothing else. It used to live in kernel/toolreg (L3), which forced the last
// `plugin-reach` edge — plugins/builtintools -> kernel/toolreg — into the
// allowlist.
//
// The rules of the road:
//   - Ports and the rows that belong to the port only.
//   - Pure. It may import the standard library and other contract packages
//     only (archcheck's principle P4).
package toolregapi

import "context"

// ScriptRunner executes a forged script tool: the code the script-tool forge
// (M794) produced, in the language it declared.
//
// The registry's contract declares its OWN runner — the daemon's script
// runner type lives in a module, and a contract may not name a module. The
// method set is the same, so the daemon's runner satisfies this one.
type ScriptRunner interface {
	// RunScript runs code and returns its output, whether that output is an
	// error verdict (non-zero exit, timeout, unavailable language) and the
	// transport error, if any.
	RunScript(ctx context.Context, language, code, inputJSON string) (output string, isError bool, err error)
}

// Config is the live daemon configuration the pre-Open phase may set up.
//
// The pre-Open phase runs after the tools are built but BEFORE the kernel
// opens, so it is the last moment at which a tool's own instance can be
// wired into the daemon's configuration. Today exactly one thing is wired
// this way: the script-tool forge executes forged tools through the
// code-execution tool's own runner (M794), and that runner must be in place
// before the kernel opens.
type Config interface {
	// SetScriptRunner installs the runner that executes forged script tools
	// (M794). It must be called before the kernel opens.
	SetScriptRunner(r ScriptRunner)
}

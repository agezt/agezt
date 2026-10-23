// SPDX-License-Identifier: MIT

// Package pluginapi is the contract for out-of-process plugins.
//
// A plugin is a child process speaking line-delimited JSON on stdio. The
// HOST — the daemon — owns the process: it spawns it, pins its binary,
// restricts the tools it may advertise, bridges its tool calls into the
// tool registry and tears it down.
//
// The plugin side — the built-in tools that host out-of-process plugins —
// may not name the host's module, so the things it needs from the host
// answer through this contract:
//
//   - The SPEC ROWS: the operator's plugin declarations. `AGEZT_PLUGINS`
//     (the plugins and their prefixes), `AGEZT_PLUGIN_PINS` (the BLAKE3
//     digests their binaries must match) and `AGEZT_PLUGIN_TOOLS` (the
//     tools each plugin may advertise). They are pure rows with pure
//     parsers, so they live here — one row, one spelling of it.
//
//   - The SPAWN PORT: launching the child process. That is the host's
//     implementation, so the plugin asks for it through the port and the
//     daemon hands the implementation over.
//
// The rules of the road:
//   - Ports and the rows that belong to the port only.
//   - Pure. This package may import the standard library and other contract
//     packages only (archcheck's principle P4) — the spawn request's tool
//     map is typed with the contract's own tool vocabulary for exactly that
//     reason.
package pluginapi

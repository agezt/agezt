// SPDX-License-Identifier: MIT

// Package acp implements an Agent Client Protocol server (SPEC-15 §3): Agezt
// as an agent backend that IDEs (Zed, and other ACP clients) drive over
// JSON-RPC 2.0 on stdio. The editor spawns the agent process, calls
// `initialize` → `session/new` → `session/prompt`, and receives streamed
// `session/update` notifications while the prompt runs.
//
// The protocol handling here is transport- and kernel-agnostic: it reads/writes
// JSON-RPC over any io.Reader/io.Writer and delegates the actual work to a
// Runner. The `agt acp` command wires a Runner backed by the control-plane
// client, so an ACP prompt runs through the same kernel tool-loop + Edict +
// journal as `agt run` — an editor driving Agezt does not bypass governance
// (SPEC-15 §3.3).
package acp

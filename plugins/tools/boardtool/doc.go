// SPDX-License-Identifier: MIT

// Package boardtool is the agent-facing wrapper over kernel/board: the shared,
// persistent, topic-addressed message board every agent — the lead, its
// sub-agents, scheduled and standing-order agents, and the continuous loops —
// can post to and read from, so they can coordinate and talk to each other
// (M647).
//
// The store itself lives in kernel/board (so the control plane can read it to
// surface the conversation in the Web UI without importing a plugin); this
// package is just the Tool that lets an agent use it, mirroring how the
// schedule/standing tools wrap kernel/cadence and kernel/standing.
//
// The tool is a PLUGIN, so it may reach the contract and the platform layers
// and nothing else: it names the board's rows where they live
// (kernel/contract/boardapi) and takes the store as the contract's PORT
// (boardapi.Store). The daemon opens the one shared store and hands it over —
// the plugin neither names the module nor opens a store of its own.
package boardtool

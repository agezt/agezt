// SPDX-License-Identifier: MIT

// Package peer is the mesh delegation tool (ROADMAP P6-MULTI / M8): it lets one
// Agezt node hand a self-contained task to a *peer* Agezt node and get the
// answer back, by driving the peer's native REST surface
// (POST /api/v1/runs, kernel/restapi). This composes the REST API into a
// node-to-node primitive — cooperating Jarvis nodes, each governing its own runs.
//
// Peers are operator-configured (AGEZT_PEERS); the local agent only names which
// peer and what task. Because the call ships a task to an external node (an
// outward, side-effecting action), it is gated Ask-first by Edict
// (remote_run capability). The peer runs the task through its own governed loop
// — delegation does not bypass the peer's Edict/journal, and the returned
// correlation id makes the remote run auditable on that node.
package peer

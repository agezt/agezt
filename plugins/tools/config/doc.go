// SPDX-License-Identifier: MIT

// Package config is the in-process `config` agent tool: it lets the agent (and
// the skills it runs) read, write, and register Config Center settings directly,
// without shelling out. It is the tool half of the Config Center's skill-facing
// surface (the other halves are the `agt config` CLI and the /api/config/* HTTP
// routes). All three go through the same kernel/settings Registry + creds vault,
// so behaviour — namespacing, secret handling, live-vs-restart — is identical.
package config

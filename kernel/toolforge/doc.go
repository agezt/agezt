// SPDX-License-Identifier: MIT

// Package toolforge is the script-tool forge (M794): agent-authored code
// promoted into durable, callable tools — the close of the write→use→improve
// cycle. An agent (or operator) DRAFTS a named script, TESTS it in the
// code-exec sandbox, and once a test of the current code has passed the
// OPERATOR promotes it; from then on every run is offered the script as a
// real tool named `forge_<name>`, executed through the same warden-isolated,
// secret-scrubbed sandbox as `code_exec` and gated by the same `code.exec`
// Edict capability. Quarantine is the instant kill switch, and ANY edit to
// the code demotes the tool back to draft with its test record cleared —
// only tested code is ever live.
//
// Storage mirrors kernel/roster: a single JSON file rewritten atomically on
// change, safe for concurrent use; every lifecycle mutation is journaled by
// the kernel (scripttool.*) so `agt why` can explain how a tool came to be.
package toolforge

// SPDX-License-Identifier: MIT

// Env-gated / workspace-scoped specs (Phase 2.2 PR 6): the last inline blocks
// of cmd/agezt's buildTools — the always-on workspace pair (shell, file) and
// the operator-opt-in externals (coding, acp_agent, homeassistant,
// remote_run). Construction reads the environment through BuildDeps.Get; a
// malformed operator spec (remote_run's peer lists, file's checkpoint init)
// returns a Build error = hard boot failure, exactly as before.
package builtintools

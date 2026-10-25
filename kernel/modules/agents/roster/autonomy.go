// SPDX-License-Identifier: MIT

package roster

// Provenance: the autonomy runbook — the machine-readable wake contract for a
//             profile. The IMPLEMENTATION moved to the contract layer
//             (kernel/contract/agentapi) during the W3.56 slice: the control
//             plane's wake evidence, the run executor's sub-agent prep and the
//             daemon's cadence all build the SAME payload, and they may not
//             name this module to reach it.
//
//             The name stays here as a delegation, so every existing caller is
//             unchanged.

import "github.com/agezt/agezt/kernel/contract/agentapi"

// AutonomyRunbook builds the machine-readable wake contract for a profile: the
// trigger/route/recovery/sleep behavior the agent operates under. It is the one
// canonical builder shared by every wake-evidence emitter so manual operator
// wakes, schedule fires, standing fires (incl. mailbox), and delegated sub-agent
// spawns all carry an identically-shaped `autonomy_runbook` payload through the
// journal, which the control plane folds into agent status as
// `last_autonomy_runbook`. Keep this the single source of truth — do not
// re-derive the contract shape elsewhere.
//
// The rule itself lives in the contract layer (kernel/contract/agentapi): the
// rule judges the contract's row, so it lives with the row.
func AutonomyRunbook(p Profile) map[string]any {
	return agentapi.AutonomyRunbook(p)
}

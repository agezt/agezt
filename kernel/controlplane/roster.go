// SPDX-License-Identifier: MIT

package controlplane

// Agent roster CRUD handlers (M783) — the management path behind `agt agent`.
// Lifecycle changes go through the kernel so every create/edit/pause/resume/
// remove is journaled (roster.*) and auditable via `agt why`. Profiles are
// addressed by ref = id OR slug everywhere, so operators can say
// `agt agent show researcher` without copying ULIDs.

import (
	approster "github.com/agezt/agezt/kernel/app/roster"
)

type agentRepairRow = approster.RepairRow

type agentRepairSummary = approster.RepairSummary

type agentRoutingPressure = approster.RoutingPressure

type agentRetryPressure = approster.RetryPressure

type agentEscalationLoad = approster.EscalationLoad

type agentWakeStatus = approster.WakeStatus

type agentLiveStatus = approster.LiveStatus

type agentLastActivity = approster.LastActivity

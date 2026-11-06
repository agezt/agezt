// SPDX-License-Identifier: MIT

package runtime

import (
	"github.com/agezt/agezt/kernel/modules/runs/api"
)

// The steering OPERATIONS moved to the runs module in W3.76b: the target
// architecture gives the runs module the "steering/interventions", so the
// service — pause, resume, step, steer, observe, intervene — lives in
// kernel/modules/runs/steer, and the per-run steering surface (the Control)
// lives there too.
//
// W3.76z took the daemon's NAMES away as well. The daemon held six verbs over
// the module's own operations (PauseRun, ResumeRun, StepRun, SteerRun,
// RunControlState, InterveneRun) — a second spelling of one family, because
// the module's service already answered in its own names. The daemon hands
// over the module's FACE now, and the application layer binds that.
//
// The daemon keeps the DOORS: the live runs the module steers (the live-run
// state's own registry, W3.76ae), the bus it publishes on, and the cancel it
// asks for. The module never names the daemon.

// SteeringService returns the RUNS MODULE's steering service — the runs
// module's face.
//
// The daemon holds no verb per steering operation any more (W3.76z): the
// module keeps the steering and the interventions, and the daemon hands over
// the module's FACE. The verbs are the MODULE's names (Pause, Resume, Step,
// Steer, State, Intervene), so one operation has one name.
//
// The face is never absent: the daemon builds the service while opening
// itself (it hands the module its own doors), so a caller never has to guard
// against nothing.
func (k *Kernel) SteeringService() api.SteeringService { return k.steerSvc }

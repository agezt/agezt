// SPDX-License-Identifier: MIT

package controlplane

// The operator action bus and the launch adapters: the one publisher the
// control plane journals operator actions through, and the two thin adapters
// that bind the host's run and repair capabilities to the application layer's
// launchers (`kernel/app/roster/launch.go`). The launch DECISIONS — does the
// run retry, what does the terminal event say, how is a panic contained —
// live in the application layer; what stays here is the host's data access:
// running on the kernel's bus and driving the governed repair.

import (
	"context"
	"fmt"
	"os"

	appeventbus "github.com/agezt/agezt/kernel/app/eventbus"
	approster "github.com/agezt/agezt/kernel/app/roster"
	appruns "github.com/agezt/agezt/kernel/app/runs"
	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/runtime"
)

func publishOperatorAction(k *runtime.Kernel, subject, corr string, payload map[string]any) {
	ports := appeventbus.PortsFromKernel(k)
	if !ports.Ready() {
		return
	}
	_, _ = ports.Publish(event.Spec{
		Subject:       subject,
		Kind:          event.KindInfo,
		Actor:         "controlplane",
		CorrelationID: corr,
		Payload:       payload,
	})
}

// launchOperatorWake hands an operator wake to the application layer's
// launcher over the host's run and publish ports. The application decides
// whether the run retries and what cost ceiling it runs under; the host
// decorates the run context and executes it.
func (s *Server) launchOperatorWake(corr string, p agentapi.Profile, intent, reason string, lineage approster.IncidentLineage) {
	approster.LaunchOperatorWake(approster.WakeLaunchPorts{
		Run: func(corr, intent string, retry *agentapi.RetryPolicy, maxCostMc int64) (string, error) {
			ctx := appruns.Decorate(context.Background(), p, agentapi.WakeContext{Source: "operator", Reason: reason}, maxCostMc)
			if retry != nil {
				return s.k.RunWithRetry(ctx, corr, intent, *retry)
			}
			return s.k.RunWith(ctx, corr, intent)
		},
		Publish: func(subject, corr string, payload map[string]any) {
			publishOperatorAction(s.k, subject, corr, payload)
		},
	}, p, corr, intent, reason, lineage)
}

// launchOperatorRepair hands an operator repair to the application layer's
// launcher over the host's governed-repair and publish ports. The launcher's
// panic firewall is the application's; the host only reports the panic it
// caught.
func (s *Server) launchOperatorRepair(corr string, p agentapi.Profile, reason string, lineage approster.IncidentLineage) {
	approster.LaunchOperatorRepair(approster.RepairLaunchPorts{
		Repair: func(slug, reason string) (approster.RepairOutcome, error) {
			if s.repairSource == nil {
				return approster.RepairOutcome{}, fmt.Errorf("agent repair is not available on this daemon")
			}
			res, err := s.repairSource.RepairAgent(slug, reason)
			if err != nil {
				return approster.RepairOutcome{}, err
			}
			return approster.RepairOutcome{
				Applied:                       res.Applied,
				RoutingTaskType:               res.RoutingTaskType,
				RoutingTaskModelChain:         res.RoutingTaskModelChain,
				PreviousRoutingTaskModelChain: res.PreviousRoutingTaskModelChain,
				Answer:                        res.Answer,
			}, nil
		},
		Publish: func(subject, corr string, payload map[string]any) {
			publishOperatorAction(s.k, subject, corr, payload)
		},
		OnPanic: func(slug string, r any) {
			fmt.Fprintf(os.Stderr, "operator repair of %q panicked: %v\n", slug, r)
		},
	}, p, corr, reason, lineage)
}

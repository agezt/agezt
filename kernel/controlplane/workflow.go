// SPDX-License-Identifier: MIT
package controlplane

import (
	"context"
	"fmt"
	appworkflow "github.com/agezt/agezt/kernel/app/workflow"
	"github.com/agezt/agezt/kernel/event"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/workflow"
	"os"
)

func (s *Server) workflowReads() *appworkflow.Reads {
	return appworkflow.NewReads(s.k.Workflows(), s.k.Journal(), nil)
}

func (s *Server) workflowLifecycle() *appworkflow.Lifecycle { return appworkflow.NewLifecycle(s.k) }

func (s *Server) workflowCopilot() *appworkflow.Copilot {
	return appworkflow.NewCopilot(s.k.Workflows(), s.k, s.k.NewCorrelation)
}

func (s *Server) workflowExecution() *appworkflow.Execution {
	return appworkflow.NewExecution(s.k.Workflows(), appworkflow.ExecutionHost{
		Correlation: s.k.NewCorrelation,
		Run: func(ctx context.Context, corr, ref string, payload any) (appworkflow.RunResult, error) {
			res, err := s.k.RunWorkflow(ctx, corr, ref, payload)
			return appworkflow.RunResult{Executed: res.Executed, Outputs: res.Outputs}, err
		},
		TestNode: func(ctx context.Context, corr string, w workflow.Workflow, node string, data map[string]any, payload any) (appworkflow.NodeResult, error) {
			res, err := s.k.TestWorkflowNode(ctx, corr, w, node, data, payload)
			return appworkflow.NodeResult{Output: res.Output, Port: res.Port, Attempts: res.Attempts}, err
		},
		WithWake: func(ctx context.Context, wake appworkflow.Wake) context.Context {
			return kernelruntime.WithWakeContext(ctx, kernelruntime.WakeContext{Source: wake.Source, Reason: wake.Reason, ScheduleID: wake.ScheduleID, StandingID: wake.StandingID, StandingName: wake.StandingName, TriggerSubject: wake.TriggerSubject, ParentCorrelation: wake.ParentCorrelation})
		},
		Panic: func(wake appworkflow.Wake, corr, name string, value any) {
			fmt.Fprintf(os.Stderr, "workflow %q (%s) panicked: %v\n", name, wake.Source, value)
			if s.k == nil || s.k.Bus() == nil {
				return
			}
			_, _ = s.k.Bus().Publish(event.Spec{Subject: "workflow." + name, Kind: event.KindWorkflowPanic, Actor: "controlplane", CorrelationID: corr, Payload: map[string]any{"workflow": name, "source": wake.Source, "panic": fmt.Sprintf("%v", value)}})
		},
	})
}

// SPDX-License-Identifier: MIT
package controlplane

import (
	"context"
	"fmt"
	appworkflow "github.com/agezt/agezt/kernel/app/workflow"
	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/event"
	"os"
)

func (s *Server) workflowReads() *appworkflow.Reads {
	ports := s.workflowPorts()
	return appworkflow.NewReads(ports.Reader, ports.Events, nil)
}

func (s *Server) workflowLifecycle() *appworkflow.Lifecycle {
	return appworkflow.NewLifecycle(s.workflowPorts().Writer)
}

func (s *Server) workflowCopilot() *appworkflow.Copilot {
	ports := s.workflowPorts()
	return appworkflow.NewCopilot(ports.Reader, ports.Designer, ports.Correlation)
}

func (s *Server) workflowExecution() *appworkflow.Execution {
	ports := s.workflowPorts()
	return appworkflow.NewExecution(ports.Reader, appworkflow.ExecutionHost{
		Correlation: ports.Correlation,
		Run:         ports.Run,
		TestNode:    ports.TestNode,
		WithWake: func(ctx context.Context, wake appworkflow.Wake) context.Context {
			return agentapi.WithWakeContext(ctx, agentapi.WakeContext{Source: wake.Source, Reason: wake.Reason, ScheduleID: wake.ScheduleID, StandingID: wake.StandingID, StandingName: wake.StandingName, TriggerSubject: wake.TriggerSubject, ParentCorrelation: wake.ParentCorrelation})
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

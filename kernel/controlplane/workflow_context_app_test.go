// SPDX-License-Identifier: MIT
package controlplane

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/workflow"
	"github.com/agezt/agezt/plugins/providers/mock"
	"strings"
	"testing"
	"time"
)

func TestWorkflowAllNineAppMutationsJoinOwnedAuditAndDomainIdentity(t *testing.T) {
	for _, cmd := range []string{CmdWorkflowSave, CmdWorkflowRestore, CmdWorkflowRemove, CmdWorkflowSetEnabled, CmdWorkflowRun, CmdWorkflowDraft, CmdWorkflowRefine, CmdWorkflowWebhook, CmdWorkflowTestNode} {
		t.Run(cmd, func(t *testing.T) {
			dir := t.TempDir()
			provider := mock.New(mock.FinalText(`{"name":"designed","nodes":[{"id":"start","type":"trigger"}]}`))
			k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: provider})
			if err != nil {
				t.Fatal(err)
			}
			defer k.Close()
			config := json.RawMessage(`{}`)
			if cmd == CmdWorkflowWebhook {
				config = json.RawMessage(`{"kind":"webhook","secret":"owned fixture credential"}`)
			}
			graph, _, err := k.SaveWorkflow("", workflow.Workflow{Name: "owned", Nodes: []workflow.Node{{ID: "start", Type: workflow.NodeTrigger, Config: config}, {ID: "work", Type: workflow.NodeTransform, Config: json.RawMessage(`{"template":"{{trigger.payload}}"}`)}}, Edges: []workflow.Edge{{From: "start", To: "work"}}})
			if err != nil {
				t.Fatal(err)
			}
			s := NewServer(k, dir)
			s.token = "primary"
			args := map[string]any{"ref": graph.ID, "workflow": graph, "enabled": false, "async": true, "description": "design graph", "instruction": "revise graph", "node": "work", "payload": "owned", "secret": "owned fixture credential", "corr": "spoof", "correlation_id": "spoof"}
			responses := callAppHost(t, s, Request{ID: cmd, Cmd: cmd, Token: "primary", Args: args})
			if len(responses) != 1 || responses[0].Type != RespResult {
				t.Fatal(cmd, responses)
			}
			kind := map[string]event.Kind{CmdWorkflowSave: event.KindWorkflowSaved, CmdWorkflowRestore: event.KindWorkflowRestored, CmdWorkflowRemove: event.KindWorkflowRemoved, CmdWorkflowSetEnabled: event.KindWorkflowUpdated, CmdWorkflowRun: event.KindWorkflowStarted, CmdWorkflowDraft: event.KindWorkflowDrafted, CmdWorkflowRefine: event.KindWorkflowDrafted, CmdWorkflowWebhook: event.KindWorkflowStarted, CmdWorkflowTestNode: event.KindWorkflowNode}[cmd]
			var begin, end, domain []*event.Event
			deadline := time.Now().Add(5 * time.Second)
			for {
				begin = nil
				end = nil
				domain = nil
				if err := k.Journal().Range(func(e *event.Event) error {
					if e.Subject == "op."+cmd {
						if e.Kind == event.KindOpInvoked {
							begin = append(begin, e)
						}
						if e.Kind == event.KindOpCompleted {
							end = append(end, e)
						}
					}
					if e.Kind == kind && e.CorrelationID != "" {
						domain = append(domain, e)
					}
					return nil
				}); err != nil {
					t.Fatal(err)
				}
				if len(domain) > 0 {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("domain event missing", cmd)
				}
				time.Sleep(time.Millisecond)
			}
			if len(begin) != 1 || len(end) != 1 || len(domain) != 1 || begin[0].CorrelationID == "" || begin[0].CorrelationID == "spoof" || end[0].CorrelationID != begin[0].CorrelationID || domain[0].CorrelationID != begin[0].CorrelationID {
				t.Fatal(cmd, begin, end, domain)
			}
			if corr, ok := responses[0].Result["correlation_id"]; ok && corr != begin[0].CorrelationID {
				t.Fatal(cmd, responses, begin)
			}
		})
	}
}
func TestWorkflowDispatcherInflightCancellationSettlesJoinedAuditWithoutProviderSuccess(t *testing.T) {
	dir := t.TempDir()
	provider := mock.New()
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: provider})
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	graph, _, err := k.SaveWorkflow("", workflow.Workflow{Name: "owned", Nodes: []workflow.Node{{ID: "start", Type: workflow.NodeTrigger}, {ID: "wait", Type: workflow.NodeDelay, Config: json.RawMessage(`{"seconds":60}`)}}, Edges: []workflow.Edge{{From: "start", To: "wait"}}})
	if err != nil {
		t.Fatal(err)
	}
	s := NewServer(k, dir)
	s.token = "primary"
	dispatcher, err := app.NewDispatcher(registeredAppOperations(), app.Dependencies{Auth: appAuthenticator{s}, Router: appTenantRouter{s}, Audit: appAuditor{}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	raw, _ := json.Marshal(map[string]any{"ref": graph.ID})
	go func() {
		_, err := dispatcher.Dispatch(ctx, opapi.Caller{Credential: "primary", Source: "controlplane"}, CmdWorkflowRun, raw, nil)
		done <- err
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		started := false
		if err := k.Journal().Range(func(e *event.Event) error {
			if e.Kind == event.KindWorkflowStarted {
				started = true
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if started {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("run did not begin")
		}
		time.Sleep(time.Millisecond)
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("canceled dispatcher retained60s delay")
	}
	var events []*event.Event
	if err := k.Journal().Range(func(e *event.Event) error {
		if e.Kind == event.KindOpInvoked || e.Kind == event.KindOpFailed || e.Kind == event.KindWorkflowStarted || e.Kind == event.KindWorkflowFailed {
			events = append(events, e)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(events) != 4 {
		t.Fatal(events)
	}
	corr := events[0].CorrelationID
	for _, e := range events {
		if corr == "" || e.CorrelationID != corr {
			t.Fatal(events)
		}
	}
	if provider.CallCount() != 0 {
		t.Fatal(provider.CallCount())
	}
	if !strings.Contains(events[len(events)-1].Subject, "op.workflow_run") {
		t.Fatal(events)
	}
}

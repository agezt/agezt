// SPDX-License-Identifier: MIT
package controlplane_test

import (
	"context"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/workflow"
	"github.com/agezt/agezt/plugins/providers/mock"
	"reflect"
	"strings"
	"testing"
)

func TestWorkflowPrimaryOperationsSocketTenantDenialPreservesGraphAndProvider(t *testing.T) {
	provider := mock.New()
	k, server, _, dir := startPair(t, provider)
	registry := withTenants(t, server, dir)
	token := mustTenant(t, registry, "acme")
	graph, _, err := k.SaveWorkflow("", workflow.Workflow{Name: "primary-private", Nodes: []workflow.Node{{ID: "start", Type: workflow.NodeTrigger}}})
	if err != nil {
		t.Fatal(err)
	}
	before := k.Workflows().List()
	client := tenantClient(t, dir, token)
	for _, cmd := range []string{controlplane.CmdWorkflowList, controlplane.CmdWorkflowShow, controlplane.CmdWorkflowRuns, controlplane.CmdWorkflowTemplates, controlplane.CmdWorkflowSave, controlplane.CmdWorkflowRestore, controlplane.CmdWorkflowRemove, controlplane.CmdWorkflowSetEnabled, controlplane.CmdWorkflowRun, controlplane.CmdWorkflowDraft, controlplane.CmdWorkflowRefine, controlplane.CmdWorkflowWebhook, controlplane.CmdWorkflowTestNode} {
		if _, err := client.Call(context.Background(), cmd, map[string]any{"tenant": "acme", "ref": graph.ID, "workflow": graph, "enabled": false, "async": true}); err == nil || (!strings.Contains(err.Error(), "forbidden") && !strings.Contains(err.Error(), "unauthorized")) {
			t.Fatal(cmd, err)
		}
	}
	if !reflect.DeepEqual(before, k.Workflows().List()) || provider.CallCount() != 0 {
		t.Fatal(before, k.Workflows().List(), provider.CallCount())
	}
}

// SPDX-License-Identifier: MIT
package controlplane_test

import (
	"context"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/modules/workflows/workflow"
	"github.com/agezt/agezt/plugins/providers/mock"
	"reflect"
	"strings"
	"testing"
)

func TestWorkflowPrimaryOperationsSocketTenantDenialPreservesGraphAndProvider(t *testing.T) {
	provider := mock.New()
	k, _, _, dir, registry := startPairWithTenants(t, provider)
	token := mustTenant(t, registry, "acme")
	graph, _, err := k.WorkflowEngine().Save("", workflow.Workflow{Name: "primary-private", Nodes: []workflow.Node{{ID: "start", Type: workflow.NodeTrigger}}})
	if err != nil {
		t.Fatal(err)
	}
	before := k.Workflows().List()
	client := tenantClient(t, dir, token)
	for _, cmd := range []string{opapi.WorkflowList, opapi.WorkflowShow, opapi.WorkflowRuns, opapi.WorkflowTemplates, opapi.WorkflowSave, opapi.WorkflowRestore, opapi.WorkflowRemove, opapi.WorkflowSetEnabled, opapi.WorkflowRun, opapi.WorkflowDraft, opapi.WorkflowRefine, opapi.WorkflowWebhook, opapi.WorkflowTestNode} {
		if _, err := client.Call(context.Background(), cmd, map[string]any{"tenant": "acme", "ref": graph.ID, "workflow": graph, "enabled": false, "async": true}); err == nil || (!strings.Contains(err.Error(), "forbidden") && !strings.Contains(err.Error(), "unauthorized")) {
			t.Fatal(cmd, err)
		}
	}
	if !reflect.DeepEqual(before, k.Workflows().List()) || provider.CallCount() != 0 {
		t.Fatal(before, k.Workflows().List(), provider.CallCount())
	}
}

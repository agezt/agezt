// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/workboard"
	"github.com/agezt/agezt/plugins/providers/mock"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestWorkboardOperationsSocketTenantDenialPreservesPrimary(t *testing.T) {
	primary, server, _, dir := startPair(t, mock.New())
	registry := withTenants(t, server, dir)
	token := mustTenant(t, registry, "acme")
	task, _, err := primary.Workboard().Create(workboard.CreateSpec{Title: "primary-private"}, time.UnixMilli(100))
	if err != nil {
		t.Fatal(err)
	}
	before := primary.Workboard().List(workboard.Filter{IncludeArchived: true})
	client := tenantClient(t, dir, token)
	commands := []string{controlplane.CmdWorkboardList, controlplane.CmdWorkboardLanes, controlplane.CmdWorkboardShow, controlplane.CmdWorkboardWatch, controlplane.CmdWorkboardCreate, controlplane.CmdWorkboardClaim, controlplane.CmdWorkboardHeartbeat, controlplane.CmdWorkboardComment, controlplane.CmdWorkboardBlock, controlplane.CmdWorkboardFail, controlplane.CmdWorkboardUnblock, controlplane.CmdWorkboardComplete, controlplane.CmdWorkboardProve, controlplane.CmdWorkboardSeat, controlplane.CmdWorkboardArchive, controlplane.CmdWorkboardLink, controlplane.CmdWorkboardPolicy, controlplane.CmdWorkboardDepend, controlplane.CmdWorkboardReclaim, controlplane.CmdWorkboardSweep, controlplane.CmdWorkboardDispatch}
	for _, cmd := range commands {
		if _, err := client.Call(context.Background(), cmd, map[string]any{"id": task.ID, "tenant": "acme", "title": "tenant attempt"}); err == nil || !strings.Contains(err.Error(), "forbidden") {
			t.Fatalf("tenant %s = %v", cmd, err)
		}
	}
	after := primary.Workboard().List(workboard.Filter{IncludeArchived: true})
	if !reflect.DeepEqual(before, after) {
		t.Fatal("tenant changed primary workboard")
	}
}

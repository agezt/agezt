// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/modules/work/workboard"
	"github.com/agezt/agezt/plugins/providers/mock"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestWorkboardOperationsSocketTenantDenialPreservesPrimary(t *testing.T) {
	primary, _, _, dir, registry := startPairWithTenants(t, mock.New())
	token := mustTenant(t, registry, "acme")
	task, _, err := primary.Workboard().Create(workboard.CreateSpec{Title: "primary-private"}, time.UnixMilli(100))
	if err != nil {
		t.Fatal(err)
	}
	before := primary.Workboard().List(workboard.Filter{IncludeArchived: true})
	client := tenantClient(t, dir, token)
	commands := []string{opapi.WorkboardList, opapi.WorkboardLanes, opapi.WorkboardShow, opapi.WorkboardWatch, opapi.WorkboardCreate, opapi.WorkboardClaim, opapi.WorkboardHeartbeat, opapi.WorkboardComment, opapi.WorkboardBlock, opapi.WorkboardFail, opapi.WorkboardUnblock, opapi.WorkboardComplete, opapi.WorkboardProve, opapi.WorkboardSeat, opapi.WorkboardArchive, opapi.WorkboardLink, opapi.WorkboardPolicy, opapi.WorkboardDepend, opapi.WorkboardReclaim, opapi.WorkboardSweep, opapi.WorkboardDispatch}
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

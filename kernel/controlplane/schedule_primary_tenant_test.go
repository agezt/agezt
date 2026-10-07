// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/plugins/providers/mock"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestSchedulePrimaryOperationsSocketTenantDenialPreservesCadence(t *testing.T) {
	provider := mock.New()
	k, server, _, dir := startPair(t, provider)
	registry := withTenants(t, server, dir)
	token := mustTenant(t, registry, "acme")
	entry, err := k.Schedules().Add("primary-private", time.Hour, "", "operator", time.Now().Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	before := k.Schedules().List()
	client := tenantClient(t, dir, token)
	for _, cmd := range []string{controlplane.CmdScheduleAdd, controlplane.CmdScheduleList, controlplane.CmdScheduleSystemTasks, controlplane.CmdScheduleRemove, controlplane.CmdScheduleRun, controlplane.CmdScheduleEnable, controlplane.CmdScheduleEdit, controlplane.CmdScheduleTest} {
		if _, err := client.Call(context.Background(), cmd, map[string]any{"tenant": "acme", "id": entry.ID, "intent": "new", "interval_sec": float64(60), "enabled": false}); err == nil || (!strings.Contains(err.Error(), "forbidden") && !strings.Contains(err.Error(), "unauthorized")) {
			t.Fatal(cmd, err)
		}
	}
	if after := k.Schedules().List(); !reflect.DeepEqual(before, after) || provider.CallCount() != 0 {
		t.Fatal("denied tenant changed cadence/provider", before, after, provider.CallCount())
	}
}

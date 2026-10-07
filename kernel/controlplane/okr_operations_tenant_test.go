// SPDX-License-Identifier: MIT
package controlplane_test

import (
	"context"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/okr"
	"github.com/agezt/agezt/plugins/providers/mock"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestOKROperationsSocketTenantDenialPreservesPrimary(t *testing.T) {
	primary, server, _, dir := startPair(t, mock.New())
	registry := withTenants(t, server, dir)
	token := mustTenant(t, registry, "acme")
	objective, err := primary.OKR().Create(okr.CreateSpec{Title: "primary-private"}, time.UnixMilli(100))
	if err != nil {
		t.Fatal(err)
	}
	before := primary.OKR().List(okr.Filter{IncludeArchived: true})
	client := tenantClient(t, dir, token)
	for _, cmd := range []string{controlplane.CmdOKRList, controlplane.CmdOKRShow, controlplane.CmdOKRCreate, controlplane.CmdOKRKeyResult, controlplane.CmdOKRLink, controlplane.CmdOKRUnlink, controlplane.CmdOKRArchive} {
		if _, err := client.Call(context.Background(), cmd, map[string]any{"id": objective.ID, "title": "tenant attempt", "tenant": "acme"}); err == nil || !strings.Contains(err.Error(), "forbidden") {
			t.Fatal(cmd, err)
		}
	}
	after := primary.OKR().List(okr.Filter{IncludeArchived: true})
	if !reflect.DeepEqual(before, after) {
		t.Fatal("tenant changed primary objectives")
	}
}

// SPDX-License-Identifier: MIT
package controlplane_test

import (
	"context"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/modules/work/okr"
	"github.com/agezt/agezt/plugins/providers/mock"
	"reflect"
	"strings"
	"testing"
)

func TestOKROperationsSocketTenantDenialPreservesPrimary(t *testing.T) {
	primary, _, _, dir, registry := startPairWithTenants(t, mock.New())
	token := mustTenant(t, registry, "acme")
	objective, err := primary.OKR().Create(okr.CreateSpec{Title: "primary-private"}, 100)
	if err != nil {
		t.Fatal(err)
	}
	before := primary.OKR().List(okr.Filter{IncludeArchived: true})
	client := tenantClient(t, dir, token)
	for _, cmd := range []string{opapi.OKRList, opapi.OKRShow, opapi.OKRCreate, opapi.OKRKeyResult, opapi.OKRLink, opapi.OKRUnlink, opapi.OKRArchive} {
		if _, err := client.Call(context.Background(), cmd, map[string]any{"id": objective.ID, "title": "tenant attempt", "tenant": "acme"}); err == nil || !strings.Contains(err.Error(), "forbidden") {
			t.Fatal(cmd, err)
		}
	}
	after := primary.OKR().List(okr.Filter{IncludeArchived: true})
	if !reflect.DeepEqual(before, after) {
		t.Fatal("tenant changed primary objectives")
	}
}

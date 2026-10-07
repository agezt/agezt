// SPDX-License-Identifier: MIT
package controlplane_test

import (
	"context"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/plugins/providers/mock"
	"strings"
	"testing"
)

func TestToolboxInstallNativeTenantDenialPreservesJournal(t *testing.T) {
	p := mock.New()
	k, s, _, dir := startPair(t, p)
	registry := withTenants(t, s, dir)
	client := tenantClient(t, dir, mustTenant(t, registry, "acme"))
	head, hash := k.Journal().Head()
	if _, err := client.Call(context.Background(), controlplane.CmdToolboxInstall, map[string]any{"names": []any{"__agezt_owned_not_in_catalog__"}}); err == nil || (!strings.Contains(err.Error(), "forbidden") && !strings.Contains(err.Error(), "unauthorized")) {
		t.Fatal(err)
	}
	after, afterHash := k.Journal().Head()
	if head != after || hash != afterHash || p.CallCount() != 0 {
		t.Fatal(head, after, p.CallCount())
	}
}

// SPDX-License-Identifier: MIT
package controlplane_test

import (
	"context"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func TestChannelInventoryNativeTenantDenialNoAuditOrProvider(t *testing.T) {
	p := mock.New()
	k, s, _, dir := startPair(t, p)
	registry := withTenants(t, s, dir)
	client := tenantClient(t, dir, mustTenant(t, registry, "acme"))
	head, hash := k.Journal().Head()
	if _, err := client.Call(context.Background(), controlplane.CmdChannelList, map[string]any{"tenant": "acme", "unknown": true}); err == nil || (!strings.Contains(err.Error(), "forbidden") && !strings.Contains(err.Error(), "unauthorized")) {
		t.Fatal(err)
	}
	after, afterHash := k.Journal().Head()
	if head != after || hash != afterHash || p.CallCount() != 0 {
		t.Fatal("denied list changed journal/provider")
	}
}

// SPDX-License-Identifier: MIT
package controlplane_test

import (
	"context"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/plugins/providers/mock"
	"strings"
	"testing"
)

func TestSettingsReadsNativeTenantDenialNoAuditOrProvider(t *testing.T) {
	p := mock.New()
	k, s, _, dir := startPair(t, p)
	registry := withTenants(t, s, dir)
	client := tenantClient(t, dir, mustTenant(t, registry, "acme"))
	head, hash := k.Journal().Head()
	for _, command := range []string{controlplane.CmdConfigSchema, controlplane.CmdConfigValues} {
		if _, err := client.Call(context.Background(), command, map[string]any{"tenant": "acme", "unknown": true}); err == nil || (!strings.Contains(err.Error(), "forbidden") && !strings.Contains(err.Error(), "unauthorized")) {
			t.Fatal(command, err)
		}
	}
	after, afterHash := k.Journal().Head()
	if head != after || hash != afterHash || p.CallCount() != 0 {
		t.Fatal("denied read changed journal/provider")
	}
}

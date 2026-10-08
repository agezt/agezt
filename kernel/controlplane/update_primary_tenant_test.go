// SPDX-License-Identifier: MIT
package controlplane_test

import (
	"context"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/plugins/providers/mock"
	"testing"
)

func TestUpdateNativePrimaryOnlyTenantDenied(t *testing.T) {
	_, server, _, dir := startPair(t, mock.New())
	registry := withTenants(t, server, dir)
	token := mustTenant(t, registry, "acme")
	client := tenantClient(t, dir, token)
	for _, cmd := range []string{controlplane.CmdUpdateCheck, controlplane.CmdUpdateApply} {
		if _, err := client.Call(context.Background(), cmd, map[string]any{"tenant": "acme", "version": "v", "sha256": "s", "url": "u"}); err == nil {
			t.Fatal("tenant update admitted", cmd)
		}
	}
}

// SPDX-License-Identifier: MIT
package controlplane_test

import (
	"context"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/plugins/providers/mock"
	"testing"
)

func TestUpdateNativePrimaryOnlyTenantDenied(t *testing.T) {
	_, _, _, dir, registry := startPairWithTenants(t, mock.New())
	token := mustTenant(t, registry, "acme")
	client := tenantClient(t, dir, token)
	for _, cmd := range []string{opapi.UpdateCheck, opapi.UpdateApply} {
		if _, err := client.Call(context.Background(), cmd, map[string]any{"tenant": "acme", "version": "v", "sha256": "s", "url": "u"}); err == nil {
			t.Fatal("tenant update admitted", cmd)
		}
	}
}

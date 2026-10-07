// SPDX-License-Identifier: MIT
package controlplane_test

import (
	"context"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/plugins/providers/mock"
	"strings"
	"testing"
)

func TestSettingsWritesNativeTenantDenialPreservesJournalAndStores(t *testing.T) {
	p := mock.New()
	k, s, _, dir := startPair(t, p)
	registry := withTenants(t, s, dir)
	client := tenantClient(t, dir, mustTenant(t, registry, "acme"))
	head, hash := k.Journal().Head()
	for _, tc := range []struct {
		command string
		args    map[string]any
	}{{controlplane.CmdConfigSet, map[string]any{"name": "AGEZT_MODEL", "value": "owned"}}, {controlplane.CmdConfigSchemaRegister, map[string]any{"section": map[string]any{"id": "owned", "name": "Owned", "fields": []any{map[string]any{"env": "AGEZT_W45B_DENIED", "type": "text"}}}}}, {controlplane.CmdConfigSchemaUnregister, map[string]any{"id": "owned", "force": true}}} {
		if _, err := client.Call(context.Background(), tc.command, tc.args); err == nil || (!strings.Contains(err.Error(), "forbidden") && !strings.Contains(err.Error(), "unauthorized")) {
			t.Fatal(tc.command, err)
		}
	}
	after, afterHash := k.Journal().Head()
	if head != after || hash != afterHash || p.CallCount() != 0 {
		t.Fatal("denied writers changed journal/provider")
	}
}

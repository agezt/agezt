// SPDX-License-Identifier: MIT
package controlplane_test

import (
	"context"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/plugins/providers/mock"
	"strings"
	"testing"
)

func TestToolboxReadsNativeTenantDenialPreservesJournal(t *testing.T) {
	provider := mock.New()
	k, server, _, dir := startPair(t, provider)
	registry := withTenants(t, server, dir)
	client := tenantClient(t, dir, mustTenant(t, registry, "acme"))
	head, hash := k.Journal().Head()
	for _, cmd := range []string{controlplane.CmdToolboxDetect, controlplane.CmdToolboxOutdated} {
		if _, err := client.Call(context.Background(), cmd, map[string]any{"tenant": "acme"}); err == nil || (!strings.Contains(err.Error(), "forbidden") && !strings.Contains(err.Error(), "unauthorized")) {
			t.Fatal(cmd, err)
		}
	}
	after, afterHash := k.Journal().Head()
	if head != after || hash != afterHash || provider.CallCount() != 0 {
		t.Fatal(head, after, provider.CallCount())
	}
}

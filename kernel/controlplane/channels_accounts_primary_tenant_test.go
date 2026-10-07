// SPDX-License-Identifier: MIT
package controlplane_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/settings"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func TestChannelAccountsNativeTenantDenialBeforePersistenceAuditAndProvider(t *testing.T) {
	p := mock.New()
	k, s, _, dir := startPair(t, p)
	registry := withTenants(t, s, dir)
	client := tenantClient(t, dir, mustTenant(t, registry, "acme"))
	head, hash := k.Journal().Head()
	for _, command := range []string{controlplane.CmdChannelAccountSet, controlplane.CmdChannelAccountRemove} {
		if _, err := client.Call(context.Background(), command, map[string]any{"tenant": "acme", "kind": "email", "label": "work", "name": "AGEZT_EMAIL_SMTP_ADDR", "value": "replacement"}); err == nil || (!strings.Contains(err.Error(), "forbidden") && !strings.Contains(err.Error(), "unauthorized")) {
			t.Fatal(command, err)
		}
	}
	after, afterHash := k.Journal().Head()
	if head != after || hash != afterHash || p.CallCount() != 0 {
		t.Fatal("denied writer audit/provider effects")
	}
	if _, err := os.Stat(settings.NewStore(dir).Path); !os.IsNotExist(err) {
		t.Fatal("denied writer created config", err)
	}
}

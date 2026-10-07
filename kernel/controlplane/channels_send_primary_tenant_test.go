// SPDX-License-Identifier: MIT
package controlplane_test

import (
	"context"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func TestChannelSendNativeTenantDenialNoEffectAuditOrProvider(t *testing.T) {
	p := mock.New()
	k, s, _, dir := startPair(t, p)
	calls := 0
	s.SetChannelSender(func(context.Context, string, string, string) error { calls++; return nil })
	registry := withTenants(t, s, dir)
	client := tenantClient(t, dir, mustTenant(t, registry, "acme"))
	head, hash := k.Journal().Head()
	if _, err := client.Call(context.Background(), controlplane.CmdSend, map[string]any{"tenant": "acme", "channel": "slack", "to": "owned", "text": "fixture"}); err == nil || (!strings.Contains(err.Error(), "forbidden") && !strings.Contains(err.Error(), "unauthorized")) {
		t.Fatal(err)
	}
	after, afterHash := k.Journal().Head()
	if head != after || hash != afterHash || calls != 0 || p.CallCount() != 0 {
		t.Fatal("denied send changed sender/journal/provider")
	}
}

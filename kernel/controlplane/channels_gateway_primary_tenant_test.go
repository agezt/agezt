// SPDX-License-Identifier: MIT
package controlplane_test

import (
	"context"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func TestChannelGatewayNativeTenantDenialNoAuditOrProvider(t *testing.T) {
	p := mock.New()
	k, _, _, dir, registry := startPairWithTenants(t, p)
	client := tenantClient(t, dir, mustTenant(t, registry, "acme"))
	for _, command := range []string{controlplane.CmdWhatsAppGatewayStatus, controlplane.CmdWhatsAppGatewayQR} {
		head, hash := k.Journal().Head()
		if _, err := client.Call(context.Background(), command, map[string]any{"tenant": "acme", "url": "owned invalid", "key": "owned secret"}); err == nil || (!strings.Contains(err.Error(), "forbidden") && !strings.Contains(err.Error(), "unauthorized")) {
			t.Fatal(command, err)
		}
		after, afterHash := k.Journal().Head()
		if head != after || hash != afterHash || p.CallCount() != 0 {
			t.Fatal("denied gateway changed journal/provider")
		}
	}
}

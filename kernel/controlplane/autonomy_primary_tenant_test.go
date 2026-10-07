// SPDX-License-Identifier: MIT
package controlplane_test

import (
	"context"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/plugins/providers/mock"
	"strings"
	"testing"
)

func TestAutonomyPrimaryFeedSocketTenantDenialPreservesJournal(t *testing.T) {
	provider := mock.New()
	k, server, _, dir := startPair(t, provider)
	registry := withTenants(t, server, dir)
	token := mustTenant(t, registry, "acme")
	if _, err := k.Bus().Publish(event.Spec{Subject: "primary-private", Kind: event.KindScheduleFired, Actor: "fixture"}); err != nil {
		t.Fatal(err)
	}
	head, hash := k.Journal().Head()
	client := tenantClient(t, dir, token)
	if _, err := client.Call(context.Background(), controlplane.CmdAutonomyFeed, map[string]any{"tenant": "acme"}); err == nil || (!strings.Contains(err.Error(), "forbidden") && !strings.Contains(err.Error(), "unauthorized")) {
		t.Fatal(err)
	}
	after, afterHash := k.Journal().Head()
	if head != after || hash != afterHash || provider.CallCount() != 0 {
		t.Fatal(head, after, provider.CallCount())
	}
}

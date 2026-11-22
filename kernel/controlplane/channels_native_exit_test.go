// SPDX-License-Identifier: MIT
package controlplane

import (
	appchannels "github.com/agezt/agezt/kernel/app/channels"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"reflect"
	"testing"
)

func TestChannelNativeElevenTypedOperationsExit(t *testing.T) {
	type signature struct {
		input, output reflect.Type
		read          bool
	}
	expected := map[string]signature{
		opapi.ChannelList:           {reflect.TypeFor[appchannels.ListInput](), reflect.TypeFor[appchannels.ListOutput](), true},
		opapi.ChannelAccountSet:     {reflect.TypeFor[appchannels.SetAccountRequest](), reflect.TypeFor[appchannels.SetAccountOutput](), false},
		opapi.ChannelAccountRemove:  {reflect.TypeFor[appchannels.RemoveAccountRequest](), reflect.TypeFor[appchannels.RemoveAccountOutput](), false},
		opapi.ChannelOAuthStart:     {reflect.TypeFor[appchannels.OAuthStartRequest](), reflect.TypeFor[appchannels.OAuthStartOutput](), false},
		opapi.ChannelOAuthCallback:  {reflect.TypeFor[appchannels.OAuthCallbackRequest](), reflect.TypeFor[appchannels.OAuthCallbackOutput](), false},
		opapi.ChannelOAuthStatus:    {reflect.TypeFor[appchannels.OAuthStatusRequest](), reflect.TypeFor[appchannels.OAuthStatusOutput](), true},
		opapi.WhatsAppGatewayStatus: {reflect.TypeFor[appchannels.GatewayRequest](), reflect.TypeFor[appchannels.GatewayStatusOutput](), true},
		opapi.WhatsAppGatewayQR:     {reflect.TypeFor[appchannels.GatewayRequest](), reflect.TypeFor[appchannels.GatewayQROutput](), true},
		opapi.Inbox:                 {reflect.TypeFor[appchannels.InboxRequest](), reflect.TypeFor[appchannels.InboxOutput](), true},
		opapi.Send:                  {reflect.TypeFor[appchannels.SendRequest](), reflect.TypeFor[appchannels.SendOutput](), false},
		opapi.ACPAgents:             {reflect.TypeFor[appchannels.ACPInput](), reflect.TypeFor[appchannels.ACPOutput](), true},
	}
	seen := map[string]int{}
	for _, operation := range registeredAppOperations() {
		s := operation.Spec()
		sig, ok := expected[s.Name]
		if !ok {
			continue
		}
		wire, found := commandRegistry[s.Name]
		if !found || !wire.AppOwned || wire.ReadOnly != sig.read || wire.TenantAllowed || wire.TenantRouted || wire.Streaming != StreamNone || s.Authz != opapi.PrimaryOnly || s.Tenancy != opapi.Primary || s.Stream != opapi.StreamNone || !s.AllowUnknownInput || s.Input != sig.input || s.Output != sig.output || s.Emission != nil || len(s.EmissionSchema) != 0 {
			t.Fatal(s, wire)
		}
		seen[s.Name]++
	}
	if len(seen) != 11 {
		t.Fatal(seen)
	}
	for _, n := range seen {
		if n != 1 {
			t.Fatal(seen)
		}
	}
	if len(channelInventoryOperations) != 1 || len(channelAccountOperations) != 2 || len(channelOAuthOperations) != 3 || len(channelGatewayOperations) != 2 || len(channelInboxOperations) != 1 || len(channelSendOperations) != 1 || len(acpInventoryOperations) != 1 {
		t.Fatal("channel operation aggregate incomplete")
	}
}

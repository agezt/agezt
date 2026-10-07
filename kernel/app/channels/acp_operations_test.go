// SPDX-License-Identifier: MIT
package channels

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/platform/schema"
)

func TestChannelACPOperationSpecOwnsPrimaryReadOnlyGETAndTypedSchema(t *testing.T) {
	if _, err := ACPInventoryOperations(nil); err == nil {
		t.Fatal("nil service provider registered")
	}
	operations, err := ACPInventoryOperations(func(context.Context) *ACPInventory { return nil })
	if err != nil || len(operations) != 1 {
		t.Fatal(operations, err)
	}
	spec := operations[0].Spec()
	if spec.Name != "acp_agents" || !spec.ReadOnly || spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary || spec.Stream != opapi.StreamNone || !spec.AllowUnknownInput || spec.HTTP.Method != "GET" || spec.HTTP.Path != "/api/acp/agents" || spec.Input != reflect.TypeFor[ACPInput]() || spec.Output != reflect.TypeFor[ACPOutput]() {
		t.Fatal(spec)
	}
	for _, raw := range []string{`{}`, `{"force":true,"active_command":42,"tenant":"ignored","unknown":{"token":"owned"}}`} {
		if err := schema.ValidateJSON(spec.InputSchema, json.RawMessage(raw)); err != nil {
			t.Fatal(raw, err)
		}
	}
	for _, raw := range []string{`{"os":"owned","agents":null,"installed_count":0,"missing_count":0}`, `{"os":"owned","agents":[],"installed_count":0,"missing_count":0,"registry_error":"owned unavailable","clients_error":"owned failure"}`, `{"os":"owned","agents":[],"installed_count":0,"missing_count":0,"registered_count":9007199254740993}`} {
		if err := schema.ValidateJSON(spec.OutputSchema, json.RawMessage(raw)); err != nil {
			t.Fatal(raw, err)
		}
		var out ACPOutput
		if err := json.Unmarshal([]byte(raw), &out); err != nil {
			t.Fatal(err)
		}
		after, _ := json.Marshal(out)
		var beforeValue, afterValue any
		json.Unmarshal([]byte(raw), &beforeValue)
		json.Unmarshal(after, &afterValue)
		if !reflect.DeepEqual(beforeValue, afterValue) {
			t.Fatal(raw, string(after))
		}
	}
	for _, raw := range []string{`{"os":"owned","agents":null,"installed_count":"wrong","missing_count":0}`, `{"os":"owned","agents":[{"slug":"x","name":"x","bin":"x","command":"x","description":"x","installed":"wrong","active":false}],"installed_count":0,"missing_count":0}`, `{"os":"owned","agents":[],"installed_count":0,"missing_count":0,"clients_cached":"wrong"}`} {
		if schema.ValidateJSON(spec.OutputSchema, json.RawMessage(raw)) == nil {
			t.Fatal("typed output schema erased", raw)
		}
	}
}

type ownedACPAuth struct {
	principal opapi.Principal
	err       error
}

func (a ownedACPAuth) Authenticate(context.Context, opapi.Caller) (opapi.Principal, error) {
	return a.principal, a.err
}

type ownedACPRoute struct{}
type ownedACPRouteKey struct{}

func (ownedACPRoute) Route(ctx context.Context, p opapi.Principal, s opapi.Spec) (context.Context, error) {
	if s.Tenancy == opapi.Primary && p.Tenant != "" {
		return nil, errors.New("primary inventory inherited tenant")
	}
	return context.WithValue(ctx, ownedACPRouteKey{}, "owned-route"), nil
}

func TestChannelACPOperationDispatchPrimaryUnauditedAndCanceledBeforeDiscovery(t *testing.T) {
	providerCalls, discoveryCalls := 0, 0
	ops, err := ACPInventoryOperations(func(ctx context.Context) *ACPInventory {
		providerCalls++
		if ctx.Value(ownedACPRouteKey{}) != "owned-route" {
			t.Fatal("service provider lost routed context")
		}
		return NewACPInventory(func() string { return " owned " }, func(got context.Context, active string, force bool) ACPOutput {
			discoveryCalls++
			if got != ctx || active != "owned" || force {
				t.Fatal("operation lost use-case context")
			}
			return ACPOutput{OS: "owned", Agents: nil, RegistryError: "owned source failure", InstalledCount: 0, MissingCount: 0}
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, principal := range []opapi.Principal{{Kind: opapi.Operator}, {Kind: opapi.Tenant, Tenant: "acme"}, {Kind: opapi.Agent}, {Kind: opapi.System}} {
		d, err := app.NewDispatcher(ops, app.Dependencies{Auth: ownedACPAuth{principal: principal}, Router: ownedACPRoute{}})
		if err != nil {
			t.Fatal(err)
		}
		beforeProvider, beforeDiscovery := providerCalls, discoveryCalls
		out, err := d.Dispatch(context.Background(), opapi.Caller{Tenant: "acme"}, "acp_agents", json.RawMessage(`{"force":true,"active_command":"ignored"}`), nil)
		if principal.Kind != opapi.Operator {
			if err == nil || providerCalls != beforeProvider || discoveryCalls != beforeDiscovery {
				t.Fatal("non-operator entered discovery", out, err)
			}
			continue
		}
		if err != nil || out.(ACPOutput).RegistryError != "owned source failure" || providerCalls != beforeProvider+1 || discoveryCalls != beforeDiscovery+1 {
			t.Fatal("primary read required audit or lost in-band result", out, err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		beforeProvider, beforeDiscovery = providerCalls, discoveryCalls
		if out, err := d.Dispatch(ctx, opapi.Caller{}, "acp_agents", json.RawMessage(`{}`), nil); !errors.Is(err, context.Canceled) || out != nil || providerCalls != beforeProvider || discoveryCalls != beforeDiscovery {
			t.Fatal("canceled request entered discovery", out, err)
		}
		for _, raw := range []string{`[]`, `"wrong"`, `{} {}`, `{"force":`} {
			if out, err := d.Dispatch(context.Background(), opapi.Caller{}, "acp_agents", json.RawMessage(raw), nil); err == nil || out != nil || providerCalls != beforeProvider || discoveryCalls != beforeDiscovery {
				t.Fatal("malformed root entered discovery", raw, out, err)
			}
		}
	}
}

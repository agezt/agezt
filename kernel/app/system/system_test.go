// SPDX-License-Identifier: MIT

package system_test

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/app/system"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/platform/schema"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

type tenantCount int

func (n tenantCount) Count() int { return int(n) }

func TestTypedHandlersWorkWithoutTransportAndRemainReadOnly(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })
	service := system.New(k, nil, nil, nil, "")
	before, _ := k.Journal().Head()
	status, err := service.Status(context.Background(), system.StatusInput{})
	if err != nil {
		t.Fatal(err)
	}
	if status.Daemon != brand.Version || status.Protocol != brand.ProtocolVersion || status.JournalHead != int64(0) {
		t.Fatalf("status identity/head=%v", status)
	}
	statusWire := wireMap(t, status)
	for _, absent := range []string{"tenants", "http_servers", "channels", "cred_chain"} {
		if _, ok := statusWire[absent]; ok {
			t.Errorf("absent field %q appeared", absent)
		}
	}
	version, err := service.Version(context.Background(), system.VersionInput{})
	if err != nil {
		t.Fatal(err)
	}
	revision, built, modified := brand.BuildInfo()
	want := map[string]any{brand.Binary: brand.Version, "protocol_version": brand.ProtocolVersion, "revision": revision, "built": built, "build_modified": modified}
	if !reflect.DeepEqual(wireMap(t, version), wireMap(t, want)) {
		t.Fatalf("version=%v want=%v", version, want)
	}
	after, _ := k.Journal().Head()
	if before != after {
		t.Fatalf("read handler changed journal: %d -> %d", before, after)
	}
}

func TestStatusRetainsMetadataAndFallbackDimensions(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })
	for _, spec := range []event.Spec{
		{Kind: event.KindProviderFallback, Payload: map[string]any{"scope": "provider", "reason": "provider failed"}},
		{Kind: event.KindProviderFallback, Payload: map[string]any{"scope": "model-chain", "reason": "model failed"}},
	} {
		spec.Subject, spec.Actor = "governor", "test"
		if _, err := k.Bus().Publish(spec); err != nil {
			t.Fatal(err)
		}
	}
	service := system.New(k, tenantCount(2), []system.HTTPBinding{{Name: "console", Addr: "127.0.0.1:9000", Loopback: true}}, []system.ChannelInfo{{Kind: "telegram", Inbound: true, Allowlist: 3}}, "ambient chain")
	status, err := service.Status(context.Background(), system.StatusInput{})
	if err != nil {
		t.Fatal(err)
	}
	if status.Tenants == nil || *status.Tenants != 2 || status.CredChain != "ambient chain" {
		t.Fatalf("optional metadata=%v", status)
	}
	statusWire := wireMap(t, status)
	for name, reason := range map[string]string{"provider_fallbacks": "provider failed", "model_fallbacks": "model failed"} {
		fallback := statusWire[name].(map[string]any)
		if fallback["count"] != float64(1) || fallback["last_reason"] != reason || fallback["last_ms"].(float64) <= 0 {
			t.Fatalf("%s=%v", name, fallback)
		}
	}
	encoded, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err := json.Unmarshal(encoded, &wire); err != nil {
		t.Fatal(err)
	}
	if len(wire["http_servers"].([]any)) != 1 || len(wire["channels"].([]any)) != 1 {
		t.Fatalf("metadata wire shape=%s", encoded)
	}
}

func wireMap(t *testing.T, value any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestStatusZeroTenantsAndMetadataWireContract(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })
	bindings := []system.HTTPBinding{{Name: "console", Addr: "127.0.0.1:9000", Loopback: true}}
	channels := []system.ChannelInfo{{Kind: "telegram", Inbound: true, Allowlist: 3}}
	service := system.New(k, tenantCount(0), bindings, channels, "ambient")
	status, err := service.Status(context.Background(), system.StatusInput{})
	if err != nil {
		t.Fatal(err)
	}
	wire := wireMap(t, status)
	if wire["tenants"] != float64(0) {
		t.Fatalf("enabled empty registry disappeared: %v", wire)
	}
	for name, want := range map[string]any{
		"http_servers":       []any{map[string]any{"name": "console", "addr": "127.0.0.1:9000", "loopback": true}},
		"channels":           []any{map[string]any{"kind": "telegram", "inbound": true, "addr": "", "allowlist": float64(3)}},
		"schedules":          map[string]any{"total": float64(0), "enabled": float64(0), "running": float64(0), "resident": false},
		"provider_fallbacks": map[string]any{"count": float64(0), "last_reason": "", "last_ms": float64(0)},
		"model_fallbacks":    map[string]any{"count": float64(0), "last_reason": "", "last_ms": float64(0)},
	} {
		if !reflect.DeepEqual(wire[name], want) {
			t.Errorf("%s wire=%v want=%v", name, wire[name], want)
		}
	}
	// Returned metadata must remain a snapshot after caller-owned host slices change.
	bindings[0].Addr, channels[0].Kind = "changed", "changed"
	if !reflect.DeepEqual(wireMap(t, status), wire) {
		t.Fatal("output aliases host metadata")
	}
}

func TestSystemOperationSchemasDescribeTypedWireOutputs(t *testing.T) {
	operations, err := system.Operations(func(context.Context) *system.Service { return nil })
	if err != nil {
		t.Fatal(err)
	}
	for _, operation := range operations {
		spec := operation.Spec()
		var output any
		if spec.Name == "status" {
			if spec.Output != reflect.TypeFor[system.StatusOutput]() {
				t.Fatalf("status type=%v", spec.Output)
			}
			output = system.StatusOutput{}
		} else {
			if spec.Output != reflect.TypeFor[system.VersionOutput]() {
				t.Fatalf("version type=%v", spec.Output)
			}
			output = system.VersionOutput{}
		}
		raw, err := json.Marshal(output)
		if err != nil {
			t.Fatal(err)
		}
		if err := schema.ValidateJSON(spec.OutputSchema, raw); err != nil {
			t.Fatalf("%s zero wire rejected: %v", spec.Name, err)
		}
		wire := wireMap(t, output)
		if spec.Name == "status" {
			wire["schedules"].(map[string]any)["running"] = "invalid"
		} else {
			wire[brand.Binary] = 42
		}
		invalid, err := json.Marshal(wire)
		if err != nil {
			t.Fatal(err)
		}
		if schema.ValidateJSON(spec.OutputSchema, invalid) == nil {
			t.Fatalf("%s schema accepted wrong nested/brand type", spec.Name)
		}
		wire = wireMap(t, output)
		if spec.Name == "status" {
			delete(wire, "delegation")
		} else {
			delete(wire, "revision")
		}
		invalid, err = json.Marshal(wire)
		if err != nil {
			t.Fatal(err)
		}
		if schema.ValidateJSON(spec.OutputSchema, invalid) == nil {
			t.Fatalf("%s schema accepted missing required output", spec.Name)
		}
	}
}

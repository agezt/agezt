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
	if status["daemon"] != brand.Version || status["protocol"] != brand.ProtocolVersion || status["journal_head"] != int64(0) {
		t.Fatalf("status identity/head=%v", status)
	}
	for _, absent := range []string{"tenants", "http_servers", "channels", "cred_chain"} {
		if _, ok := status[absent]; ok {
			t.Errorf("absent field %q appeared", absent)
		}
	}
	version, err := service.Version(context.Background(), system.VersionInput{})
	if err != nil {
		t.Fatal(err)
	}
	revision, built, modified := brand.BuildInfo()
	want := map[string]any{brand.Binary: brand.Version, "protocol_version": brand.ProtocolVersion, "revision": revision, "built": built, "build_modified": modified}
	if !reflect.DeepEqual(version, want) {
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
	if status["tenants"] != 2 || status["cred_chain"] != "ambient chain" {
		t.Fatalf("optional metadata=%v", status)
	}
	for name, reason := range map[string]string{"provider_fallbacks": "provider failed", "model_fallbacks": "model failed"} {
		fallback := status[name].(map[string]any)
		if fallback["count"] != 1 || fallback["last_reason"] != reason || fallback["last_ms"].(int64) <= 0 {
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

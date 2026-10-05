// SPDX-License-Identifier: MIT

package controlplane

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/agezt/agezt/kernel/app/providers"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/creds"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func TestProviderCommandMetadataComesFromAppSpecs(t *testing.T) {
	if len(providerOperations) != 6 {
		t.Fatalf("provider operations=%d", len(providerOperations))
	}
	for _, operation := range providerOperations {
		spec := operation.Spec()
		wire := commandRegistry[spec.Name]
		if !wire.AppOwned || wire.ReadOnly != (spec.Name == CmdProviderKeyList) || wire.ReadOnly != spec.ReadOnly || wire.TenantAllowed || wire.TenantRouted || wire.Streaming != StreamNone || spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary || !spec.AllowUnknownInput {
			t.Fatalf("provider metadata=%+v %+v", spec, wire)
		}
		if spec.Name == CmdProviderKeyAdd && spec.Output != reflect.TypeFor[providers.KeyAddOutput]() {
			t.Fatal("key add output is opaque")
		}
	}
}

func TestProviderAppSocketKeyringAdmissionPrivacyAndReload(t *testing.T) {
	t.Setenv(creds.PassphraseEnvVar, "isolated-provider-fixture")
	var reloads atomic.Int32
	dir := t.TempDir()
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New(), OnReload: func() error { reloads.Add(1); return nil }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })
	s := NewServer(k, dir)
	s.token = "primary"
	secret := strings.Join([]string{"provider", "fixture", "private", "1234"}, "-")
	call := func(command string, args map[string]any) Response {
		t.Helper()
		responses := callAppHost(t, s, Request{ID: command, Cmd: command, Token: "primary", Args: args})
		last := responses[len(responses)-1]
		if last.Type != RespResult {
			t.Fatalf("%s failed: %+v", command, last)
		}
		return last
	}
	call(CmdProviderConnect, map[string]any{"id": "fixture", "api": "https://fixture.invalid", "model": "hint-only", "ignored": true})
	if len(k.Catalog().Providers["fixture"].Models) != 0 {
		t.Fatal("connect synthesized model membership")
	}
	call(CmdProviderKeyAdd, map[string]any{"provider": "fixture", "env": "PROVIDER_FIXTURE_KEY", "label": "first", "value": secret, "tenant": "ignored"})
	call(CmdProviderKeyAdd, map[string]any{"provider": "fixture", "env": "PROVIDER_FIXTURE_KEY", "label": "second", "value": secret + "-5678", "active": false})
	if reloads.Load() != 2 {
		t.Fatalf("inactive add reloaded=%d", reloads.Load())
	}
	before, _ := k.Journal().Head()
	listed := call(CmdProviderKeyList, map[string]any{"provider": "fixture", "env": "PROVIDER_FIXTURE_KEY", "active": "ignored", "value": 42})
	after, _ := k.Journal().Head()
	if before != after || len(listed.Result["keys"].([]any)) != 2 {
		t.Fatalf("list changed journal/wire=%v %d->%d", listed, before, after)
	}
	encoded, err := json.Marshal(listed)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), secret) {
		t.Fatal("key value reached socket response")
	}
	other := call(CmdProviderKeyList, map[string]any{"provider": "other", "env": "PROVIDER_FIXTURE_KEY"})
	if len(other.Result["keys"].([]any)) != 0 {
		t.Fatal("keyring scope crossed provider")
	}
	call(CmdProviderKeyActivate, map[string]any{"provider": "fixture", "env": "PROVIDER_FIXTURE_KEY", "label": "second"})
	call(CmdProviderKeyRemove, map[string]any{"provider": "fixture", "env": "PROVIDER_FIXTURE_KEY", "label": "first"})
	call(CmdProviderKeyRemove, map[string]any{"provider": "fixture", "env": "PROVIDER_FIXTURE_KEY", "label": "second"})
	call(CmdProviderReload, nil)
	if reloads.Load() != 5 {
		t.Fatalf("provider lifecycle reloads=%d want=5", reloads.Load())
	}
	counts := map[string][]*event.Event{}
	if err := k.Journal().Range(func(e *event.Event) error {
		if strings.HasPrefix(e.Subject, "op.provider_") {
			counts[e.Subject] = append(counts[e.Subject], e)
			if strings.Contains(string(e.Payload), secret) {
				t.Error("private key entered operation audit")
			}
			var payload map[string]any
			if err := json.Unmarshal(e.Payload, &payload); err != nil {
				return err
			}
			if e.Kind == event.KindOpInvoked {
				if _, ok := payload["tenant"]; ok {
					t.Error("primary keyring operation got ignored tenant identity")
				}
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for subject, events := range counts {
		if len(events)%2 != 0 {
			t.Fatalf("unpaired provider audit: %s", subject)
		}
		for i := 0; i < len(events); i += 2 {
			if events[i].Kind != event.KindOpInvoked || events[i+1].Kind != event.KindOpCompleted || events[i].CorrelationID == "" || events[i].CorrelationID != events[i+1].CorrelationID {
				t.Fatalf("provider audit arc: %s %v", subject, events)
			}
		}
	}
	if len(counts["op."+CmdProviderKeyAdd]) != 4 || len(counts["op."+CmdProviderKeyList]) != 0 {
		t.Fatalf("provider audit counts=%v", counts)
	}
}

func TestProviderAppRejectsBeforeVaultAndReloadEffects(t *testing.T) {
	for _, mode := range []string{"bad-input", "audit-unavailable"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv(creds.PassphraseEnvVar, "isolated-provider-fixture")
			var reloads atomic.Int32
			dir := t.TempDir()
			k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New(), OnReload: func() error { reloads.Add(1); return nil }})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { k.Close() })
			s := NewServer(k, dir)
			s.token = "primary"
			args := map[string]any{"env": "PROVIDER_FIXTURE_KEY", "label": "fixture", "value": "synthetic-fixture-value"}
			if mode == "bad-input" {
				args["active"] = "wrong"
			} else {
				_ = k.Journal().Close()
			}
			response := callAppHost(t, s, Request{ID: "rejected", Cmd: CmdProviderKeyAdd, Token: "primary", Args: args})[0]
			if response.Type != RespError || reloads.Load() != 0 {
				t.Fatalf("rejected keyring request=%v reloads=%d", response, reloads.Load())
			}
			if _, err := os.Stat(creds.NewStore(dir).Path); !os.IsNotExist(err) {
				t.Fatalf("rejected keyring request created vault: %v", err)
			}
		})
	}
}

// SPDX-License-Identifier: MIT
package controlplane

import (
	"bufio"
	"context"
	"encoding/json"
	appsettings "github.com/agezt/agezt/kernel/app/settings"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/settings"
	"github.com/agezt/agezt/plugins/providers/mock"
	"net"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestSettingsTypedNativeMandatoryAuditAndCanceledAdmission(t *testing.T) {
	for _, command := range []string{CmdConfigSet, CmdConfigSchemaRegister, CmdConfigSchemaUnregister} {
		for _, gate := range []string{"closed-journal", "canceled"} {
			t.Run(command+"/"+gate, func(t *testing.T) {
				dir := t.TempDir()
				p := mock.New()
				k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: p})
				if err != nil {
					t.Fatal(err)
				}
				defer k.Close()
				reg := settings.NewRegistry(dir)
				seed := settings.Section{ID: "owned-seed", Name: "Owned", Fields: []settings.Field{{Env: "AGEZT_W45C_PUBLIC", Type: settings.TypeText}}}
				if err := reg.Register(seed); err != nil {
					t.Fatal(err)
				}
				s := NewServer(k, dir)
				s.token = "primary"
				args := map[string]any{}
				switch command {
				case CmdConfigSet:
					args = map[string]any{"name": "AGEZT_W45C_PUBLIC", "value": "owned"}
				case CmdConfigSchemaRegister:
					args = map[string]any{"section": map[string]any{"id": "owned-add", "name": "Owned Added", "fields": []any{map[string]any{"env": "AGEZT_W45C_ADDED", "type": "text"}}}}
				case CmdConfigSchemaUnregister:
					args = map[string]any{"id": "owned-seed"}
				}
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				if gate == "closed-journal" {
					if err := k.Journal().Close(); err != nil {
						t.Fatal(err)
					}
				} else {
					cancel()
				}
				a, b := net.Pipe()
				defer a.Close()
				defer b.Close()
				a.SetDeadline(time.Now().Add(3 * time.Second))
				done := make(chan struct{})
				go func() { defer close(done); s.handleConn(ctx, b) }()
				raw, _ := json.Marshal(Request{ID: "owned", Cmd: command, Token: "primary", Args: args})
				a.Write(append(raw, 10))
				line, err := bufio.NewReader(a).ReadBytes(10)
				if err != nil {
					t.Fatal(err)
				}
				a.Close()
				<-done
				var reply Response
				json.Unmarshal(line, &reply)
				effect := false
				switch command {
				case CmdConfigSet:
					store := settings.NewStore(dir)
					store.Load()
					_, effect = store.Get("AGEZT_W45C_PUBLIC")
				case CmdConfigSchemaRegister:
					_, effect = reg.FieldByEnv("AGEZT_W45C_ADDED")
				case CmdConfigSchemaUnregister:
					_, present := reg.FieldByEnv("AGEZT_W45C_PUBLIC")
					effect = !present
				}
				if reply.Type != RespError || effect || p.CallCount() != 0 || (gate == "canceled" && !strings.Contains(reply.Error, "context canceled")) {
					t.Errorf("EXPECTED:%s rejects before effects ACTUAL:type=%s error=%q effect=%v", gate, reply.Type, reply.Error, effect)
				}
			})
		}
	}
	for _, command := range []string{CmdConfigSchema, CmdConfigValues} {
		t.Run(command+"/canceled", func(t *testing.T) {
			dir := t.TempDir()
			p := mock.New()
			k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: p})
			if err != nil {
				t.Fatal(err)
			}
			defer k.Close()
			s := NewServer(k, dir)
			s.token = "primary"
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			a, b := net.Pipe()
			defer a.Close()
			defer b.Close()
			a.SetDeadline(time.Now().Add(3 * time.Second))
			done := make(chan struct{})
			go func() { defer close(done); s.handleConn(ctx, b) }()
			raw, _ := json.Marshal(Request{ID: "owned", Cmd: command, Token: "primary", Args: map[string]any{"unknown": true}})
			a.Write(append(raw, 10))
			line, err := bufio.NewReader(a).ReadBytes(10)
			if err != nil {
				t.Fatal(err)
			}
			a.Close()
			<-done
			var reply Response
			json.Unmarshal(line, &reply)
			if reply.Type != RespError || !strings.Contains(reply.Error, "context canceled") || p.CallCount() != 0 {
				t.Errorf("EXPECTED:canceled read admission ACTUAL:type=%s error=%q", reply.Type, reply.Error)
			}
		})
	}
}

func TestSettingsTypedNativeFivePolicyAndSignatures(t *testing.T) {
	type signature struct {
		input, output reflect.Type
		read          bool
	}
	expected := map[string]signature{CmdConfigSchema: {reflect.TypeFor[appsettings.SchemaInput](), reflect.TypeFor[appsettings.SchemaOutput](), true}, CmdConfigValues: {reflect.TypeFor[appsettings.ValuesInput](), reflect.TypeFor[appsettings.ValuesOutput](), true}, CmdConfigSet: {reflect.TypeFor[appsettings.SetRequest](), reflect.TypeFor[appsettings.SetOutput](), false}, CmdConfigSchemaRegister: {reflect.TypeFor[appsettings.RegisterRequest](), reflect.TypeFor[appsettings.RegisterOutput](), false}, CmdConfigSchemaUnregister: {reflect.TypeFor[appsettings.UnregisterRequest](), reflect.TypeFor[appsettings.UnregisterOutput](), false}}
	seen := map[string]int{}
	for _, operation := range registeredAppOperations() {
		sp := operation.Spec()
		sig, ok := expected[sp.Name]
		if !ok {
			continue
		}
		wire, ok := commandRegistry[sp.Name]
		if !ok || !wire.AppOwned || wire.ReadOnly != sig.read || wire.TenantAllowed || wire.TenantRouted || wire.Streaming != StreamNone || sp.ReadOnly != sig.read || sp.Authz != opapi.PrimaryOnly || sp.Tenancy != opapi.Primary || sp.Stream != opapi.StreamNone || !sp.AllowUnknownInput || sp.Input != sig.input || sp.Output != sig.output || sp.Emission != nil || len(sp.EmissionSchema) != 0 {
			t.Fatal(sp, wire)
		}
		seen[sp.Name]++
	}
	if len(seen) != 5 || len(settingsOperations) != 5 {
		t.Fatal(seen)
	}
	for _, n := range seen {
		if n != 1 {
			t.Fatal(seen)
		}
	}
}

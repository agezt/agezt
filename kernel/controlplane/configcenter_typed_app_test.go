// SPDX-License-Identifier: MIT
package controlplane

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	appconfigcenter "github.com/agezt/agezt/kernel/app/configcenter"
	core "github.com/agezt/agezt/kernel/configcenter"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
	"net"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestConfigCenterTypedNativeAdmissionNoEffects(t *testing.T) {
	for _, command := range []string{CmdConfigCenterSet, CmdConfigCenterDelete, CmdConfigCenterSetRating, CmdConfigCenterSetAccess, CmdConfigCenterGet, CmdConfigCenterList, CmdConfigCenterAccessLog, CmdConfigCenterAudit, CmdConfigCenterHealth} {
		for _, gate := range []string{"canceled", "closed-journal"} {
			write := command == CmdConfigCenterSet || command == CmdConfigCenterDelete || command == CmdConfigCenterSetRating || command == CmdConfigCenterSetAccess
			if gate == "closed-journal" && !write {
				continue
			}
			t.Run(command+"/"+gate, func(t *testing.T) {
				dir := t.TempDir()
				p := mock.New()
				k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: p})
				if err != nil {
					t.Fatal(err)
				}
				defer k.Close()
				entry := core.NewConfigEntry("owned", "seed")
				if err := k.ConfigCenter().Set(entry); err != nil {
					t.Fatal(err)
				}
				before, _ := json.Marshal(k.ConfigCenter().ListEntries())
				s := NewServer(k, dir)
				s.token = "primary"
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				if gate == "canceled" {
					cancel()
				} else {
					if err := k.Journal().Close(); err != nil {
						t.Fatal(err)
					}
				}
				a, b := net.Pipe()
				defer a.Close()
				defer b.Close()
				a.SetDeadline(time.Now().Add(3 * time.Second))
				done := make(chan struct{})
				go func() { defer close(done); s.handleConn(ctx, b) }()
				raw, _ := json.Marshal(Request{ID: "owned", Cmd: command, Token: "primary", Args: map[string]any{"key": "owned", "value": "replacement", "rating": "public", "allowed_agents": []any{"new"}, "unknown": true}})
				if _, err := a.Write(append(raw, 10)); err != nil {
					t.Fatal(err)
				}
				line, err := bufio.NewReader(a).ReadBytes(10)
				if err != nil {
					t.Fatal(err)
				}
				a.Close()
				<-done
				var reply Response
				if err := json.Unmarshal(line, &reply); err != nil {
					t.Fatal(err)
				}
				after, _ := json.Marshal(k.ConfigCenter().ListEntries())
				effect := !bytes.Equal(before, after)
				if reply.Type != RespError || effect || p.CallCount() != 0 || (gate == "canceled" && !strings.Contains(reply.Error, "context canceled")) {
					t.Errorf("EXPECTED:%s rejects before effects ACTUAL:type=%s error=%q effect=%v", gate, reply.Type, reply.Error, effect)
				}
			})
		}
	}
}
func TestConfigCenterTypedNativeNinePolicyAndSignatures(t *testing.T) {
	type signature struct {
		input, output reflect.Type
		read          bool
	}
	expected := map[string]signature{
		CmdConfigCenterGet: {reflect.TypeFor[appconfigcenter.KeyRequest](), reflect.TypeFor[appconfigcenter.GetOutput](), true}, CmdConfigCenterList: {reflect.TypeFor[appconfigcenter.ListRequest](), reflect.TypeFor[appconfigcenter.ListOutput](), true}, CmdConfigCenterAccessLog: {reflect.TypeFor[appconfigcenter.AccessLogRequest](), reflect.TypeFor[appconfigcenter.AccessLogOutput](), true}, CmdConfigCenterAudit: {reflect.TypeFor[appconfigcenter.SinceRequest](), reflect.TypeFor[appconfigcenter.AuditOutput](), true}, CmdConfigCenterHealth: {reflect.TypeFor[appconfigcenter.HealthInput](), reflect.TypeFor[appconfigcenter.HealthOutput](), true}, CmdConfigCenterSet: {reflect.TypeFor[appconfigcenter.SetRequest](), reflect.TypeFor[appconfigcenter.SetOutput](), false}, CmdConfigCenterDelete: {reflect.TypeFor[appconfigcenter.KeyRequest](), reflect.TypeFor[appconfigcenter.DeleteOutput](), false}, CmdConfigCenterSetRating: {reflect.TypeFor[appconfigcenter.SetRatingRequest](), reflect.TypeFor[appconfigcenter.SetRatingOutput](), false}, CmdConfigCenterSetAccess: {reflect.TypeFor[appconfigcenter.SetAccessRequest](), reflect.TypeFor[appconfigcenter.SetAccessOutput](), false}}

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
	if len(seen) != 9 || len(configCenterOperations) != 9 {
		t.Fatal(seen)
	}
	for _, n := range seen {
		if n != 1 {
			t.Fatal(seen)
		}
	}
}

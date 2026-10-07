// SPDX-License-Identifier: MIT
package controlplane

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	appchannels "github.com/agezt/agezt/kernel/app/channels"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/creds"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/settings"
	"github.com/agezt/agezt/plugins/builtinchannels"
	"github.com/agezt/agezt/plugins/providers/mock"
	"net"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestChannelAccountNativeTypedMetadata(t *testing.T) {
	if len(channelAccountOperations) != 2 {
		t.Fatal(channelAccountOperations)
	}
	seen := map[string]int{}
	for _, op := range registeredAppOperations() {
		s := op.Spec()
		if s.Name != CmdChannelAccountSet && s.Name != CmdChannelAccountRemove {
			continue
		}
		seen[s.Name]++
		wire := commandRegistry[s.Name]
		input, output, path := reflect.TypeFor[appchannels.SetAccountRequest](), reflect.TypeFor[appchannels.SetAccountOutput](), "/api/channel/account/set"
		if s.Name == CmdChannelAccountRemove {
			input, output, path = reflect.TypeFor[appchannels.RemoveAccountRequest](), reflect.TypeFor[appchannels.RemoveAccountOutput](), "/api/channel/account/remove"
		}
		if !wire.AppOwned || wire.ReadOnly || wire.TenantAllowed || wire.TenantRouted || wire.Streaming != StreamNone || s.Authz != opapi.PrimaryOnly || s.Tenancy != opapi.Primary || s.HTTP.Method != "POST" || s.HTTP.Path != path || s.Input != input || s.Output != output {
			t.Fatal(s, wire)
		}
	}
	if len(seen) != 2 || seen[CmdChannelAccountSet] != 1 || seen[CmdChannelAccountRemove] != 1 {
		t.Fatal(seen)
	}
}
func TestChannelAccountNativeMandatoryAuditAndCanceledAdmission(t *testing.T) {
	for _, command := range []string{CmdChannelAccountSet, CmdChannelAccountRemove} {
		for _, gate := range []string{"closed-journal", "canceled"} {
			t.Run(command+"/"+gate, func(t *testing.T) {
				dir := t.TempDir()
				p := mock.New()
				k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: p})
				if err != nil {
					t.Fatal(err)
				}
				defer k.Close()
				builtinchannels.RegisterAll()
				t.Setenv(creds.AutoEncryptEnvVar, "off")
				t.Setenv(creds.PassphraseEnvVar, "")
				store := settings.NewStore(dir)
				store.Set("AGEZT_EMAIL_SMTP_ADDR#work", "before")
				if err := store.Save(); err != nil {
					t.Fatal(err)
				}
				vault := creds.NewStore(dir)
				if err := vault.Set("AGEZT_EMAIL_PASSWORD#work", "owned-before"); err != nil {
					t.Fatal(err)
				}
				if err := vault.Save(); err != nil {
					t.Fatal(err)
				}
				configBefore, err := os.ReadFile(store.Path)
				if err != nil {
					t.Fatal(err)
				}
				vaultBefore, err := os.ReadFile(vault.Path)
				if err != nil {
					t.Fatal(err)
				}
				s := NewServer(k, dir)
				s.token = "primary"
				args := map[string]any{"kind": "email", "label": "work", "name": "AGEZT_EMAIL_SMTP_ADDR", "value": "after"}
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
				configAfter, _ := os.ReadFile(store.Path)
				vaultAfter, _ := os.ReadFile(vault.Path)
				effect := !bytes.Equal(configBefore, configAfter) || !bytes.Equal(vaultBefore, vaultAfter)
				if reply.Type != RespError || effect || p.CallCount() != 0 || (gate == "canceled" && !strings.Contains(reply.Error, "context canceled")) {
					t.Errorf("EXPECTED:%s rejects before effects ACTUAL:type=%s error=%q effect=%v", gate, reply.Type, reply.Error, effect)
				}
			})
		}
	}
}

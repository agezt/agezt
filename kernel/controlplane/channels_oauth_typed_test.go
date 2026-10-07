// SPDX-License-Identifier: MIT
package controlplane

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"github.com/agezt/agezt/kernel/app"
	appchannels "github.com/agezt/agezt/kernel/app/channels"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/creds"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
	"net"
	"os"
	"reflect"
	"testing"
	"time"
)

func TestChannelOAuthNativeTypedMetadata(t *testing.T) {
	if len(channelOAuthOperations) != 3 {
		t.Fatal(channelOAuthOperations)
	}
	seen := map[string]int{}
	inputs := map[string]reflect.Type{CmdChannelOAuthStart: reflect.TypeFor[appchannels.OAuthStartRequest](), CmdChannelOAuthCallback: reflect.TypeFor[appchannels.OAuthCallbackRequest](), CmdChannelOAuthStatus: reflect.TypeFor[appchannels.OAuthStatusRequest]()}
	outputs := map[string]reflect.Type{CmdChannelOAuthStart: reflect.TypeFor[appchannels.OAuthStartOutput](), CmdChannelOAuthCallback: reflect.TypeFor[appchannels.OAuthCallbackOutput](), CmdChannelOAuthStatus: reflect.TypeFor[appchannels.OAuthStatusOutput]()}
	for _, op := range registeredAppOperations() {
		s := op.Spec()
		if _, ok := inputs[s.Name]; !ok {
			continue
		}
		seen[s.Name]++
		wire := commandRegistry[s.Name]
		if !wire.AppOwned || wire.ReadOnly != (s.Name == CmdChannelOAuthStatus) || wire.TenantAllowed || wire.TenantRouted || wire.Streaming != StreamNone || s.Authz != opapi.PrimaryOnly || s.Tenancy != opapi.Primary || s.Input != inputs[s.Name] || s.Output != outputs[s.Name] {
			t.Fatal(s, wire)
		}
	}
	if len(seen) != 3 {
		t.Fatal(seen)
	}
	for _, n := range seen {
		if n != 1 {
			t.Fatal(seen)
		}
	}
}
func TestChannelOAuthNativeAuditAndCanceledAdmissionBeforeService(t *testing.T) {
	for _, command := range []string{CmdChannelOAuthStart, CmdChannelOAuthCallback, CmdChannelOAuthStatus} {
		for _, gate := range []string{"closed-journal", "canceled"} {
			t.Run(command+"/"+gate, func(t *testing.T) {
				t.Setenv(creds.AutoEncryptEnvVar, "off")
				t.Setenv(creds.PassphraseEnvVar, "")
				root := t.TempDir()
				p := mock.New()
				k, err := runtime.Open(runtime.Config{BaseDir: root, Provider: p})
				if err != nil {
					t.Fatal(err)
				}
				defer k.Close()
				s := NewServer(k, root)
				s.token = "primary"
				_ = s.channelOAuth()
				s.channelOAuthState.Put("owned", appchannels.OAuthFlow{Kind: "slack", Label: "work", Status: "pending", TokenEnv: "AGEZT_OWNED_TOKEN", TokenURL: "https://owned.example/token"}, time.Now())
				vault := creds.NewStore(root)
				if err := vault.Set("AGEZT_OWNED_TOKEN#work", "owned-before"); err != nil {
					t.Fatal(err)
				}
				if err := vault.Save(); err != nil {
					t.Fatal(err)
				}
				before, err := os.ReadFile(vault.Path)
				if err != nil {
					t.Fatal(err)
				}
				providers := 0
				ops, err := appchannels.OAuthOperations(func(ctx context.Context) *appchannels.OAuth {
					providers++
					if ctx.Value(systemHostKey{}).(*Server) != s {
						t.Fatal("selected server lost")
					}
					return s.channelOAuth()
				})
				if err != nil {
					t.Fatal(err)
				}
				s.operationOnce.Do(func() {
					s.operations, s.operationErr = app.NewDispatcher(ops, app.Dependencies{Auth: appAuthenticator{s}, Router: appTenantRouter{s}, Audit: appAuditor{}})
				})
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
				raw, _ := json.Marshal(Request{ID: "owned", Cmd: command, Token: "primary", Args: map[string]any{"kind": "slack", "client_id": "id", "client_secret": "secret", "redirect_uri": "https://owned.example/cb", "code": "code", "state": "owned"}})
				if _, err := a.Write(append(raw, 10)); err != nil {
					t.Fatal(err)
				}
				line, err := bufio.NewReader(a).ReadBytes(10)
				a.Close()
				<-done
				if err != nil {
					t.Fatal(err)
				}
				var reply Response
				if json.Unmarshal(line, &reply) != nil {
					t.Fatal(string(line))
				}
				after, _ := os.ReadFile(vault.Path)
				flow, _ := s.channelOAuthState.Flow("owned")
				denied := command != CmdChannelOAuthStatus || gate == "canceled"
				if denied {
					if reply.Type != RespError || providers != 0 || gate == "canceled" && reply.Error != "context canceled" {
						t.Fatal(reply, providers)
					}
				} else if reply.Type != RespResult || providers != 1 || len(reply.Result) != 4 || reply.Result["status"] != "pending" {
					t.Fatal("read required audit", reply, providers)
				}
				if !bytes.Equal(before, after) || flow.Status != "pending" || p.CallCount() != 0 {
					t.Fatal("admission changed vault/flow/model")
				}
			})
		}
	}
}

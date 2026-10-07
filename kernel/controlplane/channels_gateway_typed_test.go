// SPDX-License-Identifier: MIT
package controlplane

import (
	"bufio"
	"context"
	"encoding/json"
	"github.com/agezt/agezt/kernel/app"
	appchannels "github.com/agezt/agezt/kernel/app/channels"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
	"net"
	"reflect"
	"testing"
	"time"
)

func TestChannelGatewayNativeTypedMetadata(t *testing.T) {
	if len(channelGatewayOperations) != 2 {
		t.Fatal(channelGatewayOperations)
	}
	seen := map[string]int{}
	for _, op := range registeredAppOperations() {
		s := op.Spec()
		if s.Name != CmdWhatsAppGatewayStatus && s.Name != CmdWhatsAppGatewayQR {
			continue
		}
		seen[s.Name]++
		wire := commandRegistry[s.Name]
		output, path := reflect.TypeFor[appchannels.GatewayStatusOutput](), "/api/whatsappgw/status"
		if s.Name == CmdWhatsAppGatewayQR {
			output, path = reflect.TypeFor[appchannels.GatewayQROutput](), "/api/whatsappgw/qr"
		}
		if !wire.AppOwned || !wire.ReadOnly || wire.TenantAllowed || wire.TenantRouted || wire.Streaming != StreamNone || s.Authz != opapi.PrimaryOnly || s.Tenancy != opapi.Primary || s.HTTP.Method != "POST" || s.HTTP.Path != path || s.Input != reflect.TypeFor[appchannels.GatewayRequest]() || s.Output != output {
			t.Fatal(s, wire)
		}
	}
	if len(seen) != 2 {
		t.Fatal(seen)
	}
	for _, n := range seen {
		if n != 1 {
			t.Fatal(seen)
		}
	}
}
func TestChannelGatewayNativeCanceledBeforeHTTPAndReadWithoutAudit(t *testing.T) {
	for _, command := range []string{CmdWhatsAppGatewayStatus, CmdWhatsAppGatewayQR} {
		for _, gate := range []string{"canceled", "closed-journal"} {
			t.Run(command+"/"+gate, func(t *testing.T) {
				p := mock.New()
				dir := t.TempDir()
				k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: p})
				if err != nil {
					t.Fatal(err)
				}
				defer k.Close()
				s := NewServer(k, dir)
				s.token = "primary"
				providers, gets := 0, 0
				ops, err := appchannels.GatewayOperations(func(ctx context.Context) *appchannels.Gateway {
					providers++
					if ctx.Value(systemHostKey{}).(*Server) != s {
						t.Fatal("selected server lost")
					}
					return appchannels.NewGateway(func(string, string, string, int64) ([]byte, int, string, error) {
						gets++
						return []byte(`{"status":"WORKING","base64":"raw"}`), 200, "application/json", nil
					})
				})
				if err != nil {
					t.Fatal(err)
				}
				s.operationOnce.Do(func() {
					s.operations, s.operationErr = app.NewDispatcher(ops, app.Dependencies{Auth: appAuthenticator{s}, Router: appTenantRouter{s}})
				})
				head, hash := k.Journal().Head()
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
				raw, _ := json.Marshal(Request{ID: "owned", Cmd: command, Token: "primary", Args: map[string]any{"url": "http://owned.example", "unknown": true}})
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
				if gate == "canceled" {
					if reply.Type != RespError || reply.Error != "context canceled" || providers != 0 || gets != 0 {
						t.Fatal(reply, providers, gets)
					}
				} else if reply.Type != RespResult || reply.Result["ok"] != true || providers != 1 || gets != 1 {
					t.Fatal("read required audit", reply, providers, gets)
				}
				after, afterHash := k.Journal().Head()
				if head != after || hash != afterHash || p.CallCount() != 0 {
					t.Fatal("read changed audit/model")
				}
			})
		}
	}
}

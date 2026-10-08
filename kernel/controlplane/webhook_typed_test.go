// SPDX-License-Identifier: MIT
package controlplane

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"reflect"
	"strings"
	"testing"
	"time"

	appwebhook "github.com/agezt/agezt/kernel/app/webhook"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func TestWebhookNativeTypedExit(t *testing.T) {
	expected := map[string][2]reflect.Type{CmdWebhookLog: {reflect.TypeFor[appwebhook.LogRequest](), reflect.TypeFor[appwebhook.LogOutput]()}, CmdWebhookStats: {reflect.TypeFor[appwebhook.StatsRequest](), reflect.TypeFor[appwebhook.StatsOutput]()}}
	seen := map[string]int{}
	for _, op := range registeredAppOperations() {
		s := op.Spec()
		sig, ok := expected[s.Name]
		if !ok {
			continue
		}
		wire, found := commandRegistry[s.Name]
		if !found || !wire.AppOwned || !wire.ReadOnly || !wire.TenantAllowed || !wire.TenantRouted || wire.Streaming != StreamNone || s.Input != sig[0] || s.Output != sig[1] || s.Authz != opapi.OwnTenant || s.Tenancy != opapi.CallerTenant || s.Stream != opapi.StreamNone || !s.AllowUnknownInput || s.Emission != nil || len(s.EmissionSchema) != 0 {
			t.Fatal(s, wire)
		}
		seen[s.Name]++
	}
	if len(webhookOperations) != 2 || len(seen) != 2 || seen[CmdWebhookLog] != 1 || seen[CmdWebhookStats] != 1 {
		t.Fatal("webhook native aggregate", seen)
	}
}

type exactWebhookReader struct{}

func (exactWebhookReader) Range(visit func(*event.Event) error) error {
	return visit(&event.Event{Kind: event.KindWebhookDelivered, Seq: 9007199254740993, TSUnixMS: 9007199254740995, Payload: json.RawMessage(`{"url":"owned","status":0}`)})
}

func TestWebhookNativeExactIntegerTerminal(t *testing.T) {
	original := webhookOperations
	ops, err := appwebhook.Operations(func(context.Context) *appwebhook.Observability {
		return appwebhook.NewObservability(exactWebhookReader{}, nil)
	})
	if err != nil {
		t.Fatal(err)
	}
	webhookOperations = ops
	t.Cleanup(func() { webhookOperations = original })
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	s := NewServer(k, t.TempDir())
	s.token = "primary"
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	a.SetDeadline(time.Now().Add(3 * time.Second))
	done := make(chan struct{})
	go func() { defer close(done); s.handleConn(context.Background(), b) }()
	raw, _ := json.Marshal(Request{ID: "owned", Cmd: CmdWebhookLog, Token: "primary"})
	if _, err := a.Write(append(raw, 10)); err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(a).ReadBytes(10)
	a.Close()
	<-done
	if err != nil || !strings.Contains(string(line), `"seq":9007199254740993`) || !strings.Contains(string(line), `"ts_unix_ms":9007199254740995`) || !strings.Contains(string(line), `"status":0`) || strings.Contains(string(line), `"error":`) {
		t.Fatal(string(line), err)
	}
}

// SPDX-License-Identifier: MIT
package controlplane

import (
	"bufio"
	"context"
	"encoding/json"
	appconfig "github.com/agezt/agezt/kernel/app/config"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
	"net"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestConfigTypedNativeCanceledAdmissionRejectsRead(t *testing.T) {
	dir := t.TempDir()
	p := mock.New()
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: p, Plugins: []runtime.PluginInfo{{Prefix: "owned", Path: "never-executed"}}})
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	s := NewServer(k, dir)
	s.token = "primary"
	head, hash := k.Journal().Head()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	a.SetDeadline(time.Now().Add(3 * time.Second))
	done := make(chan struct{})
	go func() { defer close(done); s.handleConn(ctx, b) }()
	raw, _ := json.Marshal(Request{ID: "owned", Cmd: CmdConfig, Token: "primary", Args: map[string]any{"unknown": true, "tenant": "spoof"}})
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
	after, afterHash := k.Journal().Head()
	if reply.Type != RespError || !strings.Contains(reply.Error, "context canceled") || head != after || hash != afterHash || p.CallCount() != 0 {
		t.Fatalf("EXPECTED:canceled admission before config read ACTUAL:reply=%+v", reply)
	}
}

func TestConfigTypedNativeOnePolicyAndSignature(t *testing.T) {
	seen := 0
	for _, op := range registeredAppOperations() {
		sp := op.Spec()
		if sp.Name != CmdConfig {
			continue
		}
		wire, ok := commandRegistry[sp.Name]
		if !ok || !wire.AppOwned || !wire.ReadOnly || wire.TenantAllowed || wire.TenantRouted || wire.Streaming != StreamNone || !sp.ReadOnly || sp.Authz != opapi.PrimaryOnly || sp.Tenancy != opapi.Primary || sp.Stream != opapi.StreamNone || !sp.AllowUnknownInput || sp.Input != reflect.TypeFor[appconfig.ShowInput]() || sp.Output != reflect.TypeFor[appconfig.ShowOutput]() {
			t.Fatal(sp, wire)
		}
		seen++
	}
	if seen != 1 || len(configReadOperations) != 1 {
		t.Fatal(seen)
	}
}

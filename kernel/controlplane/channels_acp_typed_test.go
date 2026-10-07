// SPDX-License-Identifier: MIT
package controlplane

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/app"
	appchannels "github.com/agezt/agezt/kernel/app/channels"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func TestChannelACPNativeMetadataComesFromTypedSpec(t *testing.T) {
	if len(acpInventoryOperations) != 1 {
		t.Fatal(acpInventoryOperations)
	}
	spec := acpInventoryOperations[0].Spec()
	wire := commandRegistry[CmdACPAgents]
	if !wire.AppOwned || !wire.ReadOnly || wire.TenantAllowed || wire.TenantRouted || wire.Streaming != StreamNone || spec.Input != reflect.TypeFor[appchannels.ACPInput]() || spec.Output != reflect.TypeFor[appchannels.ACPOutput]() || spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary || spec.HTTP.Method != "GET" || spec.HTTP.Path != "/api/acp/agents" {
		t.Fatal(spec, wire)
	}
}

func TestChannelACPNativeTypedIntegerTerminalAndCanceledAdmission(t *testing.T) {
	for _, count := range []int64{9007199254740993, 9223372036854775807} {
		p := mock.New()
		dir := t.TempDir()
		k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: p})
		if err != nil {
			t.Fatal(err)
		}
		defer k.Close()
		s := NewServer(k, dir)
		s.token = "primary"
		providerCalls, discoveryCalls := 0, 0
		ops, err := appchannels.ACPInventoryOperations(func(context.Context) *appchannels.ACPInventory {
			providerCalls++
			return appchannels.NewACPInventory(func() string { return "owned" }, func(context.Context, string, bool) appchannels.ACPOutput {
				discoveryCalls++
				return appchannels.ACPOutput{OS: "owned", Agents: nil, RegisteredCount: int(count), RegistryError: "owned registry unavailable"}
			})
		})
		if err != nil {
			t.Fatal(err)
		}
		s.operationOnce.Do(func() {
			s.operations, s.operationErr = app.NewDispatcher(ops, app.Dependencies{Auth: appAuthenticator{s}, Router: appTenantRouter{s}})
		})
		call := func(ctx context.Context) []byte {
			a, b := net.Pipe()
			a.SetDeadline(time.Now().Add(3 * time.Second))
			done := make(chan struct{})
			go func() { defer close(done); s.handleConn(ctx, b) }()
			raw, _ := json.Marshal(Request{ID: "owned", Cmd: CmdACPAgents, Token: "primary", Args: map[string]any{"force": true, "unknown": 42}})
			if _, err := a.Write(append(raw, 10)); err != nil {
				t.Fatal(err)
			}
			line, err := bufio.NewReader(a).ReadBytes(10)
			a.Close()
			<-done
			if err != nil {
				t.Fatal(err)
			}
			return line
		}
		head, hash := k.Journal().Head()
		line := call(context.Background())
		var reply struct {
			Type   string `json:"type"`
			Result struct {
				Count  json.Number `json:"registered_count"`
				Agents []any       `json:"agents"`
				Error  string      `json:"registry_error"`
			} `json:"result"`
		}
		if err := json.Unmarshal(line, &reply); err != nil || reply.Type != RespResult || reply.Result.Count.String() != strconv.FormatInt(count, 10) || reply.Result.Agents != nil || reply.Result.Error != "owned registry unavailable" || providerCalls != 1 || discoveryCalls != 1 {
			t.Fatal(string(line), err, providerCalls, discoveryCalls)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		line = call(ctx)
		var canceled Response
		json.Unmarshal(line, &canceled)
		if canceled.Type != RespError || canceled.Error != "context canceled" || providerCalls != 1 || discoveryCalls != 1 {
			t.Fatal(string(line), providerCalls, discoveryCalls)
		}
		after, afterHash := k.Journal().Head()
		if head != after || hash != afterHash || p.CallCount() != 0 {
			t.Fatal("read/canceled ACP wrote audit/provider")
		}
	}
}

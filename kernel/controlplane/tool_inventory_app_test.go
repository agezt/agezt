// SPDX-License-Identifier: MIT
package controlplane

import (
	"bufio"
	"context"
	"encoding/json"
	apptools "github.com/agezt/agezt/kernel/app/tools"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/platform/schema"
	"net"
	"reflect"
	"testing"
	"time"
)

func TestToolInventoryNativeMetadataComesFromTypedPrimaryRead(t *testing.T) {
	seen := 0
	for _, operation := range registeredAppOperations() {
		spec := operation.Spec()
		if spec.Name != CmdToolList {
			continue
		}
		wire, ok := commandRegistry[spec.Name]
		if !ok || !wire.AppOwned || !wire.ReadOnly || wire.TenantAllowed || wire.TenantRouted || wire.Streaming != StreamNone || !spec.ReadOnly || spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary || spec.Stream != opapi.StreamNone || spec.Input != reflect.TypeFor[apptools.InventoryInput]() || spec.Output != reflect.TypeFor[apptools.InventoryOutput]() || len(spec.InputSchema) == 0 || len(spec.OutputSchema) == 0 || !spec.AllowUnknownInput {
			t.Fatal(spec, wire)
		}
		seen++
	}
	if seen != 1 || len(toolInventoryOperations) != 1 {
		t.Fatal(seen, len(toolInventoryOperations))
	}
}
func TestToolInventoryAppNativeEmptyUnknownArgsAndUnauditedRead(t *testing.T) {
	k, s, _, provider := pulseAppFixture(t)
	head, hash := k.Journal().Head()
	out := callAppHost(t, s, Request{ID: "inventory", Cmd: CmdToolList, Token: "primary", Args: map[string]any{"unused": true, "tenant": "spoof"}})
	if len(out) != 1 || out[0].Type != RespResult || !reflect.DeepEqual(out[0].Result, map[string]any{"tools": []any{}, "count": float64(0)}) {
		t.Fatal(out)
	}
	raw, _ := json.Marshal(out[0].Result)
	if err := schema.ValidateJSON(toolInventoryOperations[0].Spec().OutputSchema, raw); err != nil {
		t.Fatal(err)
	}
	after, afterHash := k.Journal().Head()
	if head != after || hash != afterHash || provider.CallCount() != 0 {
		t.Fatal(head, after, provider.CallCount())
	}
}
func TestToolInventoryAppNativeCanceledAdmissionDoesNotReturnInventory(t *testing.T) {
	_, s, _, provider := pulseAppFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client, conn := net.Pipe()
	defer client.Close()
	done := make(chan struct{})
	go func() { defer close(done); s.handleConn(ctx, conn) }()
	client.SetDeadline(time.Now().Add(time.Second))
	raw, _ := json.Marshal(Request{ID: "canceled", Cmd: CmdToolList, Token: "primary"})
	if _, err := client.Write(append(raw, 10)); err != nil {
		t.Fatal(err)
	}
	scanner := bufio.NewScanner(client)
	var responses []Response
	for scanner.Scan() {
		var out Response
		_ = json.Unmarshal(scanner.Bytes(), &out)
		responses = append(responses, out)
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	<-done
	if len(responses) != 1 || responses[0].Type != RespError || responses[0].Error != "context canceled" || provider.CallCount() != 0 {
		t.Fatal(responses, provider.CallCount())
	}
}

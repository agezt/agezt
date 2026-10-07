// SPDX-License-Identifier: MIT
package controlplane

import (
	"bufio"
	"context"
	"encoding/json"
	appautonomy "github.com/agezt/agezt/kernel/app/autonomy"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/platform/schema"
	"net"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestAutonomyNativeMetadataComesFromOneTypedPrimaryRead(t *testing.T) {
	seen := 0
	for _, operation := range registeredAppOperations() {
		spec := operation.Spec()
		if !strings.HasPrefix(spec.Name, "autonomy_") {
			continue
		}
		wire, ok := commandRegistry[spec.Name]
		if spec.Name != CmdAutonomyFeed || !ok || !wire.AppOwned || !wire.ReadOnly || wire.TenantAllowed || wire.TenantRouted || wire.Streaming != StreamNone || !spec.ReadOnly || spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary || spec.Stream != opapi.StreamNone || spec.Input != reflect.TypeFor[appautonomy.FeedRequest]() || spec.Output != reflect.TypeFor[appautonomy.FeedOutput]() || len(spec.InputSchema) == 0 || len(spec.OutputSchema) == 0 || !spec.AllowUnknownInput {
			t.Fatal(spec, wire)
		}
		seen++
	}
	if seen != 1 || len(autonomyOperations) != 1 {
		t.Fatal(seen, len(autonomyOperations))
	}
}
func TestAutonomyAppNativeLimitsReadsUnauditedAndRowSchema(t *testing.T) {
	k, s, _, provider := pulseAppFixture(t)
	for i := 0; i < 75; i++ {
		if _, err := k.Bus().Publish(event.Spec{Subject: "owned.one", Kind: event.KindScheduleFired, Actor: "fixture"}); err != nil {
			t.Fatal(err)
		}
	}
	head, hash := k.Journal().Head()
	for _, tc := range []struct {
		args  map[string]any
		count int
	}{{nil, 60}, {map[string]any{"limit": "1"}, 60}, {map[string]any{"limit": nil}, 60}, {map[string]any{"limit": false}, 60}, {map[string]any{"limit": float64(0)}, 1}, {map[string]any{"limit": float64(-10)}, 1}, {map[string]any{"limit": float64(1.9)}, 1}, {map[string]any{"limit": float64(500)}, 75}, {map[string]any{"limit": float64(1), "unused": true, "tenant": "spoof"}, 1}} {
		out := callAppHost(t, s, Request{ID: "feed", Cmd: CmdAutonomyFeed, Token: "primary", Args: tc.args})
		if len(out) != 1 || out[0].Type != RespResult || out[0].Result["count"] != float64(tc.count) {
			t.Fatal(tc, out)
		}
		raw, _ := json.Marshal(out[0].Result)
		if err := schema.ValidateJSON(autonomyOperations[0].Spec().OutputSchema, raw); err != nil {
			t.Fatal(err)
		}
		item := out[0].Result["items"].([]any)[0].(map[string]any)
		if item["seq"] != float64(head) || item["correlation_id"] != "" {
			t.Fatal(item, head)
		}
	}
	after, afterHash := k.Journal().Head()
	if head != after || hash != afterHash || provider.CallCount() != 0 {
		t.Fatal(head, after, provider.CallCount())
	}
}
func TestAutonomyAppNativeCanceledAdmissionDoesNotReturnFeed(t *testing.T) {
	k, s, _, provider := pulseAppFixture(t)
	_, err := k.Bus().Publish(event.Spec{Subject: "owned", Kind: event.KindScheduleFired, Actor: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	client, conn := net.Pipe()
	defer client.Close()
	done := make(chan struct{})
	go func() { defer close(done); s.handleConn(ctx, conn) }()
	client.SetDeadline(time.Now().Add(time.Second))
	raw, _ := json.Marshal(Request{ID: "canceled", Cmd: CmdAutonomyFeed, Token: "primary"})
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

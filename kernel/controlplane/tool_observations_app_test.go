// SPDX-License-Identifier: MIT
package controlplane

import (
	"bufio"
	"context"
	"encoding/json"
	apptools "github.com/agezt/agezt/kernel/app/tools"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"net"
	"reflect"
	"testing"
	"time"
)

func TestToolNativeThreeOperationPoliciesAndTypedObservationSpecs(t *testing.T) {
	want := map[string]bool{CmdToolList: false, CmdToolLog: true, CmdToolStats: true}
	seen := map[string]bool{}
	for _, operation := range registeredAppOperations() {
		spec := operation.Spec()
		tenant, known := want[spec.Name]
		if !known {
			continue
		}
		wire, ok := commandRegistry[spec.Name]
		auth, tenancy := opapi.PrimaryOnly, opapi.Primary
		if tenant {
			auth, tenancy = opapi.OwnTenant, opapi.CallerTenant
		}
		if seen[spec.Name] || !ok || !wire.AppOwned || !wire.ReadOnly || wire.TenantAllowed != tenant || wire.TenantRouted != tenant || wire.Streaming != StreamNone || !spec.ReadOnly || spec.Authz != auth || spec.Tenancy != tenancy || spec.Stream != opapi.StreamNone || !spec.AllowUnknownInput {
			t.Fatal(spec, wire)
		}
		if tenant && spec.Input != reflect.TypeFor[apptools.ObservationRequest]() {
			t.Fatal(spec)
		}
		if spec.Name == CmdToolLog && spec.Output != reflect.TypeFor[apptools.LogOutput]() {
			t.Fatal(spec)
		}
		if spec.Name == CmdToolStats && spec.Output != reflect.TypeFor[apptools.StatsOutput]() {
			t.Fatal(spec)
		}
		seen[spec.Name] = true
	}
	if len(seen) != 3 || len(toolObservationOperations) != 2 {
		t.Fatal(seen, len(toolObservationOperations))
	}
}
func TestToolObservationsNativeCanceledAdmissionReturnsNoRows(t *testing.T) {
	for _, cmd := range []string{CmdToolLog, CmdToolStats} {
		_, s, _, provider := pulseAppFixture(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		client, conn := net.Pipe()
		done := make(chan struct{})
		go func() { defer close(done); s.handleConn(ctx, conn) }()
		client.SetDeadline(time.Now().Add(time.Second))
		raw, _ := json.Marshal(Request{ID: cmd, Cmd: cmd, Token: "primary"})
		if _, err := client.Write(append(raw, 10)); err != nil {
			t.Fatal(err)
		}
		scanner := bufio.NewScanner(client)
		var responses []Response
		for scanner.Scan() {
			var response Response
			_ = json.Unmarshal(scanner.Bytes(), &response)
			responses = append(responses, response)
		}
		client.Close()
		<-done
		if err := scanner.Err(); err != nil {
			t.Fatal(err)
		}
		if len(responses) != 1 || responses[0].Type != RespError || responses[0].Error != "context canceled" || provider.CallCount() != 0 {
			t.Fatal(cmd, responses, provider.CallCount())
		}
	}
}

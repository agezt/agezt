// SPDX-License-Identifier: MIT
package controlplane

import (
	"bufio"
	"context"
	"encoding/json"
	apptools "github.com/agezt/agezt/kernel/app/tools"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/toolbox"
	"net"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestToolboxReadsNativeCanceledAdmissionRejectsBeforeHost(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Chdir(t.TempDir())
	for _, cmd := range []string{CmdToolboxDetect, CmdToolboxOutdated} {
		t.Run(cmd, func(t *testing.T) {
			k, s, _, p := pulseAppFixture(t)
			head, hash := k.Journal().Head()
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			client, conn := net.Pipe()
			defer client.Close()
			defer conn.Close()
			done := make(chan struct{})
			go func() { defer close(done); s.handleConn(ctx, conn) }()
			client.SetDeadline(time.Now().Add(5 * time.Second))
			raw, _ := json.Marshal(Request{ID: cmd, Cmd: cmd, Token: "primary", Args: map[string]any{"unknown": true}})
			if _, err := client.Write(append(raw, 10)); err != nil {
				t.Fatal(err)
			}
			line, err := bufio.NewReader(client).ReadBytes(10)
			if err != nil {
				t.Fatal(err)
			}
			client.Close()
			<-done
			var reply Response
			if err := json.Unmarshal(line, &reply); err != nil {
				t.Fatal(err)
			}
			after, afterHash := k.Journal().Head()
			if reply.Type != RespError || !strings.Contains(reply.Error, "context canceled") || head != after || hash != afterHash || p.CallCount() != 0 {
				t.Fatalf("EXPECTED:canceled read admission ACTUAL:type=%s error=%q seq=%d/%d", reply.Type, reply.Error, head, after)
			}
		})
	}
}

func TestToolboxReadNativePoliciesAndTypesComeFromTwoAppSpecs(t *testing.T) {
	want := map[string]reflect.Type{CmdToolboxDetect: reflect.TypeFor[toolbox.Inventory](), CmdToolboxOutdated: reflect.TypeFor[apptools.ToolboxOutdatedOutput]()}
	seen := map[string]bool{}
	for _, op := range registeredAppOperations() {
		spec := op.Spec()
		output, known := want[spec.Name]
		if !known {
			continue
		}
		input := reflect.TypeFor[apptools.ToolboxDetectInput]()
		if spec.Name == CmdToolboxOutdated {
			input = reflect.TypeFor[apptools.ToolboxOutdatedInput]()
		}
		wire, ok := commandRegistry[spec.Name]
		if seen[spec.Name] || !ok || !wire.AppOwned || !wire.ReadOnly || wire.TenantAllowed || wire.TenantRouted || wire.Streaming != StreamNone || !spec.ReadOnly || spec.Authz != opapi.PrimaryOnly || spec.Tenancy != opapi.Primary || spec.Stream != opapi.StreamNone || !spec.AllowUnknownInput || spec.Input != input || spec.Output != output {
			t.Fatal(spec, wire)
		}
		seen[spec.Name] = true
	}
	if len(seen) != 2 || len(toolboxReadOperations) != 2 {
		t.Fatal(seen)
	}
}

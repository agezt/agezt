// SPDX-License-Identifier: MIT
package controlplane

import (
	"bufio"
	"context"
	"encoding/json"
	"github.com/agezt/agezt/kernel/app"
	appautonomy "github.com/agezt/agezt/kernel/app/autonomy"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"net"
	"testing"
	"time"
)

func TestAppHostNativeTypedLargeIntegerWireExact(t *testing.T) {
	for _, want := range []int64{9007199254740993, 9223372036854775807} {
		_, s, _, _ := pulseAppFixture(t)
		const name = "zz_integer_wire_proof"
		operation, err := app.NewOperation(opapi.Spec{Name: name, ReadOnly: true}, func(context.Context, struct{}) (appautonomy.FeedOutput, error) {
			return appautonomy.FeedOutput{Items: []appautonomy.FeedItem{{Seq: want, TSUnixMS: want}}, Count: 1}, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		wire, err := appCommandSpec(operation)
		if err != nil {
			t.Fatal(err)
		}
		old, exists := commandRegistry[name]
		commandRegistry[name] = wire
		t.Cleanup(func() {
			if exists {
				commandRegistry[name] = old
			} else {
				delete(commandRegistry, name)
			}
		})
		s.operationOnce.Do(func() {
			s.operations, s.operationErr = app.NewDispatcher([]app.Operation{operation}, app.Dependencies{Auth: appAuthenticator{s}, Router: appTenantRouter{s}})
		})
		client, conn := net.Pipe()
		done := make(chan struct{})
		go func() { defer close(done); s.handleConn(context.Background(), conn) }()
		client.SetDeadline(time.Now().Add(time.Second))
		raw, _ := json.Marshal(Request{ID: name, Cmd: name, Token: "primary"})
		if _, err := client.Write(append(raw, 10)); err != nil {
			t.Fatal(err)
		}
		line, err := bufio.NewReader(client).ReadBytes(10)
		client.Close()
		<-done
		if exists {
			commandRegistry[name] = old
		} else {
			delete(commandRegistry, name)
		}
		if err != nil {
			t.Fatal(err)
		}
		var response struct {
			Result struct {
				Items []struct {
					Seq json.Number `json:"seq"`
					TS  json.Number `json:"ts_unix_ms"`
				} `json:"items"`
			} `json:"result"`
		}
		if err := json.Unmarshal(line, &response); err != nil {
			t.Fatal(err)
		}
		if len(response.Result.Items) != 1 {
			t.Fatal(string(line))
		}
		item := response.Result.Items[0]
		seq, seqErr := item.Seq.Int64()
		ts, tsErr := item.TS.Int64()
		if seqErr != nil || tsErr != nil || seq != want || ts != want {
			t.Fatalf("EXPECTED: native integer identity/time preserve %d; ACTUAL: seq=%s ts=%s seqErr=%v tsErr=%v", want, item.Seq, item.TS, seqErr, tsErr)
		}
	}
}

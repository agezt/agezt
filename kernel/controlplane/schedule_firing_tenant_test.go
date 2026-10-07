// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"encoding/json"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
	"strings"
	"testing"
	"time"
)

func TestScheduleFiringSocketReadsRetainSelectedTenantJournalAndRunJoin(t *testing.T) {
	primary, server, owner, dir := startPair(t, mock.New(mock.FinalText("unused")))
	registry := withTenants(t, server, dir)
	token := mustTenant(t, registry, "acme")
	_ = mustTenant(t, registry, "other")
	seed := func(k *runtime.Kernel, id string, spent int64) {
		t.Helper()
		for _, spec := range []event.Spec{
			{Kind: event.KindScheduleFired, Subject: "schedule", Actor: "fixture", CorrelationID: id, Payload: map[string]any{"schedule_id": id, "intent": id}},
			{Kind: event.KindTaskReceived, Subject: "task", Actor: "fixture", CorrelationID: id, Payload: map[string]any{"intent": id}},
			{Kind: event.KindBudgetConsumed, Subject: "budget", Actor: "fixture", CorrelationID: id, Payload: map[string]any{"cost_microcents": spent}},
			{Kind: event.KindTaskCompleted, Subject: "task", Actor: "fixture", CorrelationID: id, Payload: map[string]any{"answer": id}},
		} {
			if _, err := k.Bus().Publish(spec); err != nil {
				t.Fatal(err)
			}
		}
	}
	seed(primary, "primary-private", 10)
	for _, id := range []string{"acme", "other"} {
		handle, err := registry.Acquire(id, time.Now())
		if err != nil {
			t.Fatal(err)
		}
		cost := int64(20)
		if id == "other" {
			cost = 30
		}
		seed(handle.Kernel.(*runtime.Kernel), id+"-private", cost)
	}
	client := tenantClient(t, dir, token)
	for _, read := range []struct {
		client   *controlplane.Client
		args     map[string]any
		identity string
		spent    int
	}{{owner, nil, "primary-private", 10}, {owner, map[string]any{"tenant": "acme"}, "acme-private", 20}, {client, map[string]any{"tenant": "acme"}, "acme-private", 20}} {
		result, err := read.client.Call(context.Background(), controlplane.CmdScheduleFires, read.args)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(result)
		if !strings.Contains(string(raw), read.identity) {
			t.Fatal(result, read.identity)
		}
		for _, identity := range []string{"primary-private", "acme-private", "other-private"} {
			if identity != read.identity && strings.Contains(string(raw), identity) {
				t.Fatal("foreign firing", identity, string(raw))
			}
		}
		rows, _ := result["fires"].([]any)
		if len(rows) != 1 {
			t.Fatal(result)
		}
		row, _ := rows[0].(map[string]any)
		if row["status"] != "completed" || row["answer_preview"] != read.identity || intOf(row["spent_mc"]) != read.spent {
			t.Fatal(row, read)
		}
		result, err = read.client.Call(context.Background(), controlplane.CmdScheduleStats, read.args)
		if err != nil || intOf(result["total"]) != 1 || intOf(result["completed"]) != 1 || intOf(result["spent_microcents"]) != read.spent || intOf(result["schedules"]) != 1 {
			t.Fatal(result, err, read)
		}
	}
	for _, cmd := range []string{controlplane.CmdScheduleFires, controlplane.CmdScheduleStats} {
		if _, err := client.Call(context.Background(), cmd, map[string]any{"tenant": "other"}); err == nil || (!strings.Contains(err.Error(), "forbidden") && !strings.Contains(err.Error(), "unauthorized")) {
			t.Fatal(cmd, err)
		}
	}
	if _, err := client.Call(context.Background(), controlplane.CmdScheduleList, map[string]any{"tenant": "acme"}); err == nil {
		t.Fatal("operator schedule list admitted tenant token")
	}
}

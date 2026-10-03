// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"testing"

	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/resume"
	"github.com/agezt/agezt/kernel/roster"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

func TestRun_AsAgent_ResumeEligibility(t *testing.T) {
	for _, tc := range []struct {
		name, key string
		value     any
		resumable bool
	}{
		{"profile defaults", "", nil, true},
		{"explicit model", "model", "custom-model", false},
		{"same model explicitly", "model", "agent-model", false},
		{"explicit system", "system", "custom-system", false},
		{"same soul explicitly", "system", "agent-soul", false},
		{"explicit empty tools", "tools", []any{}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			prov := mock.New(mock.FinalText("ok"))
			observed := make(chan *resume.Ticket, 1)
			var k *runtime.Kernel
			prov.OnRequest = func(req llm.CompletionRequest) {
				ticket, _, err := k.ResumeStore().Get(req.CorrelationID)
				if err != nil {
					t.Errorf("read ticket: %v", err)
				}
				observed <- ticket
			}
			var c *controlplane.Client
			k, _, c, _ = startPairWithConfig(t, runtime.Config{Provider: prov, ResumeEnabled: true})
			if _, err := k.AddProfile(roster.Profile{Slug: "ops", Soul: "agent-soul", Model: "agent-model"}); err != nil {
				t.Fatal(err)
			}
			args := map[string]any{"agent": "ops", "intent": "do the task"}
			if tc.key != "" {
				args[tc.key] = tc.value
			}
			if _, err := c.Stream(context.Background(), controlplane.CmdRun, args, func(*event.Event) {}); err != nil {
				t.Fatal(err)
			}
			ticket := <-observed
			if ticket == nil || ticket.Resumable != tc.resumable {
				t.Fatalf("ticket=%+v; want Resumable=%v", ticket, tc.resumable)
			}
		})
	}
}

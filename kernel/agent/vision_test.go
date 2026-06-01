// SPDX-License-Identifier: MIT

package agent_test

import (
	"context"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestRun_ImagesReachProvider — LoopConfig.Images is attached to the initial
// user message and reaches the provider's CompletionRequest (M93).
func TestRun_ImagesReachProvider(t *testing.T) {
	var gotImages []string
	prov := mock.New(mock.FinalText("done"))
	prov.OnRequest = func(req agent.CompletionRequest) {
		for _, m := range req.Messages {
			if m.Role == agent.RoleUser {
				gotImages = m.Images
			}
		}
	}
	b, _ := newTestBus(t)
	_, err := agent.Run(context.Background(), agent.LoopConfig{
		Provider: prov, Bus: b, Actor: "agent-test", Model: "mock",
		Images: []string{"a.png", "b.jpg"},
	}, "describe these")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(gotImages) != 2 || gotImages[0] != "a.png" {
		t.Errorf("provider saw images %v; want [a.png b.jpg]", gotImages)
	}
}

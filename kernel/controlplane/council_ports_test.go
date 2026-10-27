// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/runtime"
)

// The council's panel comes from the kernel — but it comes through the
// APPLICATION's ports, not through the host's own wiring.
//
// The application knows which capabilities the council needs and how each of
// them answers; the host hands over the handle and nothing else. The test
// pins the wiring that makes that true: the panel the council publishes is
// the panel the kernel holds, the panel the application seats is the panel
// that comes back, and the model it seats is a model the application knows.
func TestTheCouncilPanelReachesTheApplication(t *testing.T) {
	_, _, client, _ := startPairWithConfig(t, runtime.Config{
		Provider: conductorFakeProvider{},
		Model:    "model-a",
		Tools:    map[string]toolapi.Tool{},
		CouncilMembers: func() []runtime.CouncilMember {
			return []runtime.CouncilMember{
				{Seat: "Chair", Model: "model-a"}, {Seat: "Scribe", Model: "model-b"},
			}
		},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	res, err := client.Call(ctx, controlplane.CmdCouncilMembers, nil)
	if err != nil {
		t.Fatalf("CmdCouncilMembers: %v", err)
	}
	members, _ := res["members"].([]any)
	if len(members) != 2 {
		t.Fatalf("members = %v, want the two the kernel holds", res["members"])
	}
	first, _ := members[0].(map[string]any)
	if first["seat"] != "Chair" || first["model"] != "model-a" {
		t.Errorf("first member = %v, want Chair/model-a", first)
	}
	second, _ := members[1].(map[string]any)
	if second["seat"] != "Scribe" || second["model"] != "model-b" {
		t.Errorf("second member = %v, want Scribe/model-b", second)
	}

	// Seating a panel goes the same way: the application asks the kernel to
	// hold it, and the model it seats is one the application knows.
	set, err := client.Call(ctx, controlplane.CmdCouncilSet, map[string]any{
		"members": []map[string]any{{"seat": "Chair", "model": "model-a"}},
	})
	if err != nil {
		t.Fatalf("CmdCouncilSet: %v", err)
	}
	if saved, _ := set["saved"].(bool); !saved {
		t.Errorf("saved = %v, want true: %v", set["saved"], set)
	}

	after, err := client.Call(ctx, controlplane.CmdCouncilMembers, nil)
	if err != nil {
		t.Fatalf("CmdCouncilMembers after the set: %v", err)
	}
	seated, _ := after["members"].([]any)
	if len(seated) != 1 {
		t.Fatalf("members = %v, want the one the application seated", after["members"])
	}
}

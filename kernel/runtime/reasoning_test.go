// SPDX-License-Identifier: MIT

package runtime_test

import (
	"context"
	"testing"
	"time"

	apptools "github.com/agezt/agezt/kernel/app/tools"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestReasoningUnwiredHoldsNoFace: a kernel whose composition root forgot to
// inject the reasoning module holds NO reasoning face.
//
// The reasoning capability IS the module's (W3.76v): the daemon used to hold
// three forwarders over the module's own operations and refuse with "reasoning
// module not wired". The daemon holds the module's FACE now — one port — and a
// daemon with no module holds none. So there is nothing left to refuse with:
// the layers above see a nil face and raise the contract's row
// (reasoningapi.ErrNoService) themselves.
func TestReasoningUnwiredHoldsNoFace(t *testing.T) {
	k, err := runtime.Open(runtime.Config{
		BaseDir:        t.TempDir(),
		NewToolInvoker: apptools.NewInvoker,
		Tools:          map[string]toolapi.Tool{},
		Provider:       mock.New(),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })

	if k.ReasoningService() != nil {
		t.Fatal("an unwired daemon must hold no reasoning face")
	}
}

// TestReasoningWiredReachesTheModule: the same kernel, once the module is
// injected, answers through it.
func TestReasoningWiredReachesTheModule(t *testing.T) {
	k, err := runtime.Open(runtime.Config{
		BaseDir:        t.TempDir(),
		NewToolInvoker: apptools.NewInvoker,
		Tools:          map[string]toolapi.Tool{},
		Provider:       mock.New(),
		CouncilMembers: func() []runtime.CouncilMember {
			return []runtime.CouncilMember{{Seat: "Solo", Model: "model-x"}}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { k.Close() })
	wireReasoning(t, k)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)

	res, err := k.ReasoningService().Council(ctx, "c", "why?", nil, 0)
	if err != nil {
		t.Fatalf("Council: %v", err)
	}
	if len(res.Members) != 1 || res.Members[0].Model != "model-x" {
		t.Errorf("members = %+v", res.Members)
	}
}

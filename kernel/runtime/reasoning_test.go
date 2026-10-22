// SPDX-License-Identifier: MIT

package runtime_test

import (
	"context"
	"strings"
	"testing"
	"time"

	apptools "github.com/agezt/agezt/kernel/app/tools"
	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestReasoningUnwired: a kernel whose composition root forgot to inject the
// reasoning module says so plainly instead of panicking or silently answering
// nothing. The daemon wires it in `openAppKernel`.
func TestReasoningUnwired(t *testing.T) {
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
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)

	if _, err := k.Council(ctx, "c", "why?", nil, 1); err == nil || !strings.Contains(err.Error(), "reasoning module not wired") {
		t.Errorf("Council: %v", err)
	}
	if _, err := k.Conduct(ctx, "c", runtime.ConductorConfig{Task: "x"}); err == nil || !strings.Contains(err.Error(), "reasoning module not wired") {
		t.Errorf("Conduct: %v", err)
	}
	if _, err := k.Research(ctx, "c", "why?", runtime.ResearchOptions{}); err == nil || !strings.Contains(err.Error(), "reasoning module not wired") {
		t.Errorf("Research: %v", err)
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

	res, err := k.Council(ctx, "c", "why?", nil, 0)
	if err != nil {
		t.Fatalf("Council: %v", err)
	}
	if len(res.Members) != 1 || res.Members[0].Model != "model-x" {
		t.Errorf("members = %+v", res.Members)
	}
}

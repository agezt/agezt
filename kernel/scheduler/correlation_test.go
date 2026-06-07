// SPDX-License-Identifier: MIT

package scheduler_test

import (
	"context"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/scheduler"
)

// When Run is called with an empty correlation id it must generate one
// ("plan-<ulid>"): that id becomes the PlanID and is stamped on every journal
// event the plan emits (plan/node started/completed/failed) and propagated via the
// run context, so an empty one makes the whole plan run uncorrelatable in `agt why`
// / the audit trail (SPEC-08). Many tests call Run(ctx, plan, "") but none asserted
// the generated id, so mutation testing (M498) showed the generation
// (`correlationID = "plan-" + ulid.New()`) could be removed undetected. Pin it.
func TestRun_GeneratesCorrelationIDWhenEmpty(t *testing.T) {
	b, _ := newBus(t)
	e := scheduler.New(scheduler.Config{Bus: b})
	plan := scheduler.Plan{Nodes: []scheduler.Node{&fakeNode{NodeID: "a", ResultOutput: "x"}}}

	res, err := e.Run(context.Background(), plan, "") // empty correlation id
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.PlanID == "" {
		t.Fatal("Run with an empty correlation id must generate a PlanID; an empty one leaves every plan journal event uncorrelatable")
	}
	if !strings.HasPrefix(res.PlanID, "plan-") {
		t.Errorf("generated PlanID = %q, want a \"plan-\" prefix", res.PlanID)
	}
}

// A caller-supplied correlation id must be preserved verbatim (not regenerated),
// so a plan run can be tied to the run/task that launched it.
func TestRun_PreservesProvidedCorrelationID(t *testing.T) {
	b, _ := newBus(t)
	e := scheduler.New(scheduler.Config{Bus: b})
	plan := scheduler.Plan{Nodes: []scheduler.Node{&fakeNode{NodeID: "a"}}}

	res, err := e.Run(context.Background(), plan, "run-12345")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.PlanID != "run-12345" {
		t.Errorf("PlanID = %q, want the provided \"run-12345\" preserved", res.PlanID)
	}
}

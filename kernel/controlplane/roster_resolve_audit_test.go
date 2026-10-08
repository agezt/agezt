// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"testing"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// agent_resolve pauses, retires, delegates or forces a routing chain, so the
// dispatch audit must record it like every other state-changing operation.
func TestAgentResolve_IsOperationAudited(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()
	if _, err := c.Call(ctx, controlplane.CmdAgentAdd, map[string]any{"profile": map[string]any{"slug": "resolvable", "soul": "You work."}}); err != nil {
		t.Fatalf("agent add: %v", err)
	}
	for _, tc := range []struct {
		args     map[string]any
		terminal event.Kind
	}{
		{map[string]any{"ref": "resolvable", "resolution": "paused", "summary": "operator pause"}, event.KindOpCompleted},
		{map[string]any{"ref": "resolvable", "resolution": "nonsense"}, event.KindOpFailed},
	} {
		sub := watchOpAudit(t, k)
		seq, _ := k.Journal().Head()
		_, err := c.Call(ctx, controlplane.CmdAgentResolve, tc.args)
		if (err == nil) != (tc.terminal == event.KindOpCompleted) {
			t.Fatalf("%v: unexpected call result %v", tc.args, err)
		}
		awaitOpAudit(t, sub)
		evs := opEvents(t, k, seq)
		if len(evs) != 2 || evs[0].Kind != event.KindOpInvoked || evs[1].Kind != tc.terminal || evs[0].CorrelationID == "" || evs[0].CorrelationID != evs[1].CorrelationID {
			t.Fatalf("%v: op events = %v, want [op.invoked %s]", tc.args, kinds(evs), tc.terminal)
		}
		if inv := payloadOf(t, evs[0]); inv["op"] != controlplane.CmdAgentResolve || inv["caller"] != "operator" {
			t.Fatalf("invoked payload = %v", inv)
		}
	}
}

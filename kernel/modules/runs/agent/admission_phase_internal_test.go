// SPDX-License-Identifier: MIT

package agent

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
)

func TestGateSharedPolicyRetainsCausalWindow(t *testing.T) {
	tool := &stubTool{def: ToolDef{Name: "probe", InputSchema: objSchema(), Capability: ToolCapability{Name: "introspect"}}, output: "ok"}
	var captured []UntrustedObservationTaint
	s := testState(LoopConfig{Tools: map[string]Tool{"probe": tool}, DirectiveTaintWindow: 1, Policy: func(ctx context.Context, tc ToolCall) PolicyVerdict {
		def, ok := PolicyToolDefFromContext(ctx)
		if !ok || !reflect.DeepEqual(def, tool.def) || tc.ID != "call" {
			t.Errorf("resolved policy inputs: def=%+v call=%+v", def, tc)
		}
		taint, ok := UntrustedObservationTaintFromContext(ctx)
		if !ok {
			t.Error("observation provenance missing")
		}
		captured = append(captured, taint)
		return PolicyVerdict{Allow: true}
	}})
	s.untrustedTaint = UntrustedObservationTaint{Sources: []string{"web"}, DirectiveLike: true, Matches: []string{"directive"}}
	s.directiveObsIter = 5
	for _, iter := range []int{5, 7} {
		if jobs, err := s.gateToolCalls(context.Background(), []ToolCall{{ID: "call", Name: "probe", Input: json.RawMessage(`{}`)}}, iter); err != nil || len(jobs) != 1 || jobs[0].tool == nil {
			t.Fatalf("iteration=%d jobs=%v error=%v", iter, jobs, err)
		}
	}
	if len(captured) != 2 || !captured[0].DirectiveLike || captured[1].DirectiveLike {
		t.Fatalf("window=%+v", captured)
	}
	for _, taint := range captured {
		if !reflect.DeepEqual(taint.Sources, []string{"web"}) || !reflect.DeepEqual(taint.Matches, []string{"directive"}) {
			t.Errorf("provenance=%+v", taint)
		}
	}
	if tool.calls != 0 {
		t.Fatal("admission performed tool effects")
	}
}

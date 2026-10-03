// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// opEvents returns the op.* events journaled since seq, in order.
func opEvents(t *testing.T, k *runtime.Kernel, since int64) []*event.Event {
	t.Helper()
	var out []*event.Event
	if err := k.Journal().Range(func(e *event.Event) error {
		if e.Seq > since && strings.HasPrefix(string(e.Kind), "op.") {
			out = append(out, e)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return out
}

func payloadOf(t *testing.T, e *event.Event) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(e.Payload, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

// TestDispatch_JournalsStateChangingOps: a state-changing command leaves an
// op.invoked / op.completed pair in the journal whether or not its handler
// publishes anything. configcenter.set and schedule_add used to change state
// with no journal trace at all (W2.1).
func TestDispatch_JournalsStateChangingOps(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	ctx := context.Background()

	for _, tc := range []struct {
		cmd     string
		args    map[string]any
		visible map[string]any // arguments that must appear verbatim
	}{
		{controlplane.CmdConfigCenterSet,
			map[string]any{"key": "ops/mode", "value": "careful", "rating": "internal"},
			map[string]any{"rating": "internal"}},
		{controlplane.CmdScheduleAdd,
			map[string]any{"intent": "tidy the inbox", "interval_sec": float64(3600)},
			map[string]any{"intent": "tidy the inbox", "interval_sec": float64(3600)}},
	} {
		seq, _ := k.Journal().Head()
		if _, err := c.Call(ctx, tc.cmd, tc.args); err != nil {
			t.Fatalf("%s: %v", tc.cmd, err)
		}
		evs := opEvents(t, k, seq)
		if len(evs) != 2 || evs[0].Kind != event.KindOpInvoked || evs[1].Kind != event.KindOpCompleted {
			t.Fatalf("%s: op events = %v, want [op.invoked op.completed]", tc.cmd, kinds(evs))
		}
		if evs[0].CorrelationID == "" || evs[0].CorrelationID != evs[1].CorrelationID {
			t.Errorf("%s: invoked/completed not correlated: %q vs %q", tc.cmd, evs[0].CorrelationID, evs[1].CorrelationID)
		}
		inv := payloadOf(t, evs[0])
		if inv["op"] != tc.cmd || inv["caller"] != "operator" {
			t.Errorf("%s: invoked payload = %v", tc.cmd, inv)
		}
		args, _ := inv["args"].(map[string]any)
		for name, want := range tc.visible {
			if args[name] != want {
				t.Errorf("%s: args[%s] = %v, want %v", tc.cmd, name, args[name], want)
			}
		}
	}
}

// TestDispatch_ReadOnlyOpsAreNotJournaled: reads (status, list views the
// console polls) must not flood the journal.
func TestDispatch_ReadOnlyOpsAreNotJournaled(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	seq, _ := k.Journal().Head()
	for _, cmd := range []string{controlplane.CmdStatus, controlplane.CmdScheduleList, controlplane.CmdConfigCenterList} {
		if _, err := c.Call(context.Background(), cmd, nil); err != nil {
			t.Fatalf("%s: %v", cmd, err)
		}
	}
	if evs := opEvents(t, k, seq); len(evs) != 0 {
		t.Fatalf("read-only ops journaled %v", kinds(evs))
	}
}

// TestDispatch_JournalsFailedOps: a refused mutation is recorded as op.failed
// with the reason the caller saw.
func TestDispatch_JournalsFailedOps(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	seq, _ := k.Journal().Head()
	_, callErr := c.Call(context.Background(), controlplane.CmdScheduleAdd, map[string]any{"interval_sec": 60})
	if callErr == nil {
		t.Fatal("schedule_add without an intent succeeded")
	}
	evs := opEvents(t, k, seq)
	if len(evs) != 2 || evs[1].Kind != event.KindOpFailed {
		t.Fatalf("op events = %v, want [op.invoked op.failed]", kinds(evs))
	}
	if msg, _ := payloadOf(t, evs[1])["error"].(string); msg == "" || !strings.Contains(callErr.Error(), msg) {
		t.Errorf("op.failed error = %q, caller saw %q", msg, callErr)
	}
}

// TestDispatch_AuditNeverJournalsSecrets: a key handed to provider_key_add is
// new, so the bus redactor does not know it yet. The audit record must not
// carry it, nor any value nested in an object argument.
func TestDispatch_AuditNeverJournalsSecrets(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	// Joined at run time so secret scanners do not flag a fixture literal.
	secret := strings.Join([]string{"zq7Vb2Lr", "9Xc4Kd8M", "n3Pw6Ty1"}, "")
	seq, _ := k.Journal().Head()
	_, _ = c.Call(context.Background(), controlplane.CmdProviderKeyAdd,
		map[string]any{"provider": "openai", "label": "spare", "value": secret})
	_, _ = c.Call(context.Background(), controlplane.CmdChannelAccountSet,
		map[string]any{"channel": "telegram", "label": "ops", "config": map[string]any{"bot": secret}})
	evs := opEvents(t, k, seq)
	if len(evs) == 0 {
		t.Fatal("no op events journaled")
	}
	for _, e := range evs {
		if strings.Contains(string(e.Payload), secret) {
			t.Fatalf("%s journaled the secret: %s", e.Kind, e.Payload)
		}
	}
	if got := payloadOf(t, evs[0])["args"].(map[string]any)["label"]; got != "spare" {
		t.Errorf("non-secret argument lost: label = %v", got)
	}
}

func kinds(evs []*event.Event) []event.Kind {
	out := make([]event.Kind, len(evs))
	for i, e := range evs {
		out[i] = e.Kind
	}
	return out
}

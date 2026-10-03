// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// The handler writes its socket response before dispatch's deferred terminal
// audit. Receiving a response is therefore not an audit completion barrier.
// Linux -race -count=20 caught JournalsFailedOps reading only op.invoked.
func watchOpAudit(t *testing.T, k *runtime.Kernel) *bus.Subscription {
	t.Helper()
	sub, err := k.Bus().Subscribe("op.>", 16)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sub.Cancel)
	return sub
}

func awaitOpAudit(t *testing.T, sub *bus.Subscription) {
	t.Helper()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for {
		select {
		case e, ok := <-sub.C:
			if !ok {
				t.Fatal("operation audit subscription closed before terminal event")
			}
			if e.Kind == event.KindOpCompleted || e.Kind == event.KindOpFailed {
				return
			}
		case <-timer.C:
			t.Fatal("operation did not publish terminal audit")
		}
	}
}

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
	sub := watchOpAudit(t, k)
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
		awaitOpAudit(t, sub)
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
	sub := watchOpAudit(t, k)
	seq, _ := k.Journal().Head()
	_, callErr := c.Call(context.Background(), controlplane.CmdScheduleAdd, map[string]any{"interval_sec": 60})
	if callErr == nil {
		t.Fatal("schedule_add without an intent succeeded")
	}
	awaitOpAudit(t, sub)
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
	sub := watchOpAudit(t, k)
	// Joined at run time so secret scanners do not flag a fixture literal.
	secret := strings.Join([]string{"zq7Vb2Lr", "9Xc4Kd8M", "n3Pw6Ty1"}, "")
	seq, _ := k.Journal().Head()
	_, _ = c.Call(context.Background(), controlplane.CmdProviderKeyAdd,
		map[string]any{"provider": "openai", "label": "spare", "value": secret})
	awaitOpAudit(t, sub)
	_, _ = c.Call(context.Background(), controlplane.CmdChannelAccountSet,
		map[string]any{"channel": "telegram", "label": "ops", "config": map[string]any{"bot": secret}})
	awaitOpAudit(t, sub)
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

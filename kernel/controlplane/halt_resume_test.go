// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestHalt_RecordsReasonInJournal — operator supplies a reason
// via args.reason; the kernel.halt event payload must contain it
// so postmortems can reconstruct why the halt happened.
func TestHalt_RecordsReasonInJournal(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))

	const reason = "deploy window starting in 5 minutes"
	res, err := c.Call(context.Background(), controlplane.CmdHalt, map[string]any{
		"reason": reason,
	})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if ok, _ := res["ok"].(bool); !ok {
		t.Errorf("ok = false; got %v", res)
	}
	if got, _ := res["reason"].(string); got != reason {
		t.Errorf("response reason = %q want %q", got, reason)
	}
	if !k.IsHalted() {
		t.Error("kernel should be halted")
	}

	// Walk the journal for the kernel.halt event; payload must
	// carry the reason verbatim.
	tail, err := c.Call(context.Background(), controlplane.CmdJournalTail, map[string]any{"n": float64(100)})
	if err != nil {
		t.Fatalf("Tail: %v", err)
	}
	events, _ := tail["events"].([]any)
	found := false
	for _, raw := range events {
		e, _ := raw.(map[string]any)
		if k, _ := e["kind"].(string); k != "halt" {
			continue
		}
		found = true
		payloadRaw := e["payload"]
		buf, _ := json.Marshal(payloadRaw)
		if !strings.Contains(string(buf), reason) {
			t.Errorf("kernel.halt payload missing reason; got %s", string(buf))
		}
	}
	if !found {
		t.Error("kernel.halt event not present in journal after Halt")
	}
}

// TestHalt_NoReasonOmitsPayloadField — backwards-compat: callers
// that don't supply a reason should not see a `reason` key in the
// journaled payload (omitempty semantic).
func TestHalt_NoReasonOmitsPayloadField(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	if _, err := c.Call(context.Background(), controlplane.CmdHalt, nil); err != nil {
		t.Fatalf("Call: %v", err)
	}
	if !k.IsHalted() {
		t.Error("kernel should be halted")
	}

	tail, err := c.Call(context.Background(), controlplane.CmdJournalTail, map[string]any{"n": float64(100)})
	if err != nil {
		t.Fatalf("Tail: %v", err)
	}
	events, _ := tail["events"].([]any)
	for _, raw := range events {
		e, _ := raw.(map[string]any)
		if k, _ := e["kind"].(string); k != "halt" {
			continue
		}
		payloadRaw := e["payload"]
		buf, _ := json.Marshal(payloadRaw)
		if strings.Contains(string(buf), `"reason"`) {
			t.Errorf("payload should omit reason field when unset; got %s", string(buf))
		}
	}
}

// TestResume_RecordsReason — symmetric to Halt, but on resume.
func TestResume_RecordsReason(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	k.Halt()

	const reason = "deploy finished, traffic green"
	res, err := c.Call(context.Background(), controlplane.CmdResume, map[string]any{
		"reason": reason,
	})
	if err != nil {
		t.Fatalf("Call: %v", err)
	}
	if got, _ := res["reason"].(string); got != reason {
		t.Errorf("response reason = %q want %q", got, reason)
	}
	if k.IsHalted() {
		t.Error("kernel should be resumed")
	}

	tail, err := c.Call(context.Background(), controlplane.CmdJournalTail, map[string]any{"n": float64(100)})
	if err != nil {
		t.Fatalf("Tail: %v", err)
	}
	events, _ := tail["events"].([]any)
	found := false
	for _, raw := range events {
		e, _ := raw.(map[string]any)
		if k, _ := e["kind"].(string); k != "resume" {
			continue
		}
		found = true
		payloadRaw := e["payload"]
		buf, _ := json.Marshal(payloadRaw)
		if !strings.Contains(string(buf), reason) {
			t.Errorf("resume payload missing reason; got %s", string(buf))
		}
	}
	if !found {
		t.Error("resume event missing")
	}
}

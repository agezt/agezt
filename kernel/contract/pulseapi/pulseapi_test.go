// SPDX-License-Identifier: MIT

package pulseapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// TestTheBriefWireShape: the brief is delivered to the operator — it is
// printed by the CLI, rendered by the Web UI and journalled — so its JSON
// keys are a contract with every consumer outside the package. The
// always-present keys and the dropped-when-empty ones are pinned.
func TestTheBriefWireShape(t *testing.T) {
	raw, err := json.Marshal(Brief{Title: "the fleet is quiet", Disposition: DispNotify, IssueKey: "runs/1", Items: 1})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, k := range []string{"title", "disposition", "issue_key", "items"} {
		if _, ok := got[k]; !ok {
			t.Errorf("the brief wire must always carry %q, got %v", k, keys(got))
		}
	}
	for _, k := range []string{"body", "correlation_id"} {
		if _, ok := got[k]; ok {
			t.Errorf("the brief wire must drop %q when it is empty, got %v", k, keys(got))
		}
	}
	if len(got) != 4 {
		t.Errorf("the brief wire has %d keys, want 4: %v", len(got), keys(got))
	}
}

// TestTheDispositionsAreTheContract: the dispositions are WIRE — they are
// stored, printed and branched on. A renamed one would silently change what
// happens to a delta.
func TestTheDispositionsAreTheContract(t *testing.T) {
	for _, c := range []struct {
		got  Disposition
		want string
	}{
		{DispDrop, "drop"},
		{DispDigest, "digest"},
		{DispNotify, "notify"},
		{DispAlert, "alert"},
		{DispAct, "act"},
	} {
		if string(c.got) != c.want {
			t.Errorf("the disposition is %q, want %q", string(c.got), c.want)
		}
	}
}

// TestTheLogSinkRendersTheBrief: the log line is what the operator reads —
// its shape is the contract, and a brief with a body is rendered differently
// from one without.
func TestTheLogSinkRendersTheBrief(t *testing.T) {
	var buf bytes.Buffer
	sink := LogSink{W: &buf}
	if err := sink.Deliver(Brief{Title: "the fleet is quiet", Disposition: DispNotify}); err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if got := buf.String(); got != "📣 BRIEF [notify] the fleet is quiet\n" {
		t.Errorf("the brief line = %q", got)
	}

	buf.Reset()
	if err := sink.Deliver(Brief{Title: "three things", Disposition: DispDigest, Body: "runs:\n  - a"}); err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	got := buf.String()
	if !strings.HasPrefix(got, "📣 BRIEF [digest] three things\n") {
		t.Errorf("the brief line = %q", got)
	}
	if !strings.Contains(got, "    runs:\n") {
		t.Errorf("the body must be indented, got %q", got)
	}
}

// TestTheLogSinkWithoutAWriterIsSilent: a sink with no writer is a no-op, not
// a panic — the daemon's stdout may be closed.
func TestTheLogSinkWithoutAWriterIsSilent(t *testing.T) {
	if err := (LogSink{}).Deliver(Brief{Title: "x"}); err != nil {
		t.Fatalf("Deliver: %v", err)
	}
}

// TestTheSinkFuncDelivers: the adapter turns a plain function into a sink.
func TestTheSinkFuncDelivers(t *testing.T) {
	var seen Brief
	sink := SinkFunc(func(b Brief) error {
		seen = b
		return nil
	})
	if err := sink.Deliver(Brief{Title: "hello"}); err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if seen.Title != "hello" {
		t.Errorf("the sink saw %+v", seen)
	}
}

// TestTheMultiSinkFansOutAndKeepsGoing: a failing sink must not suppress the
// others — a Telegram outage must not silence the daemon log — and the first
// error is what the caller hears.
func TestTheMultiSinkFansOutAndKeepsGoing(t *testing.T) {
	var delivered []string
	boom := errors.New("telegram is down")
	sink := MultiSink{
		nil, // a nil sink is skipped, not called into
		SinkFunc(func(Brief) error {
			delivered = append(delivered, "log")
			return nil
		}),
		SinkFunc(func(Brief) error {
			delivered = append(delivered, "telegram")
			return boom
		}),
		SinkFunc(func(Brief) error {
			delivered = append(delivered, "webhook")
			return nil
		}),
	}
	err := sink.Deliver(Brief{Title: "x"})
	if !errors.Is(err, boom) {
		t.Fatalf("the multi sink must answer the first error, got %v", err)
	}
	want := []string{"log", "telegram", "webhook"}
	if len(delivered) != len(want) {
		t.Fatalf("the sinks ran %v, want %v", delivered, want)
	}
	for i, s := range want {
		if delivered[i] != s {
			t.Fatalf("the sinks ran %v, want %v", delivered, want)
		}
	}
}

// keys returns the key names of a decoded JSON object — for the failure
// messages.
func keys(m map[string]json.RawMessage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/agezt/agezt/kernel/bus"
	"github.com/agezt/agezt/kernel/contract/eventapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/journal"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestBusPortStaysANilPortWithoutABus: a daemon with no bus must hand over a
// NIL port, not a port wrapping a nil implementation. The check is on the
// concrete type on purpose — a nil *bus.Bus handed to an interface parameter
// is already a non-nil interface, so the code_exec tool would bind a
// publisher that is not there instead of skipping the event.
func TestBusPortStaysANilPortWithoutABus(t *testing.T) {
	if got := busPort(nil); got != nil {
		t.Fatalf("a missing bus must yield a nil port, got %v", got)
	}
}

// TestJournalPortStaysANilPortWithoutAJournal: the same nil-port rule for
// the journal — the runs tool would bind a reader that is not there instead
// of reporting that the run history is unavailable.
func TestJournalPortStaysANilPortWithoutAJournal(t *testing.T) {
	if got := journalPort(nil); got != nil {
		t.Fatalf("a missing journal must yield a nil port, got %v", got)
	}
}

// TestTheBusPortPublishesTheSpecUnchanged: the wire of the publish port.
//
// The port takes the contract's spec and the bus takes the SAME row — the
// platform's event package keeps it as an alias — so nothing can be
// translated on the way in. The proof is behavioural: publish through the
// port and read back what the journal holds, field for field. An adapter
// that dropped the correlation, rewrote the actor or lost the payload would
// write a DIFFERENT event than the tool asked for.
func TestTheBusPortPublishesTheSpecUnchanged(t *testing.T) {
	j, err := journal.Open(t.TempDir(), journal.Options{})
	if err != nil {
		t.Fatalf("journal.Open: %v", err)
	}
	t.Cleanup(func() { j.Close() })

	port := busPort(bus.New(j))
	if port == nil {
		t.Fatal("a live bus must yield a live port")
	}

	payload := map[string]any{"language": "python", "exit_code": 0}
	if err := port.Publish(eventapi.Spec{
		Subject:       "code.exec",
		Kind:          event.KindCodeExecuted,
		Actor:         "tool.code_exec",
		CorrelationID: "run-1",
		CausationID:   "run-0",
		Payload:       payload,
		Tags:          map[string]string{"tool": "code_exec"},
	}); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	evs, err := j.Tail(10)
	if err != nil {
		t.Fatalf("Tail: %v", err)
	}
	if len(evs) != 1 {
		t.Fatalf("the port must write exactly one event, got %d", len(evs))
	}
	got := evs[0]
	if got.Subject != "code.exec" {
		t.Errorf("the subject drifted: %q", got.Subject)
	}
	if got.Kind != event.KindCodeExecuted {
		t.Errorf("the kind drifted: %q", got.Kind)
	}
	if got.Actor != "tool.code_exec" {
		t.Errorf("the actor drifted: %q", got.Actor)
	}
	if got.CorrelationID != "run-1" {
		t.Errorf("the correlation drifted: %q", got.CorrelationID)
	}
	if got.CausationID != "run-0" {
		t.Errorf("the causation drifted: %q", got.CausationID)
	}
	if got.Tags["tool"] != "code_exec" {
		t.Errorf("the tags drifted: %v", got.Tags)
	}
	var body map[string]any
	if err := json.Unmarshal(got.Payload, &body); err != nil {
		t.Fatalf("the payload is not JSON: %v (%s)", err, got.Payload)
	}
	if body["language"] != "python" {
		t.Errorf("the payload drifted: %v", body)
	}
}

// TestTheJournalPortReadsTheJournalFaithfully: the wire of the read port.
//
// The journal hands back the DURABLE event (with its hash chain and its
// sequence) and the port hands back the read PROJECTION — the kind, the
// correlation, the timestamp and the payload. The copy is field by field and
// it is measured here: a reader that saw a different event than the journal
// holds would fold a different history, and the runs tool would answer with
// runs that never happened.
func TestTheJournalPortReadsTheJournalFaithfully(t *testing.T) {
	j, err := journal.Open(t.TempDir(), journal.Options{})
	if err != nil {
		t.Fatalf("journal.Open: %v", err)
	}
	t.Cleanup(func() { j.Close() })

	if _, err := j.Append(event.Spec{
		Subject:       "task.received",
		Kind:          event.KindTaskReceived,
		Actor:         "kernel",
		CorrelationID: "run-1",
		Payload:       map[string]any{"intent": "do the thing"},
	}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	port := journalPort(j)
	if port == nil {
		t.Fatal("a live journal must yield a live port")
	}
	recs, err := port.Tail(10)
	if err != nil {
		t.Fatalf("Tail: %v", err)
	}
	if len(recs) != 1 {
		t.Fatalf("the port must read exactly one record, got %d", len(recs))
	}
	got := recs[0]
	if got.Kind != event.KindTaskReceived {
		t.Errorf("the kind drifted: %q", got.Kind)
	}
	if got.CorrelationID != "run-1" {
		t.Errorf("the correlation drifted: %q", got.CorrelationID)
	}
	if got.TSUnixMS <= 0 {
		t.Errorf("the timestamp drifted: %d", got.TSUnixMS)
	}
	var body map[string]any
	if err := json.Unmarshal(got.Payload, &body); err != nil {
		t.Fatalf("the payload is not JSON: %v (%s)", err, got.Payload)
	}
	if body["intent"] != "do the thing" {
		t.Errorf("the payload drifted: %v", body)
	}
}

// TestTheBusPortReportsAFailedPublish: a write that did not land must be
// reported.
//
// The port answers exactly one thing — did it land — so a publisher that
// swallowed the error would tell the tool that the event is in the run
// timeline when it is not, and the operator would look for a code.executed
// entry that was never written.
func TestTheBusPortReportsAFailedPublish(t *testing.T) {
	j, err := journal.Open(t.TempDir(), journal.Options{})
	if err != nil {
		t.Fatalf("journal.Open: %v", err)
	}
	t.Cleanup(func() { j.Close() })

	b := bus.New(j)
	b.Close() // a closed bus refuses to publish
	port := busPort(b)
	if port == nil {
		t.Fatal("a live bus must yield a live port")
	}
	err = port.Publish(eventapi.Spec{
		Subject: "code.exec",
		Kind:    event.KindCodeExecuted,
		Actor:   "tool.code_exec",
	})
	if err == nil {
		t.Fatal("a refused publish must be reported, not swallowed")
	}
}

// TestTheJournalPortReportsAFailedRead: a read that did not happen must be
// reported.
//
// The read port is what the runs tool asks for the daemon's history. A port
// that answered "nothing found" for a read that FAILED would tell the
// operator that the daemon has no past runs — when in truth the history
// could not be read at all. The difference between "there is nothing" and
// "I could not look" is the whole point of the port.
func TestTheJournalPortReportsAFailedRead(t *testing.T) {
	dir := t.TempDir()
	j, err := journal.Open(dir, journal.Options{})
	if err != nil {
		t.Fatalf("journal.Open: %v", err)
	}
	t.Cleanup(func() { j.Close() })

	port := journalPort(j)
	if port == nil {
		t.Fatal("a live journal must yield a live port")
	}

	// Put a segment the journal cannot decode into its directory: the read
	// can no longer succeed.
	if err := os.WriteFile(filepath.Join(dir, "00000002.jsonl"), []byte("not an event\n"), 0o600); err != nil {
		t.Fatalf("write a foreign segment: %v", err)
	}
	recs, err := port.Tail(10)
	if err == nil {
		t.Fatalf("a failed read must be reported, got %d records", len(recs))
	}
}

// TestToolKernelDepsSuppliesTheBusAndJournalPorts: the daemon hands the
// code_exec and runs tools their publish and read ports HERE — at the
// composition root — so a daemon that dropped them would leave two tools
// answering "not available" on every live box, while every test that built
// the tool by hand stayed green.
//
// The two ports are wired to the SAME live daemon: what the publish port
// writes, the read port must see. That is the whole duty of the wiring, and
// it is the only place it can be observed.
func TestToolKernelDepsSuppliesTheBusAndJournalPorts(t *testing.T) {
	k, err := runtime.Open(runtime.Config{BaseDir: t.TempDir(), Provider: mock.New()})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	deps := toolKernelDeps(k, t.TempDir(), io.Discard)
	if deps.Bus == nil {
		t.Fatal("the daemon must supply the publish port")
	}
	if deps.Journal == nil {
		t.Fatal("the daemon must supply the read port")
	}

	const corr = "run-wire-1"
	if err := deps.Bus.Publish(eventapi.Spec{
		Subject:       "code.exec",
		Kind:          event.KindCodeExecuted,
		Actor:         "tool.code_exec",
		CorrelationID: corr,
		Payload:       map[string]any{"language": "python"},
	}); err != nil {
		t.Fatalf("Publish through the port: %v", err)
	}

	recs, err := deps.Journal.Tail(200)
	if err != nil {
		t.Fatalf("Tail through the port: %v", err)
	}
	found := false
	for _, r := range recs {
		if r.CorrelationID == corr {
			found = true
			if r.Kind != event.KindCodeExecuted {
				t.Errorf("the published event drifted: %+v", r)
			}
		}
	}
	if !found {
		t.Fatalf("the read port must see what the publish port wrote (correlation %q)", corr)
	}
}

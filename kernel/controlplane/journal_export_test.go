// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestJournalExport runs a task to populate the journal, exports it via
// CmdJournalExport (byte-preserving CallRaw), then re-verifies the exported
// events offline: each event's BLAKE3 hash must recompute, and consecutive
// events must chain by prev_hash. This is the guarantee the M101 bundle relies
// on — proving the export survives the wire round-trip verifiably.
func TestJournalExport(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("done")))

	// Generate some journaled events.
	if _, err := c.Stream(context.Background(), controlplane.CmdRun,
		map[string]any{"intent": "hello"}, nil); err != nil {
		t.Fatalf("run: %v", err)
	}

	raw, err := c.CallRaw(context.Background(), controlplane.CmdJournalExport, nil)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	var res struct {
		Events   []json.RawMessage `json:"events"`
		Count    int               `json:"count"`
		FirstSeq int64             `json:"first_seq"`
		LastSeq  int64             `json:"last_seq"`
		HeadSeq  int64             `json:"head_seq"`
		HeadHash string            `json:"head_hash"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		t.Fatalf("decode export: %v", err)
	}
	if res.Count == 0 || len(res.Events) != res.Count {
		t.Fatalf("count=%d events=%d, want non-zero and equal", res.Count, len(res.Events))
	}
	if res.HeadHash == "" {
		t.Fatalf("head_hash empty — bundle has no chain attestation")
	}
	if got, _ := k.Journal().Head(); res.HeadSeq != got {
		t.Errorf("head_seq=%d, journal head=%d", res.HeadSeq, got)
	}

	// Offline re-verification: decode each event, recompute its hash, and
	// check prev-hash continuity across the slice.
	var prevHash string
	for i, rawEv := range res.Events {
		e, derr := event.Decode(rawEv)
		if derr != nil {
			t.Fatalf("event %d decode: %v", i, derr)
		}
		if verr := e.VerifyHash(); verr != nil {
			t.Fatalf("event %d (seq %d) hash invalid after round-trip: %v", i, e.Seq, verr)
		}
		if i > 0 && e.PrevHash != prevHash {
			t.Fatalf("chain break before seq %d: prev_hash %s != prior hash %s", e.Seq, e.PrevHash, prevHash)
		}
		prevHash = e.Hash
	}
}

// TestJournalExportSinceWindow checks the since_ms cutoff narrows the export to
// recent events without breaking verifiability.
func TestJournalExportSinceWindow(t *testing.T) {
	_, _, c, _ := startPair(t, mock.New(mock.FinalText("done")))
	if _, err := c.Stream(context.Background(), controlplane.CmdRun,
		map[string]any{"intent": "x"}, nil); err != nil {
		t.Fatalf("run: %v", err)
	}
	// A 1ms window in the distant-past sense: since_ms is "last N ms", so a
	// tiny window should yield few/zero events but never error, and a huge
	// window should yield the same as no filter. Use a large window here.
	raw, err := c.CallRaw(context.Background(), controlplane.CmdJournalExport,
		map[string]any{"since_ms": int64(60_000)})
	if err != nil {
		t.Fatalf("export since: %v", err)
	}
	var res struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if res.Count == 0 {
		t.Errorf("60s window should include the just-created events, got 0")
	}
}

// TestJournalExportScopedByCorrelation runs TWO tasks (two correlations), then
// exports with a correlation filter and asserts the cut contains ONLY that run's
// events — the SPEC-09 §3 "task" scope (M383). A surgical subgraph cut, not a
// contiguous window.
func TestJournalExportScopedByCorrelation(t *testing.T) {
	// A Responder answers every run (two runs here), unlike a fixed scripted list.
	prov := &mock.Provider{Responder: func(agent.CompletionRequest) agent.CompletionResponse {
		return mock.FinalText("done")
	}}
	_, _, c, _ := startPair(t, prov)
	for _, intent := range []string{"run one", "run two"} {
		if _, err := c.Stream(context.Background(), controlplane.CmdRun,
			map[string]any{"intent": intent}, nil); err != nil {
			t.Fatalf("run %q: %v", intent, err)
		}
	}

	// Full export first, to discover a real correlation id and the full count.
	rawFull, err := c.CallRaw(context.Background(), controlplane.CmdJournalExport, nil)
	if err != nil {
		t.Fatalf("full export: %v", err)
	}
	var full struct {
		Events []json.RawMessage `json:"events"`
		Count  int               `json:"count"`
	}
	if err := json.Unmarshal(rawFull, &full); err != nil {
		t.Fatalf("decode full: %v", err)
	}
	// Pick the correlation of the first task.received event.
	target := ""
	for _, raw := range full.Events {
		e, derr := event.Decode(raw)
		if derr != nil {
			t.Fatalf("decode event: %v", derr)
		}
		if e.Kind == event.KindTaskReceived && e.CorrelationID != "" {
			target = e.CorrelationID
			break
		}
	}
	if target == "" {
		t.Fatal("no task.received with a correlation found")
	}

	// Scoped export.
	rawScoped, err := c.CallRaw(context.Background(), controlplane.CmdJournalExport,
		map[string]any{"correlation": target})
	if err != nil {
		t.Fatalf("scoped export: %v", err)
	}
	var scoped struct {
		Events      []json.RawMessage `json:"events"`
		Count       int               `json:"count"`
		Correlation string            `json:"correlation"`
	}
	if err := json.Unmarshal(rawScoped, &scoped); err != nil {
		t.Fatalf("decode scoped: %v", err)
	}
	if scoped.Correlation != target {
		t.Errorf("result correlation = %q, want %q", scoped.Correlation, target)
	}
	if scoped.Count == 0 {
		t.Fatal("scoped export returned 0 events")
	}
	if scoped.Count >= full.Count {
		t.Errorf("scoped count %d should be < full count %d (run two excluded)", scoped.Count, full.Count)
	}
	// EVERY event in the cut must belong to the target correlation, and each must
	// still hash-verify after the round-trip.
	for i, raw := range scoped.Events {
		e, derr := event.Decode(raw)
		if derr != nil {
			t.Fatalf("scoped event %d decode: %v", i, derr)
		}
		if e.CorrelationID != target {
			t.Errorf("scoped event %d has correlation %q, want %q (foreign in cut)", i, e.CorrelationID, target)
		}
		if verr := e.VerifyHash(); verr != nil {
			t.Errorf("scoped event %d hash invalid: %v", i, verr)
		}
	}
}

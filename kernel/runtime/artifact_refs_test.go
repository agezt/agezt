// SPDX-License-Identifier: MIT

package runtime_test

import (
	"testing"

	"github.com/agezt/agezt/kernel/contract/toolapi"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/modules/artifacts/artifact"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// journalRawRefEvent journals a tool.result that names an offloaded blob —
// the shape the agent loop writes when a tool output is too large to inline
// (SPEC-04 §3.6). The raw_ref is the journal's claim on those bytes.
func journalRawRefEvent(t *testing.T, k *runtime.Kernel, corr, ref string) {
	t.Helper()
	_, err := k.Journal().Append(event.Spec{
		Subject:       "agent." + corr + ".tool",
		Kind:          event.KindToolResult,
		Actor:         "kernel",
		CorrelationID: corr,
		Payload:       map[string]any{"tool": "shell", "output": "preview", "raw_ref": ref, "output_bytes": 4096},
	})
	if err != nil {
		t.Fatalf("append tool.result: %v", err)
	}
}

// TestArtifactCollect_KeepsTheBlobTheJournalNames is the end-to-end proof: a
// tool output the journal named must survive the collector, and the journal's
// raw_ref must keep working after it.
func TestArtifactCollect_KeepsTheBlobTheJournalNames(t *testing.T) {
	k, err := runtime.Open(runtime.Config{
		BaseDir:  t.TempDir(),
		Provider: mock.New(mock.FinalText("ok")),
		Tools:    map[string]toolapi.Tool{},
	})
	if err != nil {
		t.Fatalf("open kernel: %v", err)
	}
	defer k.Close()

	idx := k.ArtifactIndex()
	if idx == nil {
		t.Fatal("the kernel has no artifact index")
	}
	data := []byte("the full tool output")
	ref, err := k.Artifacts().Put(data)
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	journalRawRefEvent(t, k, "corr-1", ref)
	entry, err := idx.IndexRef(ref, artifact.Entry{Kind: "tool-output", Source: "run", Name: "out.txt"}, 1)
	if err != nil {
		t.Fatalf("IndexRef: %v", err)
	}

	// The entry is old enough to be a collection candidate.
	collected, _ := idx.Collect(1000)
	if collected != 0 {
		t.Errorf("collected %d entries, want 0 (the journal still names the blob)", collected)
	}
	if _, ok := idx.Get(entry.ID); !ok {
		t.Error("the entry was forgotten even though the journal still names its bytes")
	}
	got, err := k.Artifacts().Get(ref)
	if err != nil {
		t.Fatalf("the journal's raw_ref points at nothing: %v", err)
	}
	if string(got) != string(data) {
		t.Errorf("the blob came back as %q, want %q", got, data)
	}

	// The explicit delete is refused with the named reason.
	if err := idx.Delete(entry.ID); err == nil {
		t.Error("the explicit delete of a journal-named artifact was allowed")
	}
}

// TestArtifactCollect_StillReapsWhatTheJournalDoesNotName: the protection is
// narrow — an artifact the journal never named is still collected.
func TestArtifactCollect_StillReapsWhatTheJournalDoesNotName(t *testing.T) {
	k, err := runtime.Open(runtime.Config{
		BaseDir:  t.TempDir(),
		Provider: mock.New(mock.FinalText("ok")),
		Tools:    map[string]toolapi.Tool{},
	})
	if err != nil {
		t.Fatalf("open kernel: %v", err)
	}
	defer k.Close()

	idx := k.ArtifactIndex()
	entry, err := idx.PutEntry(artifact.Entry{Kind: "image", Source: "inbound", Name: "in.png"}, []byte("png-bytes"), 1)
	if err != nil {
		t.Fatalf("PutEntry: %v", err)
	}
	collected, _ := idx.Collect(1000)
	if collected != 1 {
		t.Errorf("collected %d entries, want 1", collected)
	}
	if _, ok := idx.Get(entry.ID); ok {
		t.Error("the entry survived the collector")
	}
}

// TestArtifactCollect_ProtectsEveryJournalNamedBlob: the guard answers for a
// whole batch at once, so it must find EVERY candidate the journal names — not
// just the first one it walks past.
func TestArtifactCollect_ProtectsEveryJournalNamedBlob(t *testing.T) {
	k, err := runtime.Open(runtime.Config{
		BaseDir:  t.TempDir(),
		Provider: mock.New(mock.FinalText("ok")),
		Tools:    map[string]toolapi.Tool{},
	})
	if err != nil {
		t.Fatalf("open kernel: %v", err)
	}
	defer k.Close()

	idx := k.ArtifactIndex()
	var refs []string
	for i := 0; i < 3; i++ {
		data := []byte{byte(i), 'b', 'y', 't', 'e', 's'}
		ref, err := k.Artifacts().Put(data)
		if err != nil {
			t.Fatalf("put: %v", err)
		}
		refs = append(refs, ref)
		journalRawRefEvent(t, k, "corr-batch", ref)
		if _, err := idx.IndexRef(ref, artifact.Entry{Kind: "tool-output", Source: "run"}, 1); err != nil {
			t.Fatalf("IndexRef: %v", err)
		}
	}
	collected, _ := idx.Collect(1000)
	if collected != 0 {
		t.Errorf("collected %d entries, want 0 (the journal names all three)", collected)
	}
	for _, ref := range refs {
		if has, _ := k.Artifacts().Has(ref); !has {
			t.Errorf("the blob %s was collected although the journal names it", ref[:12])
		}
	}
}

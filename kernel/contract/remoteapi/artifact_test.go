// SPDX-License-Identifier: MIT

package remoteapi

import (
	"encoding/json"
	"testing"
)

// TestArtifactWireShape: the json names are the wire between two daemons —
// a peer of another version reads these keys, so they are a promise.
func TestArtifactWireShape(t *testing.T) {
	raw, err := json.Marshal(Artifact{
		ID:        "art-1",
		Ref:       "ref-1",
		Name:      "result.txt",
		Mime:      "text/plain",
		Kind:      "tool-output",
		Source:    "run",
		Sender:    "agent-run-abc",
		Corr:      "run-abc",
		Size:      11,
		CreatedMS: 1000,
		Caption:   "note",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"id":"art-1","ref":"ref-1","name":"result.txt","mime":"text/plain","kind":"tool-output","source":"run","sender":"agent-run-abc","corr":"run-abc","size":11,"created_ms":1000,"caption":"note"}`
	if string(raw) != want {
		t.Fatalf("artifact wire = %s, want %s", raw, want)
	}
}

// TestArtifactWireOmitsTheEmptyFields: an artifact without a name, a caption
// or a sender travels with the keys that carry a value only.
func TestArtifactWireOmitsTheEmptyFields(t *testing.T) {
	raw, err := json.Marshal(Artifact{ID: "art-2", Ref: "ref-2", Size: 3, CreatedMS: 7})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"id":"art-2","ref":"ref-2","size":3,"created_ms":7}`
	if string(raw) != want {
		t.Fatalf("artifact wire = %s, want %s", raw, want)
	}
}

// TestArtifactReadsTheWire: a peer that answers with the wire keys is read
// into the row, field by field.
func TestArtifactReadsTheWire(t *testing.T) {
	var a Artifact
	if err := json.Unmarshal([]byte(`{"id":"art-3","ref":"ref-3","name":"n","mime":"m","kind":"k","source":"s","sender":"se","corr":"c","size":5,"created_ms":9,"caption":"cap"}`), &a); err != nil {
		t.Fatal(err)
	}
	want := Artifact{ID: "art-3", Ref: "ref-3", Name: "n", Mime: "m", Kind: "k", Source: "s", Sender: "se", Corr: "c", Size: 5, CreatedMS: 9, Caption: "cap"}
	if a != want {
		t.Fatalf("artifact = %+v, want %+v", a, want)
	}
}

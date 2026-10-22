// SPDX-License-Identifier: MIT

package remote

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/remoteapi"
	"github.com/agezt/agezt/kernel/event"
)

// TestArtifactIsTheContractRow: the mirror's artifact row IS the row the peer
// serves (the contract row), so the host decodes the peer's answer into it
// without a field-by-field projection — and the two sides cannot drift.
func TestArtifactIsTheContractRow(t *testing.T) {
	if reflect.TypeOf(Artifact{}) != reflect.TypeOf(remoteapi.Artifact{}) {
		t.Fatalf("the mirror's artifact row drifted from the contract row: %v vs %v", reflect.TypeOf(Artifact{}), reflect.TypeOf(remoteapi.Artifact{}))
	}
	// A row is usable as the contract row without a conversion...
	var row remoteapi.Artifact = Artifact{ID: "art-1", Ref: "ref-1", Size: 3, CreatedMS: 7}
	if row.ID != "art-1" || row.Ref != "ref-1" || row.Size != 3 || row.CreatedMS != 7 {
		t.Fatal(row)
	}
	// ...and so is a whole slice, which is what the host hands over.
	var rows []Artifact = []remoteapi.Artifact{{ID: "art-1"}}
	if len(rows) != 1 || rows[0].ID != "art-1" {
		t.Fatal(rows)
	}
}

// TestEventIsTheCanonicalEvent: the mirror's event IS the canonical event —
// the very row the peer serves — so the host decodes the peer's answer into
// it without a projection.
func TestEventIsTheCanonicalEvent(t *testing.T) {
	if reflect.TypeOf(Event{}) != reflect.TypeOf(event.Event{}) {
		t.Fatalf("the mirror's event row drifted from the canonical event: %v vs %v", reflect.TypeOf(Event{}), reflect.TypeOf(event.Event{}))
	}
	var ev event.Event = Event{ID: "e1", Seq: 1, Kind: event.KindInfo, Payload: json.RawMessage(`{"a":1}`)}
	if ev.ID != "e1" || ev.Seq != 1 || ev.Kind != event.KindInfo || string(ev.Payload) != `{"a":1}` {
		t.Fatal(ev)
	}
}

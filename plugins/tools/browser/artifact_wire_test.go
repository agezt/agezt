// SPDX-License-Identifier: MIT

package browser

import (
	"encoding/json"
	"testing"

	"github.com/agezt/agezt/kernel/contract/remoteapi"
)

// The artifact record the tool reports back IS the contract row's wire, key
// for key.
//
// The tool names the CONTRACT row (not the artifacts module's alias of it),
// so the keys it emits and the values it fills them with have to be that
// row's own. Nothing tied them together: the record is a hand-written map,
// so a renamed contract field would leave the tool emitting an empty value
// under a key nobody reads any more — silently, for every consumer of the
// tool's answer.
func TestActionArtifactRecordIsTheContractRow(t *testing.T) {
	row := remoteapi.Artifact{
		ID:   "art-1",
		Ref:  "ref-1",
		Name: "shot.png",
		Mime: "image/png",
		Kind: "image",
		Size: 42,
	}
	rec := actionArtifactRecord(row)

	rawRec, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("marshal the record: %v", err)
	}
	rawRow, err := json.Marshal(row)
	if err != nil {
		t.Fatalf("marshal the row: %v", err)
	}
	var got, want map[string]any
	if err := json.Unmarshal(rawRec, &got); err != nil {
		t.Fatalf("decode the record: %v", err)
	}
	if err := json.Unmarshal(rawRow, &want); err != nil {
		t.Fatalf("decode the row: %v", err)
	}
	for k, v := range got {
		w, ok := want[k]
		if !ok {
			t.Errorf("the tool emits %q, which the contract row does not carry", k)
			continue
		}
		if v != w {
			t.Errorf("the tool emits %q = %v, the contract row says %v", k, v, w)
		}
	}
	// The exact set of keys is pinned too: a dropped key would leave the
	// model with a field it expects and never gets.
	for _, k := range []string{"id", "ref", "name", "mime", "kind", "size"} {
		if _, ok := got[k]; !ok {
			t.Errorf("the tool no longer emits %q", k)
		}
	}
	if len(got) != 6 {
		t.Errorf("the tool emits %d keys, want 6: %v", len(got), got)
	}
}

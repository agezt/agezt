// SPDX-License-Identifier: MIT

package main

import (
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/remoteapi"
)

// TestPeerArtifactEntryIsTheContractRow: the CLI reads a peer's artifact rows
// with the contract row — the same declaration the serving daemon uses — so
// the CLI does not carry a second description of the peer's wire.
func TestPeerArtifactEntryIsTheContractRow(t *testing.T) {
	if reflect.TypeOf(peerArtifactEntry{}) != reflect.TypeOf(remoteapi.Artifact{}) {
		t.Fatalf("the CLI's peer row drifted from the contract row: %v vs %v", reflect.TypeOf(peerArtifactEntry{}), reflect.TypeOf(remoteapi.Artifact{}))
	}
	var row remoteapi.Artifact = peerArtifactEntry{ID: "art-1", Ref: "ref-1", Size: 3, CreatedMS: 7}
	if row.ID != "art-1" || row.Ref != "ref-1" || row.Size != 3 || row.CreatedMS != 7 {
		t.Fatal(row)
	}
}

// SPDX-License-Identifier: MIT

package restapi

import (
	"reflect"
	"testing"

	"github.com/agezt/agezt/kernel/contract/remoteapi"
)

// TestArtifactEntryIsTheContractRow: the row this adapter serves is the
// contract row — the same declaration the reading side of the peer call uses
// — so the wire cannot drift between the daemon that serves it and the daemon
// that reads it.
func TestArtifactEntryIsTheContractRow(t *testing.T) {
	if reflect.TypeOf(ArtifactEntry{}) != reflect.TypeOf(remoteapi.Artifact{}) {
		t.Fatalf("the served artifact row drifted from the contract row: %v vs %v", reflect.TypeOf(ArtifactEntry{}), reflect.TypeOf(remoteapi.Artifact{}))
	}
}

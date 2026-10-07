// SPDX-License-Identifier: MIT
package controlplane

import (
	"github.com/agezt/agezt/kernel/artifact"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestArtifactAppDeleteMetadataFailureRetainsOwnedEntryAndBlob(t *testing.T) {
	k, s, entry, _ := artifactAppFixture(t)
	path := filepath.Join(k.BaseDir(), "artifacts", "index", entry.ID+".json")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "blocker"), []byte("owned"), 0600); err != nil {
		t.Fatal(err)
	}
	before := k.ArtifactIndex().List(artifact.Filter{})
	response := callAppHost(t, s, Request{ID: "delete-failure", Cmd: CmdArtifactDelete, Token: "primary", Args: map[string]any{"id": entry.ID}})[0]
	after := k.ArtifactIndex().List(artifact.Filter{})
	_, blobErr := k.Artifacts().Get(entry.Ref)
	if response.Type != RespError || !strings.Contains(response.Error, "remove metadata") || response.Result != nil || !reflect.DeepEqual(before, after) || blobErr != nil {
		t.Fatalf("EXPECTED: one metadata-removal error and unchanged index/blob; ACTUAL: response=%+v changed=%v blobErr=%v", response, !reflect.DeepEqual(before, after), blobErr)
	}
}

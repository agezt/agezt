// SPDX-License-Identifier: MIT
package controlplane_test

import (
	"context"
	"github.com/agezt/agezt/kernel/artifact"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/plugins/providers/mock"
	"reflect"
	"strings"
	"testing"
)

func TestStorageArtifactSocketTenantDenialPreservesPrimary(t *testing.T) {
	k, server, _, dir := startPair(t, mock.New())
	registry := withTenants(t, server, dir)
	token := mustTenant(t, registry, "acme")
	entry, err := k.ArtifactIndex().PutEntry(artifact.Entry{Name: "primary-private"}, []byte("owned"), 100)
	if err != nil {
		t.Fatal(err)
	}
	before := k.ArtifactIndex().List(artifact.Filter{})
	client := tenantClient(t, dir, token)
	for _, cmd := range []string{controlplane.CmdStorageStats, controlplane.CmdArtifactGet, controlplane.CmdArtifactList, controlplane.CmdArtifactDelete, controlplane.CmdArtifactCollect} {
		if _, err := client.Call(context.Background(), cmd, map[string]any{"id": entry.ID, "ref": entry.Ref, "dry_run": false, "tenant": "acme"}); err == nil || !strings.Contains(err.Error(), "forbidden") {
			t.Fatal(cmd, err)
		}
	}
	after := k.ArtifactIndex().List(artifact.Filter{})
	if !reflect.DeepEqual(before, after) {
		t.Fatal("tenant changed primary metadata")
	}
	if _, err = k.Artifacts().Get(entry.Ref); err != nil {
		t.Fatal("tenant changed primary blob", err)
	}
}

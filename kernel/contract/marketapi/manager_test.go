// SPDX-License-Identifier: MIT

package marketapi

import (
	"reflect"
	"testing"
)

// TestTheManagerPortHasExactlyEightVerbs: the marketplace port is EIGHT verbs
// wide — the catalogue is read (List, Show, Sources) and the lifecycle is
// driven (InstallContext, UninstallContext, AddSource, RemoveSource, Sync).
//
// The width is the port's contract with every holder: the kernel holds it, the
// control plane drives it, and a verb that appears here appears for all of
// them. A port that grew a ninth verb would be a change nobody chose.
func TestTheManagerPortHasExactlyEightVerbs(t *testing.T) {
	port := reflect.TypeOf((*Manager)(nil)).Elem()
	if port.NumMethod() != 8 {
		t.Fatalf("the marketplace port has %d verbs, want 8", port.NumMethod())
	}
	want := map[string]bool{
		"List":             true,
		"Show":             true,
		"Sources":          true,
		"InstallContext":   true,
		"UninstallContext": true,
		"AddSource":        true,
		"RemoveSource":     true,
		"Sync":             true,
	}
	for i := 0; i < port.NumMethod(); i++ {
		name := port.Method(i).Name
		if !want[name] {
			t.Errorf("the marketplace port grew an unexpected verb %q", name)
		}
		delete(want, name)
	}
	for name := range want {
		t.Errorf("the marketplace port lost the verb %q", name)
	}
}

// TestTheManagerRowsKeepTheirWireShape: the manager's rows travel over the
// wire — the catalogue the UI lists, the install record the provenance is
// written against, and the progress frame a long install streams. The key set
// is pinned, because a renamed key is a record nobody can read back.
func TestTheManagerRowsKeepTheirWireShape(t *testing.T) {
	for _, c := range []struct {
		name string
		row  any
		want []string
	}{
		{
			name: "Listing",
			row:  Listing{},
			want: []string{"builtin", "installed", "marketplace", "mcp_count", "name", "skill_count", "tool_count", "version"},
		},
		{
			name: "InstalledPack",
			row:  InstalledPack{},
			want: []string{"installed_ms", "marketplace", "name", "version"},
		},
		{
			name: "Event",
			row:  Event{},
			want: []string{"ok", "stage"},
		},
		{
			name: "Source",
			row:  Source{},
			want: []string{"name", "url"},
		},
		{
			name: "SyncResult",
			row:  SyncResult{},
			want: []string{"bytes", "fetched_ms", "packs", "source", "url"},
		},
	} {
		got := wireKeys(t, c.row)
		if len(got) != len(c.want) {
			t.Errorf("the %s wire keys = %v, want %v", c.name, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("the %s wire keys = %v, want %v", c.name, got, c.want)
				break
			}
		}
	}
}

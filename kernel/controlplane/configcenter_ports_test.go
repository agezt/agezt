// SPDX-License-Identifier: MIT
package controlplane

import (
	"context"
	"testing"

	appconfigcenter "github.com/agezt/agezt/kernel/app/configcenter"
	core "github.com/agezt/agezt/kernel/configcenter"
	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// hasConfigEntry reports whether the permission picture's config rows hold the
// given key.
func hasConfigEntry(rows []any, key string) bool {
	for _, row := range rows {
		entry, _ := row.(map[string]any)
		if entry["key"] == key {
			return true
		}
	}
	return false
}

// The host does not bind the config center family's ports — the APPLICATION
// fills them from the kernel handle.
//
// The test pins that binding end to end: the daemon's config entries answer
// from the kernel's center, and the reads and the writes both see it.
func TestTheAdapterBindsTheConfigCenterPortsToItsOwnKernel(t *testing.T) {
	dir := t.TempDir()
	k, err := runtime.Open(withRoster(t, runtime.Config{BaseDir: dir, Provider: mock.New()}))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer k.Close()

	s := NewServer(k, dir)
	s.token = "primary"

	ports := s.configCenterPorts()
	if ports.Reader == nil {
		t.Fatal("the daemon's config center ports have no reader — the kernel holds a config center")
	}
	if ports.Writer == nil {
		t.Fatal("the daemon's config center ports have no writer — the kernel holds a config center")
	}

	// The write goes through the daemon's own ports and lands in its kernel.
	if err := ports.Set(core.NewConfigEntry("daemon.key", "value")); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// The permission picture asks the config center family for the entries —
	// the roster does not reach for the kernel to find them.
	if _, err := k.AddProfile(agentapi.Profile{Slug: "owned", Name: "Owned", Enabled: true}); err != nil {
		t.Fatalf("AddProfile: %v", err)
	}
	picture := callAppHost(t, s, Request{ID: "p", Cmd: CmdAgentPermissions, Token: "primary", Args: map[string]any{"ref": "owned"}})[0]
	if picture.Type != RespResult {
		t.Fatalf("the agent permissions: %s", picture.Error)
	}
	rows, _ := picture.Result["config_entries"].([]any)
	if !hasConfigEntry(rows, "daemon.key") {
		t.Fatalf("the permission picture holds %v, want the config center's own entry", rows)
	}

	// The daemon's read service sees it.
	read, err := s.configCenterReads().Get(context.Background(), appconfigcenter.GetInput{Key: "daemon.key"})
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if read.Entry.Key != "daemon.key" {
		t.Fatalf("Get = %+v, want the entry the daemon's ports wrote", read.Entry)
	}

	// And the daemon's write service deletes it.
	if _, err := s.configCenterWrites().Delete(context.Background(), appconfigcenter.DeleteInput{Key: "daemon.key"}); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	entries, ok := ports.Entries()
	if !ok {
		t.Fatal("the daemon's config center ports say the daemon holds no config center — it holds one")
	}
	for _, entry := range entries {
		if entry.Key == "daemon.key" {
			t.Fatalf("Entries = %+v, want the entry the daemon's write service deleted", entries)
		}
	}
}

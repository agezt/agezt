// SPDX-License-Identifier: MIT
package controlplane

import (
	"testing"

	core "github.com/agezt/agezt/kernel/configcenter"
	"github.com/agezt/agezt/kernel/contract/agentapi"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// An agent's removal deletes the config-center entries that BELONG to it and
// prunes the agent's access references from the entries that stay.
//
// The test pins the wiring the adapter owns: the teardown's config port is
// bound to the daemon's own config center, so the entries the daemon holds are
// the entries the removal touches — and the ones it does not touch are the
// ones that belong to somebody else.
func TestTheAdapterDeletesTheAgentsOwnConfigEntries(t *testing.T) {
	dir := t.TempDir()
	k, err := runtime.Open(withRoster(t, runtime.Config{BaseDir: dir, Provider: mock.New()}))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer k.Close()
	if _, err := k.AddProfile(agentapi.Profile{Slug: "owned", Name: "Owned", Enabled: true}); err != nil {
		t.Fatalf("AddProfile: %v", err)
	}

	own := &core.ConfigEntry{
		Key:       "agent/owned/runtime",
		Value:     "mode=private",
		Rating:    core.RatingInternal,
		CreatedBy: "owned",
		Metadata:  map[string]string{"agent": "owned"},
	}
	if err := k.ConfigCenter().Set(own); err != nil {
		t.Fatalf("Set the agent's own entry: %v", err)
	}
	shared := &core.ConfigEntry{
		Key:           "shared/api-key",
		Value:         "shared",
		Rating:        core.RatingRestricted,
		AllowedAgents: []string{"owned", "other"},
		Metadata:      map[string]string{"agent": "other"},
	}
	if err := k.ConfigCenter().Set(shared); err != nil {
		t.Fatalf("Set the shared entry: %v", err)
	}

	s := NewServer(k, dir)
	s.token = "primary"

	removed := callAppHost(t, s, Request{ID: "rm", Cmd: CmdAgentRemove, Token: "primary", Args: map[string]any{
		"ref":     "owned",
		"cascade": map[string]any{"config": true},
	}})[0]
	if removed.Type != RespResult {
		t.Fatalf("the agent removal: %s", removed.Error)
	}

	entries, ok := s.configCenterPorts().Entries()
	if !ok {
		t.Fatal("the daemon's config center ports say the daemon holds no config center — it holds one")
	}
	if hasConfigEntryKey(entries, "agent/owned/runtime") {
		t.Fatalf("Entries = %+v, want the agent's own entry deleted", entries)
	}

	// The shared entry stays — it belongs to somebody else — but the removed
	// agent's access reference is pruned from it.
	kept, found := findConfigEntry(entries, "shared/api-key")
	if !found {
		t.Fatalf("Entries = %+v, want the shared entry to stay", entries)
	}
	for _, allowed := range kept.AllowedAgents {
		if allowed == "owned" {
			t.Fatalf("the shared entry still names the removed agent: %v", kept.AllowedAgents)
		}
	}
	if len(kept.AllowedAgents) != 1 || kept.AllowedAgents[0] != "other" {
		t.Fatalf("the shared entry's allowed agents = %v, want only the other agent", kept.AllowedAgents)
	}
}

// hasConfigEntryKey reports whether the entries hold the given key.
func hasConfigEntryKey(entries []*core.ConfigEntry, key string) bool {
	_, found := findConfigEntry(entries, key)
	return found
}

// findConfigEntry is the entry with the given key, and whether it is there.
func findConfigEntry(entries []*core.ConfigEntry, key string) (*core.ConfigEntry, bool) {
	for _, entry := range entries {
		if entry != nil && entry.Key == key {
			return entry, true
		}
	}
	return nil, false
}

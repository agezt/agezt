// SPDX-License-Identifier: MIT

package market

import "testing"

// packAt is samplePack at an explicit version, so one fakeLib can serve the
// same pack at different versions across calls.
func packAt(version string) Pack {
	p := samplePack()
	p.Version = version
	return p
}

// InstallRefusesDowngrade builds managers that share one store/forge/mcp but
// serve a catalogue at the given version, so a downgrade attempt drives the
// real Install path over pre-existing install state.
func downgradeManager(t *testing.T, version string, store *Store, forge *fakeForge, mc *fakeMCP) *Manager {
	t.Helper()
	clock := int64(1000)
	return NewManager(Config{
		Library: fakeLib{p: packAt(version)},
		Store:   store,
		Skills:  forge,
		MCP:     mc,
		Now:     func() int64 { return clock },
	})
}

// The install record is keyed by pack name, so recording an older pack over an
// installed newer one silently REWRITES provenance: installed.json then claims
// the older version, no error surfaces anywhere, and List (which compares
// versions) reports nothing to update. The agent tool always installs the
// catalogued version, so a marketplace that rolled back would downgrade every
// agent-initiated install without a single signal.
func TestInstallRefusesSilentDowngrade(t *testing.T) {
	store := NewStore(t.TempDir())
	forge := &fakeForge{}
	mc := &fakeMCP{}

	if _, err := downgradeManager(t, "2.0.0", store, forge, mc).Install("corr", "official", "web-research-pack", "", nil); err != nil {
		t.Fatalf("install 2.0.0: %v", err)
	}

	_, err := downgradeManager(t, "1.9.0", store, forge, mc).Install("corr", "official", "web-research-pack", "", nil)
	if err == nil {
		t.Fatal("installing 1.9.0 over 2.0.0 succeeded: silent downgrade")
	}

	// Provenance must be untouched and nothing may have been materialized:
	// a refusal that leaves a partial footprint is not a refusal.
	ip, ok, err := store.InstalledByName("web-research-pack")
	if err != nil || !ok {
		t.Fatalf("InstalledByName after refused install: ok=%v err=%v", ok, err)
	}
	if ip.Version != "2.0.0" {
		t.Errorf("installed version = %q, want 2.0.0 (record must survive the refusal)", ip.Version)
	}
	if len(forge.created) != 1 {
		t.Errorf("skills materialized = %v, want exactly the one from the first install", forge.created)
	}
	if len(mc.added) != 1 {
		t.Errorf("mcp servers added = %v, want exactly the one from the first install", mc.added)
	}
}

// The guard must not over-refuse: re-installing the SAME version is the
// documented idempotence contract ("re-installing updates the record"), and
// installing a strictly newer version is the normal update path.
func TestInstallAllowsSameAndNewerVersions(t *testing.T) {
	for _, step := range []struct{ version string }{
		{"2.0.0"}, // first install
		{"2.0.0"}, // idempotent re-install
		{"2.1.0"}, // genuine update
		{"2.1.0"}, // idempotent again
	} {
		store := NewStore(t.TempDir())
		m := downgradeManager(t, step.version, store, &fakeForge{}, &fakeMCP{})
		if _, err := m.Install("corr", "official", "web-research-pack", "", nil); err != nil {
			t.Fatalf("install %s: %v", step.version, err)
		}
	}
}

// Downgrade-after-update across ONE store: install 2.0.0, update to 2.1.0,
// then the catalogue rolls back to 2.0.0 — the same pack the operator had
// before, still a downgrade from what is installed now.
func TestInstallRefusesDowngradeAfterUpdate(t *testing.T) {
	store := NewStore(t.TempDir())
	forge := &fakeForge{}
	mc := &fakeMCP{}
	mk := func(v string) *Manager { return downgradeManager(t, v, store, forge, mc) }

	if _, err := mk("2.0.0").Install("corr", "official", "web-research-pack", "", nil); err != nil {
		t.Fatalf("install 2.0.0: %v", err)
	}
	if _, err := mk("2.1.0").Install("corr", "official", "web-research-pack", "", nil); err != nil {
		t.Fatalf("update to 2.1.0: %v", err)
	}
	if _, err := mk("2.0.0").Install("corr", "official", "web-research-pack", "", nil); err == nil {
		t.Fatal("catalogue rollback to a previously-installed version succeeded: silent downgrade")
	}
	ip, ok, err := store.InstalledByName("web-research-pack")
	if err != nil || !ok {
		t.Fatalf("InstalledByName: ok=%v err=%v", ok, err)
	}
	if ip.Version != "2.1.0" {
		t.Errorf("installed version = %q, want 2.1.0", ip.Version)
	}
}

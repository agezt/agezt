// SPDX-License-Identifier: MIT

package main

import (
	"path/filepath"
	"testing"

	"github.com/agezt/agezt/kernel/cadence"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/standing"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestSeedBuiltInGuardiansSeedsTheLiveFleet: the daemon's guardians are
// seeded through the contract's port, and the seeding is REAL — the roster
// holds the guardians afterwards. A daemon that handed the seeder nothing
// would report a fleet it never created.
func TestSeedBuiltInGuardiansSeedsTheLiveFleet(t *testing.T) {
	// The schedule library is the daemon's file (W3.49): the test opens it
	// and hands the kernel the contract's host port.
	dir := t.TempDir()
	sched, err := cadence.OpenStore(filepath.Join(dir, "cadence"))
	if err != nil {
		t.Fatalf("open the schedule store: %v", err)
	}
	// The standing library is the daemon's file too (W3.50): the test opens
	// it and hands the kernel the contract's store port.
	standingStore, err := standing.Open(filepath.Join(dir, "standing"))
	if err != nil {
		t.Fatalf("open the standing store: %v", err)
	}
	k, err := runtime.Open(runtime.Config{BaseDir: dir, Provider: mock.New(), Schedules: sched, Standing: standingStore})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { k.Close() })

	seeded, err := seedBuiltInGuardians(k)
	if err != nil {
		t.Fatalf("seedBuiltInGuardians: %v", err)
	}
	if len(seeded) == 0 {
		t.Fatal("the daemon must seed its guardian fleet")
	}

	// The seeder's work must be in the LIVE roster, not in a copy.
	agents := k.Roster().List()
	if len(agents) < len(seeded) {
		t.Fatalf("the live roster holds %d agents, the seeder reported %d", len(agents), len(seeded))
	}
	slugs := map[string]bool{}
	for _, a := range agents {
		slugs[a.Slug] = true
	}
	for _, s := range seeded {
		if !slugs[s.Slug] {
			t.Fatalf("the guardian %q was reported but is not in the roster", s.Slug)
		}
	}
}

// TestSeedBuiltInGuardiansRefusesAMissingKernel: a daemon with no kernel
// hands the seeder a nil port, and the seeder refuses it — the boot step
// hears about it instead of the daemon panicking on the first read.
func TestSeedBuiltInGuardiansRefusesAMissingKernel(t *testing.T) {
	seeded, err := seedBuiltInGuardians(nil)
	if err == nil {
		t.Fatalf("a missing kernel must be refused, got %v", seeded)
	}
}

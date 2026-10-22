// SPDX-License-Identifier: MIT

package skill

import (
	"strings"
	"testing"
)

// TestSeedBuiltins_DoesNotResurrectAQuarantinedBuiltin proves the built-in
// seeding bug.
//
// The seeder promotes every built-in it finds to ACTIVE, so the built-ins are
// in the retrieval pool out of the box. But it promoted them unconditionally:
// a built-in the operator had pulled out of production (quarantined) came
// straight back into the retrieval pool on the very next boot — the operator's
// decision was undone by the daemon's own seeding.
//
// The seeder's job is to make the built-ins AVAILABLE, not to re-open a
// lifecycle decision. A quarantined built-in stays quarantined until an
// operator promotes it on purpose.
func TestSeedBuiltins_DoesNotResurrectAQuarantinedBuiltin(t *testing.T) {
	f := newForge(t)
	seeded, err := SeedBuiltins(f, "")
	if err != nil {
		t.Fatalf("SeedBuiltins: %v", err)
	}
	if len(seeded) == 0 {
		t.Fatal("nothing was seeded")
	}
	var target Seeded
	for _, s := range seeded {
		if strings.Contains(strings.ToLower(s.Name), "browser") {
			target = s
			break
		}
	}
	if target.ID == "" {
		t.Fatalf("no browser built-in among the seeded skills: %+v", seeded)
	}
	if target.Status != StatusActive {
		t.Fatalf("the seeded built-in is %q, want active", target.Status)
	}

	// The operator pulls it out of production.
	if err := f.Quarantine("op", target.ID, "operator: it misbehaved"); err != nil {
		t.Fatalf("Quarantine: %v", err)
	}
	before, ok, err := f.Get(target.ID)
	if err != nil || !ok {
		t.Fatalf("Get: %v (found=%v)", err, ok)
	}
	if before.Status != StatusQuarantined {
		t.Fatalf("the built-in is %q, want quarantined", before.Status)
	}

	// The next boot seeds again.
	if _, err := SeedBuiltins(f, ""); err != nil {
		t.Fatalf("SeedBuiltins (second): %v", err)
	}

	after, ok, err := f.Get(target.ID)
	if err != nil || !ok {
		t.Fatalf("Get: %v (found=%v)", err, ok)
	}
	if after.Status != StatusQuarantined {
		t.Errorf("the quarantined built-in came back as %q — the seeding resurrected it", after.Status)
	}
}

// TestSeedBuiltins_ReportsTheQuarantinedStatus: the seeding reports the
// status it left the skill in, so the boot log can say why a built-in is not
// in the pool.
func TestSeedBuiltins_ReportsTheQuarantinedStatus(t *testing.T) {
	f := newForge(t)
	seeded, err := SeedBuiltins(f, "")
	if err != nil {
		t.Fatalf("SeedBuiltins: %v", err)
	}
	var target Seeded
	for _, s := range seeded {
		if strings.Contains(strings.ToLower(s.Name), "browser") {
			target = s
			break
		}
	}
	if target.ID == "" {
		t.Fatalf("no browser built-in among the seeded skills: %+v", seeded)
	}
	if err := f.Quarantine("op", target.ID, "operator"); err != nil {
		t.Fatalf("Quarantine: %v", err)
	}
	again, err := SeedBuiltins(f, "")
	if err != nil {
		t.Fatalf("SeedBuiltins (second): %v", err)
	}
	for _, s := range again {
		if s.ID != target.ID {
			continue
		}
		if s.Status != StatusQuarantined {
			t.Errorf("the seeded report says %q, want quarantined", s.Status)
		}
		return
	}
	t.Error("the quarantined built-in was not reported by the second seeding")
}

// TestSeedBuiltins_StillActivatesAFreshBuiltIn: the fix must not stop the
// seeding from doing its job — a built-in that is not pulled out of production
// is still promoted into the retrieval pool.
func TestSeedBuiltins_StillActivatesAFreshBuiltIn(t *testing.T) {
	f := newForge(t)
	seeded, err := SeedBuiltins(f, "")
	if err != nil {
		t.Fatalf("SeedBuiltins: %v", err)
	}
	for _, s := range seeded {
		if s.Status != StatusActive {
			t.Errorf("the seeded built-in %q is %q, want active", s.Name, s.Status)
		}
	}
}

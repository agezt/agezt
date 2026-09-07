// SPDX-License-Identifier: MIT

package market

import (
	"strings"
	"testing"
)

// An explicit version is an exact request. Before this, BOTH remote branches
// dropped the argument: unqualified resolved to the newest, qualified returned
// whatever the named marketplace had cached — so `install x 1.9.0` could
// silently materialize 2.0.0. These tests pin honor-or-error, never
// substitution.
func TestUnqualifiedResolveHonorsExplicitVersion(t *testing.T) {
	for _, tc := range []struct {
		vFirst, vSecond, ask, want string
		why                        string
	}{
		// The older one, deliberately: newest-wins must not override a request.
		{"1.9.0", "2.0.0", "1.9.0", "1.9.0", "asked for the older version"},
		// Control: asking for the newest agrees with the default resolution.
		{"1.9.0", "2.0.0", "2.0.0", "2.0.0", "asked for the newest"},
		// Prerelease exactness: 2.0.0-rc.1 is not 2.0.0.
		{"2.0.0-rc.1", "2.0.0", "2.0.0-rc.1", "2.0.0-rc.1", "prerelease is not its release"},
		// Multi-digit, where string order would also lie.
		{"1.10.0", "1.9.0", "1.9.0", "1.9.0", "multi-digit minor"},
	} {
		store := twoMarketplaceStore(t, "shared-pack", tc.vFirst, tc.vSecond)
		lib := NewCompositeLibrary(nil, store)
		p, err := lib.ResolvePack("", "shared-pack", tc.ask)
		if err != nil {
			t.Fatalf("unqualified resolve (%s): %v", tc.why, err)
		}
		if p.Version != tc.want {
			t.Errorf("asked for %s (%s): got %s, want %s", tc.ask, tc.why, p.Version, tc.want)
		}
	}
}

// A version no marketplace carries must be an ERROR that names what was asked
// and what exists — not a silent substitute of whatever is newest.
func TestUnqualifiedResolveExplicitVersionAbsentIsAnError(t *testing.T) {
	store := twoMarketplaceStore(t, "shared-pack", "1.9.0", "2.0.0")
	lib := NewCompositeLibrary(nil, store)
	p, err := lib.ResolvePack("", "shared-pack", "3.0.0")
	if err == nil {
		t.Fatalf("asked for absent version 3.0.0, silently resolved %s", p.Version)
	}
	for _, want := range []string{"3.0.0", "1.9.0", "2.0.0"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q should mention %s so the operator can act", err.Error(), want)
		}
	}
}

// The qualified branch dropped the version argument too: it returned the named
// marketplace's cached pack regardless of what was asked.
func TestQualifiedResolveHonorsExplicitVersion(t *testing.T) {
	store := twoMarketplaceStore(t, "shared-pack", "1.9.0", "2.0.0")
	lib := NewCompositeLibrary(nil, store)

	// Match: zzz-market carries 2.0.0.
	p, err := lib.ResolvePack("zzz-market", "shared-pack", "2.0.0")
	if err != nil {
		t.Fatalf("qualified match: %v", err)
	}
	if p.Version != "2.0.0" {
		t.Errorf("qualified match got %s, want 2.0.0", p.Version)
	}

	// Mismatch: aaa-market carries 1.9.0, not 2.0.0. Substituting its own
	// version for an explicit request is the same silent lie as above.
	_, err = lib.ResolvePack("aaa-market", "shared-pack", "2.0.0")
	if err == nil {
		t.Fatal("qualified resolve silently returned a version other than the one requested")
	}
	if !strings.Contains(err.Error(), "1.9.0") || !strings.Contains(err.Error(), "2.0.0") {
		t.Errorf("error %q should name both the cached and the requested version", err.Error())
	}
}

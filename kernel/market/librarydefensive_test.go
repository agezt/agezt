// SPDX-License-Identifier: MIT

package market

import (
	"strings"
	"testing"
)

// fakeLib (market_test.go) is a version-IGNORING Library: it returns its one
// pack for a matching name whatever version was asked for. Both production
// Libraries honor the contract now, but the interface cannot force an
// implementation to — a third-party Library plugged in as the builtin seed
// would silently substitute its own version. These tests make the composite
// itself verify what it was handed.

// Qualified as official: the builtin is the only source consulted, so a
// version it returns but was not asked for must be an error naming both —
// never a silent substitute.
func TestCompositeVerifiesBuiltinHonoredExplicitVersion(t *testing.T) {
	lib := NewCompositeLibrary(fakeLib{p: samplePack()}, nil) // "web-research-pack" @ 1.0.0

	// Controls: no version requested, and the version it actually carries.
	if p, err := lib.ResolvePack("official", "web-research-pack", ""); err != nil || p.Version != "1.0.0" {
		t.Fatalf("unversioned official resolve: err=%v version=%s", err, p.Version)
	}
	if p, err := lib.ResolvePack("official", "web-research-pack", "1.0.0"); err != nil || p.Version != "1.0.0" {
		t.Fatalf("official resolve at own version: err=%v version=%s", err, p.Version)
	}

	p, err := lib.ResolvePack("official", "web-research-pack", "9.9.9")
	if err == nil {
		t.Fatalf("version-ignoring builtin silently substituted %s for requested 9.9.9", p.Version)
	}
	for _, want := range []string{"9.9.9", "1.0.0"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q should name %s so the operator can act", err.Error(), want)
		}
	}
}

// Unqualified: a builtin that ignores the version is treated exactly like a
// builtin that answered "not at that version" — resolution falls through to
// the synced marketplaces, which may genuinely carry the requested version.
// The builtin shadow policy is untouched for unversioned lookups.
func TestCompositeFallsThroughWhenBuiltinIgnoresVersion(t *testing.T) {
	builtinPack := samplePack() // "web-research-pack" @ 1.0.0
	store := twoMarketplaceStore(t, builtinPack.Name, "9.9.9", "9.9.9")
	lib := NewCompositeLibrary(fakeLib{p: builtinPack}, store)

	p, err := lib.ResolvePack("", builtinPack.Name, "9.9.9")
	if err != nil {
		t.Fatalf("unqualified resolve after builtin mismatch: %v", err)
	}
	if p.Version != "9.9.9" {
		t.Fatalf("builtin's %s shadowed a marketplace carrying the requested 9.9.9", p.Version)
	}

	// And with no remote carrying it, the request still fails loudly rather
	// than falling back to the builtin's unasked-for version.
	emptyStore := NewStore(t.TempDir())
	lib = NewCompositeLibrary(fakeLib{p: builtinPack}, emptyStore)
	if _, err := lib.ResolvePack("", builtinPack.Name, "9.9.9"); err == nil {
		t.Fatal("unqualified resolve silently returned a version nobody asked for")
	}
}

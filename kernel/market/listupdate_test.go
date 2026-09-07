// SPDX-License-Identifier: MIT

package market

import "testing"

// stubLib is a Library whose catalogue is fixed by the test. The Library
// interface exists precisely so catalogue sources plug in without market
// importing them, so this is the designed seam for controlling the catalogued
// version — the ordering logic under test is List's, not the catalogue's.
type stubLib struct{ entries []MarketplaceEntry }

func (s stubLib) Marketplaces() []Marketplace {
	return []Marketplace{{Name: "stub", FormatVersion: 1, Packs: s.entries}}
}

func (s stubLib) ResolvePack(marketplace, name, version string) (Pack, error) {
	// List never resolves packs; browse-only tests never reach this.
	return Pack{}, ErrNotFoundPack(marketplace, name)
}

// ErrNotFoundPack gives ResolvePack a body without importing errors here.
func ErrNotFoundPack(marketplace, name string) error {
	return &notFoundError{marketplace: marketplace, name: name}
}

type notFoundError struct{ marketplace, name string }

func (e *notFoundError) Error() string { return "market: pack not found: " + e.name }

// listingFor installs `installed` of one pack, catalogues `catalog`, and runs
// the REAL List path so the proof cannot drift from production.
func listingFor(t *testing.T, installed, catalog string) Listing {
	t.Helper()
	store := NewStore(t.TempDir())
	if err := store.RecordInstall(InstalledPack{Name: "demo-pack", Version: installed, Marketplace: "stub"}); err != nil {
		t.Fatalf("RecordInstall: %v", err)
	}
	m := NewManager(Config{
		Library: stubLib{entries: []MarketplaceEntry{{Name: "demo-pack", Version: catalog, Description: "d"}}},
		Store:   store,
	})
	out, err := m.List("")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	for _, l := range out {
		if l.Name == "demo-pack" {
			return l
		}
	}
	t.Fatal("demo-pack missing from listing")
	return Listing{}
}

// "Update available" must mean strictly NEWER, not merely different. The old
// `!=` comparison advertised an update whenever the strings differed, so a
// catalogue that had rolled BACK (or shipped a prerelease of an already
// installed release) offered a downgrade as an update — an operator following
// the badge would silently lose code.
func TestListUpdateAvailableRequiresStrictlyNewer(t *testing.T) {
	for _, tc := range []struct {
		installed, catalog string
		want               bool
		why                string
	}{
		{"2.0.0", "1.9.0", false, "catalogue rolled back"},
		{"1.5.0", "1.5.0", false, "identical versions"},
		{"2.0.0", "2.0.0-rc.1", false, "prerelease of an installed release is older"},
		{"1.0.0", "1.0.0-beta.2", false, "prerelease of an installed release is older"},
		{"1.9.0", "1.10.0", true, "multi-digit minor is genuinely newer"},
		{"2.0.0", "2.0.1", true, "patch bump is newer"},
		{"1.0.0-alpha", "1.0.0", true, "release is newer than its prerelease"},
	} {
		l := listingFor(t, tc.installed, tc.catalog)
		if l.UpdateAvailable != tc.want {
			t.Errorf("installed %q vs catalog %q (%s): UpdateAvailable = %v, want %v",
				tc.installed, tc.catalog, tc.why, l.UpdateAvailable, tc.want)
		}
	}
}

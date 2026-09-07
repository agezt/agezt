// SPDX-License-Identifier: MIT

package market

import (
	"context"
	"net/http/httptest"
	"testing"
)

// syncSource mirrors the sync_test harness: publish a marketplace over
// httptest and sync it into the store under the given LOCAL source name —
// which is what CachedMarketplaces sorts by.
func syncSource(t *testing.T, store *Store, name string, srv *httptest.Server) {
	t.Helper()
	src := Source{Name: name, URL: srv.URL + "/marketplace.json"}
	if err := store.AddSource(src); err != nil {
		t.Fatalf("add source %s: %v", name, err)
	}
	syncer := &Syncer{HTTP: srv.Client(), Timeout: DefaultSyncTimeout}
	if _, err := syncer.Sync(context.Background(), store, src, 100); err != nil {
		t.Fatalf("sync %s: %v", name, err)
	}
}

// twoMarketplaceStore caches the given pack name under two synced marketplaces
// whose names sort in a KNOWN order, carrying vFirst and vSecond respectively.
// The alphabetically-first marketplace is "aaa-market", so iteration order and
// version order can be made to disagree on purpose.
func twoMarketplaceStore(t *testing.T, pack, vFirst, vSecond string) *Store {
	t.Helper()
	pa := remotePack(pack)
	pa.Version = vFirst
	pb := remotePack(pack)
	pb.Version = vSecond
	srvA := serveMarketplace(t, []Pack{pa}, nil)
	srvB := serveMarketplace(t, []Pack{pb}, nil)
	t.Cleanup(srvA.Close)
	t.Cleanup(srvB.Close)
	store := NewStore(t.TempDir())
	syncSource(t, store, "aaa-market", srvA)
	syncSource(t, store, "zzz-market", srvB)
	return store
}

// The unqualified resolve ("find this pack wherever it lives") returned the
// FIRST hit in marketplace-name order — so which version an operator got
// depended on alphabetical marketplace names, not on the pack's versions. With
// "aaa-market" holding the older pack, an install resolved the older version
// while a strictly newer one sat one entry away.
func TestUnqualifiedResolvePicksNewestAcrossMarketplaces(t *testing.T) {
	for _, tc := range []struct {
		vFirst, vSecond, want string
		why                   string
	}{
		// aaa carries OLD, zzz carries NEW: iteration order and version order
		// disagree — the discriminating case.
		{"1.9.0", "2.0.0", "2.0.0", "older pack in the alphabetically-first marketplace"},
		// Mirror: aaa carries NEW. Agrees with iteration order, so it must keep
		// resolving the new one — a control that the fix picks by version, not
		// by inverting the order.
		{"2.0.0", "1.9.0", "2.0.0", "newer pack in the alphabetically-first marketplace"},
		// Multi-digit, where string order would also lie.
		{"1.10.0", "1.9.0", "1.10.0", "multi-digit minor beats nine"},
	} {
		store := twoMarketplaceStore(t, "shared-pack", tc.vFirst, tc.vSecond)
		lib := NewCompositeLibrary(nil, store)
		p, err := lib.ResolvePack("", "shared-pack", "")
		if err != nil {
			t.Fatalf("unqualified resolve (%s): %v", tc.why, err)
		}
		if p.Version != tc.want {
			t.Errorf("unqualified resolve with %s: got version %s, want %s", tc.why, p.Version, tc.want)
		}
	}
}

// The documented clash policy: the built-in seed always wins a name clash — "a
// remote can't shadow Official" — even when a remote carries a strictly newer
// version. Newest-wins applies among remotes only.
func TestUnqualifiedResolveBuiltinStillShadowsRemote(t *testing.T) {
	builtinPack := samplePack() // "web-research-pack" @ 1.0.0
	// Both remotes carry the BUILTIN's own pack at a strictly newer version, so
	// the shadow policy is genuinely exercised — the builtin must win even
	// though newest-wins would pick the remote.
	store := twoMarketplaceStore(t, builtinPack.Name, "9.9.9", "9.9.9")
	lib := NewCompositeLibrary(fakeLib{p: builtinPack}, store)

	remote, err := NewCompositeLibrary(nil, store).ResolvePack("", builtinPack.Name, "")
	if err != nil {
		t.Fatalf("remote-only resolve: %v", err)
	}
	if remote.Version != "9.9.9" {
		t.Fatalf("fixture sanity: remote carries %s, want 9.9.9", remote.Version)
	}

	p, err := lib.ResolvePack("", builtinPack.Name, "")
	if err != nil {
		t.Fatalf("unqualified resolve with builtin present: %v", err)
	}
	if p.Name != builtinPack.Name || p.Version != builtinPack.Version {
		t.Errorf("builtin shadowed by remote: got %s@%s, want the builtin %s@%s",
			p.Name, p.Version, builtinPack.Name, builtinPack.Version)
	}
}

// Qualifying the marketplace is an explicit choice of source: it must return
// exactly that marketplace's pack even when another carries a newer one.
func TestQualifiedResolveStaysExact(t *testing.T) {
	store := twoMarketplaceStore(t, "shared-pack", "2.0.0", "1.9.0")
	lib := NewCompositeLibrary(nil, store)
	p, err := lib.ResolvePack("zzz-market", "shared-pack", "")
	if err != nil {
		t.Fatalf("qualified resolve: %v", err)
	}
	if p.Version != "1.9.0" {
		t.Errorf("qualified resolve got %s, want zzz-market's own 1.9.0", p.Version)
	}
}

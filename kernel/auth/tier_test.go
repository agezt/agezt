// SPDX-License-Identifier: MIT

package auth

import "testing"

// The zero value of Tier must not be usable as an authority.
//
// httpserver.RouteOpts carries a required Tier, and httpserver.Handle panics
// when the tier is not Valid() — the guard that is supposed to catch a route
// registered with no declared authority. While TierPublic was the zero value,
// a RouteOpts literal that omitted `Tier:` got TierPublic, Valid() said yes,
// and the route was registered with NO authentication and no panic. The
// dangerous value was a valid one, so validating the value could never find it.
func TestZeroTierIsNotAUsableAuthority(t *testing.T) {
	var unset Tier // what a caller gets by forgetting to set the field
	if unset.Valid() {
		t.Fatalf("the zero Tier must be invalid; it is %q, so a RouteOpts literal "+
			"omitting Tier registers an unauthenticated route with no diagnostic", unset)
	}
	if unset == TierPublic {
		t.Fatal("the zero Tier must not be TierPublic")
	}
	if got := unset.String(); got != "unset" {
		t.Errorf("zero Tier String() = %q, want %q", got, "unset")
	}
}

func TestEveryDeclaredTierIsValid(t *testing.T) {
	for _, tier := range []Tier{TierPublic, TierUser, TierAdmin} {
		if !tier.Valid() {
			t.Errorf("%q must be a valid authority", tier)
		}
	}
	// An out-of-range value is still rejected.
	if Tier(TierAdmin + 1).Valid() {
		t.Error("a tier above TierAdmin must not validate")
	}
}

// Tiers are ordered, and higher authority includes the lower ones — the
// property httpserver's middleware and token.Authorize rely on.
func TestTiersAreOrdered(t *testing.T) {
	if !(TierPublic < TierUser && TierUser < TierAdmin) {
		t.Fatalf("tier ordering changed: public=%d user=%d admin=%d",
			TierPublic, TierUser, TierAdmin)
	}
}

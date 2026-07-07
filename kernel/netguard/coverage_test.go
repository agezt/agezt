// SPDX-License-Identifier: MIT

package netguard_test

import (
	"net"
	"testing"

	"github.com/agezt/agezt/kernel/netguard"
)

// TestControl_BadAddressNoPort covers the net.SplitHostPort error branch of
// Control: an address with no port at all can't be split.
func TestControl_BadAddressNoPort(t *testing.T) {
	g := netguard.New()
	if err := g.Control("tcp", "noport", nil); err == nil {
		t.Fatalf("Control should reject an address with no host:port")
	}
}

// TestAllowed_BroadcastAndMulticast covers the multicast/broadcast case arm.
func TestAllowed_BroadcastAndMulticast(t *testing.T) {
	g := netguard.New()
	for _, s := range []string{"255.255.255.255", "224.0.0.1", "ff02::1"} {
		if ok, reason := g.Allowed(net.ParseIP(s)); ok {
			t.Fatalf("Allowed(%s) = allowed, want blocked (reason=%q)", s, reason)
		}
	}
}

// TestAllowed_EmbeddedV4Forms exercises embeddedV4:
//   - NAT64 (64:ff9b::a.b.c.d) collapses to the embedded v4 and is classified.
//   - IPv4-compatible ::a.b.c.d collapses to the embedded v4.
//   - ::1 (loopback) and :: (unspecified) keep their own classification (the
//     exclusion branch returns nil so they are NOT treated as embedded v4).
//   - A plain global IPv6 has no embedding and reaches the final return nil.
func TestAllowed_EmbeddedV4Forms(t *testing.T) {
	g := netguard.New()

	// NAT64 wrapping the metadata IP must be blocked (link-local after collapse).
	if ok, _ := g.Allowed(net.ParseIP("64:ff9b::a9fe:a9fe")); ok {
		t.Fatalf("NAT64-embedded 169.254.169.254 should be blocked")
	}
	// IPv4-compatible wrapping a private IP must be blocked.
	if ok, _ := g.Allowed(net.ParseIP("::0a01:0203")); ok { // ::10.1.2.3
		t.Fatalf("IPv4-compatible-embedded 10.1.2.3 should be blocked")
	}
	// ::1 stays loopback (blocked by default) via its own reason, not embedded v4.
	if ok, reason := g.Allowed(net.ParseIP("::1")); ok || reason == "" {
		t.Fatalf("::1 should be blocked as loopback, got ok=%v reason=%q", ok, reason)
	}
	// A plain global IPv6 has no embedded v4 and is allowed.
	if ok, _ := g.Allowed(net.ParseIP("2606:4700:4700::1111")); !ok {
		t.Fatalf("a public IPv6 should be allowed")
	}
}

// TestAllowed_Unspecified covers the IsUnspecified case arm for both families.
func TestAllowed_Unspecified(t *testing.T) {
	g := netguard.New()
	for _, s := range []string{"0.0.0.0", "::"} {
		if ok, reason := g.Allowed(net.ParseIP(s)); ok {
			t.Fatalf("Allowed(%s) = allowed, want blocked (reason=%q)", s, reason)
		}
	}
}

// TestAllowed_NilIP covers the nil-address guard.
func TestAllowed_NilIP(t *testing.T) {
	if ok, reason := netguard.New().Allowed(nil); ok || reason == "" {
		t.Fatalf("Allowed(nil) should be blocked with a reason")
	}
}

// TestAllowed_MalformedLengthIP covers embeddedV4's `v6 == nil` guard: a non-nil
// net.IP with an invalid byte length has neither a To4 nor a To16 form, so
// embeddedV4 returns nil and Allowed falls through the classification switch.
func TestAllowed_MalformedLengthIP(t *testing.T) {
	// 3 bytes: not a valid 4- or 16-byte address.
	netguard.New().Allowed(net.IP{1, 2, 3})
}

// TestAllowed_NAT64EmbeddedPublic collapses a NAT64-wrapped PUBLIC v4 which must
// remain allowed — exercising the NAT64 branch returning a non-blocked address.
func TestAllowed_NAT64EmbeddedPublic(t *testing.T) {
	g := netguard.New()
	if ok, _ := g.Allowed(net.ParseIP("64:ff9b::0808:0808")); !ok { // 8.8.8.8
		t.Fatalf("NAT64-embedded public 8.8.8.8 should be allowed")
	}
}

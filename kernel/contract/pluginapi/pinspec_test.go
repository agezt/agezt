// SPDX-License-Identifier: MIT

package pluginapi_test

import (
	"strings"
	"testing"

	"github.com/agezt/agezt/kernel/contract/pluginapi"
)

// TestParsePinSpec_Basic happy-path of the env-spec parser.
func TestParsePinSpec_Basic(t *testing.T) {
	hash1 := strings.Repeat("a", 64)
	hash2 := strings.Repeat("b", 64)
	pins, err := pluginapi.ParsePinSpec(" search=" + hash1 + " , scrape=" + hash2 + " ")
	if err != nil {
		t.Fatalf("ParsePinSpec: %v", err)
	}
	if got, want := pins["search"], hash1; got != want {
		t.Errorf("search = %q, want %q", got, want)
	}
	if got, want := pins["scrape"], hash2; got != want {
		t.Errorf("scrape = %q, want %q", got, want)
	}
	if len(pins) != 2 {
		t.Errorf("got %d entries, want 2", len(pins))
	}
}

// TestParsePinSpec_RejectsBadFormat: missing '=', empty prefix,
// non-hex pin all return errors (operator gets fast feedback).
func TestParsePinSpec_RejectsBadFormat(t *testing.T) {
	good := strings.Repeat("c", 64)
	cases := []string{
		"search",                            // no '='
		"=" + good,                          // empty prefix
		"search=notahash",                   // pin too short
		"search=" + strings.Repeat("z", 64), // pin not hex
	}
	for _, c := range cases {
		_, err := pluginapi.ParsePinSpec(c)
		if err == nil {
			t.Errorf("ParsePinSpec(%q): expected error", c)
		}
	}
}

// TestParsePinSpec_RejectsDuplicatePrefix ensures a duplicate prefix is a hard error
// (it would otherwise silently keep the last pin, shadowing an intended one) — M216.
func TestParsePinSpec_RejectsDuplicatePrefix(t *testing.T) {
	a := strings.Repeat("a", 64)
	b := strings.Repeat("b", 64)
	_, err := pluginapi.ParsePinSpec("search=" + a + ",scrape=" + b + ",search=" + b)
	if err == nil {
		t.Fatal("a duplicate pin prefix should be rejected")
	}
	if !strings.Contains(err.Error(), "search") || !strings.Contains(err.Error(), "more than once") {
		t.Errorf("error should name the duplicate prefix: %v", err)
	}
}

// TestParsePinSpec_Empty / whitespace-only → empty map, no error.
func TestParsePinSpec_Empty(t *testing.T) {
	for _, c := range []string{"", "   ", ","} {
		pins, err := pluginapi.ParsePinSpec(c)
		if err != nil {
			t.Errorf("ParsePinSpec(%q): %v", c, err)
		}
		if len(pins) != 0 {
			t.Errorf("ParsePinSpec(%q) = %v, want empty", c, pins)
		}
	}
}

// TestPinSpec_UnusedPins reports entries whose prefix didn't match
// any spawned plugin.
func TestPinSpec_UnusedPins(t *testing.T) {
	pins := pluginapi.PinSpec{
		"search": strings.Repeat("a", 64),
		"scrape": strings.Repeat("b", 64),
		"ghost":  strings.Repeat("c", 64),
	}
	stale := pins.UnusedPins([]string{"search", "scrape"})
	if len(stale) != 1 || stale[0] != "ghost" {
		t.Errorf("UnusedPins = %v, want [ghost]", stale)
	}
}

// TestLooksLikeBLAKE3Pin: the pin format check — a 64-char lowercase hex
// digest and nothing else. The parser that accepts a pin and the host that
// verifies the binary against it must agree on this, and they share one
// spelling of it (the contract's).
func TestLooksLikeBLAKE3Pin(t *testing.T) {
	if !pluginapi.LooksLikeBLAKE3Pin(strings.Repeat("a", 64)) {
		t.Error("64 hex chars must look like a pin")
	}
	for _, bad := range []string{
		"",
		strings.Repeat("a", 63),
		strings.Repeat("a", 65),
		strings.Repeat("A", 64), // uppercase
		strings.Repeat("z", 64), // not hex
	} {
		if pluginapi.LooksLikeBLAKE3Pin(bad) {
			t.Errorf("%q must not look like a pin", bad)
		}
	}
}

// SPDX-License-Identifier: MIT

package builtinmarket

import (
	"strings"
	"testing"
)

// An explicit version is an exact request — the same rule the composite
// library's remote branches follow. The builtin index is name -> one pack
// (every builtin carries 1.0.0), so a requested version either matches or it
// does not exist here. Returning the catalogue's own pack for a version it
// does not carry silently installs a version nobody asked for — and it does so
// BEFORE the composite library's remote branches run, so a version a synced
// marketplace genuinely has would be shadowed by the substitution.
func TestResolvePackHonorsExplicitVersion(t *testing.T) {
	l := New()
	if len(l.packs) == 0 {
		t.Fatal("no packs to resolve")
	}
	p := l.packs[0]

	// Controls: no version requested, and the catalogue's own version — both
	// must keep resolving.
	if got, err := l.ResolvePack("", p.Name, ""); err != nil || got.Name != p.Name {
		t.Fatalf("unversioned resolve of %q: err=%v got=%q", p.Name, err, got.Name)
	}
	if got, err := l.ResolvePack("", p.Name, p.Version); err != nil || got.Version != p.Version {
		t.Fatalf("resolve of %q at its own version %s: err=%v got=%s", p.Name, p.Version, err, got.Version)
	}

	// A version the catalogue does not carry must be an error naming what it
	// actually has — never a silent substitute.
	got, err := l.ResolvePack("", p.Name, "9.9.9")
	if err == nil {
		t.Fatalf("asked %q at 9.9.9, silently resolved %s", p.Name, got.Version)
	}
	if !strings.Contains(err.Error(), p.Version) {
		t.Errorf("error %q should name the catalogue's own version %s so the operator can act", err.Error(), p.Version)
	}
	if !strings.Contains(err.Error(), "9.9.9") {
		t.Errorf("error %q should name the requested version", err.Error())
	}
}

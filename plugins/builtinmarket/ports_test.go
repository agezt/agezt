// SPDX-License-Identifier: MIT

package builtinmarket

import (
	"errors"
	"testing"

	"github.com/agezt/agezt/kernel/contract/skillapi"
)

// fakeBuiltins is a built-ins port stand-in: the marketplace is a PLUGIN, so
// it reads the embedded bundles through the contract's port. The fake proves
// the plugin really reads through the port — and that a bundle which cannot
// be read or parsed is skipped rather than fatal.
type fakeBuiltins struct {
	names   []string
	bundles map[string][]byte
	failing map[string]error
}

func (f fakeBuiltins) Names() []string { return f.names }

func (f fakeBuiltins) Bundle(name string) ([]byte, map[string][]byte, error) {
	if err, ok := f.failing[name]; ok {
		return nil, nil, err
	}
	md, ok := f.bundles[name]
	if !ok {
		return nil, nil, errors.New("no such built-in bundle")
	}
	return md, map[string][]byte{"reference/notes.md": []byte("notes")}, nil
}

// TestTheCatalogueIsBuiltThroughThePort: the packs are built from what the
// port hands over — the plugin never reaches for the embedded bytes itself.
func TestTheCatalogueIsBuiltThroughThePort(t *testing.T) {
	l := New(fakeBuiltins{
		names: []string{"alpha"},
		bundles: map[string][]byte{
			"alpha": []byte("---\nname: alpha\ndescription: the alpha skill\ntriggers: [a]\n---\nbody\n"),
		},
	})
	if len(l.packs) != 1 {
		t.Fatalf("the catalogue holds %d packs, want 1", len(l.packs))
	}
	p := l.packs[0]
	if p.Name != "alpha-pack" {
		t.Fatalf("the pack's name = %q, want alpha-pack", p.Name)
	}
	if p.Description != "the alpha skill" {
		t.Errorf("the pack's description = %q, want the parsed one", p.Description)
	}
	if len(p.Tags) != 2 || p.Tags[0] != "alpha" || p.Tags[1] != "a" {
		t.Errorf("the pack's tags = %v, want the bundle name and the parsed triggers", p.Tags)
	}
	if len(p.Skills) != 1 || p.Skills[0].SkillMD == "" {
		t.Fatalf("the pack carries no skill: %+v", p.Skills)
	}
	if len(p.Skills[0].Resources) != 1 {
		t.Errorf("the pack's resources = %v, want the port's one", p.Skills[0].Resources)
	}
}

// TestABundleThatCannotBeReadIsSkipped: the catalogue is best-effort — a
// bundle the port cannot hand over is skipped, not fatal, and the rest of the
// catalogue still stands.
func TestABundleThatCannotBeReadIsSkipped(t *testing.T) {
	l := New(fakeBuiltins{
		names:   []string{"broken", "alpha"},
		bundles: map[string][]byte{"alpha": []byte("---\nname: alpha\n---\nbody\n")},
		failing: map[string]error{"broken": errors.New("boom")},
	})
	found := false
	for _, p := range l.packs {
		if p.Name == "alpha-pack" {
			found = true
		}
		if p.Name == "broken-pack" {
			t.Fatal("a bundle that cannot be read must not become a pack")
		}
	}
	if !found {
		t.Fatalf("the readable bundle is missing: %+v", l.packs)
	}
}

// TestABundleThatDoesNotParseIsSkipped: a bundle whose SKILL.md is malformed
// is skipped too — the catalogue publishes what it can read.
func TestABundleThatDoesNotParseIsSkipped(t *testing.T) {
	l := New(fakeBuiltins{
		names:   []string{"garbage"},
		bundles: map[string][]byte{"garbage": []byte("no frontmatter here")},
	})
	for _, p := range l.packs {
		if p.Name == "garbage-pack" {
			t.Fatal("a bundle whose SKILL.md does not parse must not become a pack")
		}
	}
}

// TestThePortIsTheContractsPort: the catalogue's constructor takes the
// CONTRACT's port, so the daemon can hand the implementations over without
// the plugin naming a module.
func TestThePortIsTheContractsPort(t *testing.T) {
	var _ skillapi.Builtins = fakeBuiltins{}
}

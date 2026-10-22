// SPDX-License-Identifier: MIT

package skill

import (
	"testing"

	"github.com/agezt/agezt/kernel/modules/skills/builtins"
)

// TestBuiltinNames_ExposesTheEmbeddedBundles: the module's public seam for the
// plugin layer. The marketplace's built-in catalogue reads the embedded
// bundles through these two functions, so they must expose exactly the bundles
// the module seeds — and the bytes must be a parseable SKILL.md plus its
// resources. A seam that drifts from the seeding would let the marketplace
// publish a catalogue the daemon does not actually ship.
func TestBuiltinNames_ExposesTheEmbeddedBundles(t *testing.T) {
	names := BuiltinNames()
	if len(names) == 0 {
		t.Fatal("BuiltinNames() returned no bundles")
	}
	if len(names) != len(builtins.Names()) {
		t.Fatalf("BuiltinNames() = %d bundles, want %d", len(names), len(builtins.Names()))
	}

	var withResources int
	for _, n := range names {
		md, res, err := BuiltinBundle(n)
		if err != nil {
			t.Fatalf("BuiltinBundle(%q): %v", n, err)
		}
		if len(md) == 0 {
			t.Errorf("BuiltinBundle(%q) returned no SKILL.md", n)
			continue
		}
		parsed, perr := ParseSkillMD(md)
		if perr != nil {
			t.Errorf("BuiltinBundle(%q) SKILL.md does not parse: %v", n, perr)
			continue
		}
		if parsed.Name == "" {
			t.Errorf("BuiltinBundle(%q) has no skill name", n)
		}
		if len(res) > 0 {
			withResources++
		}
	}
	if withResources == 0 {
		t.Error("no bundle carries any resource file")
	}

	// The seam hands out a copy: a caller mutating the slice must not reach the
	// module's own bundle list.
	names[0] = "modified"
	if BuiltinNames()[0] == "modified" {
		t.Error("BuiltinNames() must return a copy")
	}
}

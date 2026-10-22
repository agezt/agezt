// SPDX-License-Identifier: MIT

package builtins

import "testing"

// TestBundle_SKILLMDisNotAResource: the SKILL.md is the skill's BODY, not one
// of its resources. Shipping it a second time as a resource would put the
// whole document into the bundle directory as well — the agent would read the
// body twice and the bundle manifest would carry a file the skill format
// reserves.
func TestBundle_SKILLMDisNotAResource(t *testing.T) {
	for _, name := range builtinBundles {
		_, res, err := Bundle(name)
		if err != nil {
			t.Fatalf("Bundle(%q): %v", name, err)
		}
		if _, ok := res["SKILL.md"]; ok {
			t.Errorf("bundle %q ships SKILL.md as a resource", name)
		}
	}
}

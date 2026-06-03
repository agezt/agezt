// SPDX-License-Identifier: MIT

package governor

// White-box test for the deterministic longest-prefix price match (M192).

import "testing"

func TestPriceFor_LongestPrefixWinsDeterministically(t *testing.T) {
	// Inject two overlapping fallback keys with DIFFERENT prices. A model
	// id that prefix-matches both must always resolve to the longer (more
	// specific) key — never flip based on Go's randomized map iteration.
	modelPriceTable["zztest-base"] = modelPrice{100, 100}
	modelPriceTable["zztest-base-pro"] = modelPrice{999, 999}
	defer func() {
		delete(modelPriceTable, "zztest-base")
		delete(modelPriceTable, "zztest-base-pro")
	}()

	// Run many times: the OLD first-match code would intermittently return
	// the cheaper base price; longest-prefix always returns the pro price.
	for i := 0; i < 100; i++ {
		p := priceFor("zztest-base-pro-v2")
		if p.InputMicrocentsPerMTok != 999 {
			t.Fatalf("iteration %d: price = %d, want 999 (longest prefix 'zztest-base-pro')",
				i, p.InputMicrocentsPerMTok)
		}
	}

	// A name that only matches the shorter key still resolves to it.
	if p := priceFor("zztest-base-lite"); p.InputMicrocentsPerMTok != 100 {
		t.Errorf("shorter-only match price = %d, want 100", p.InputMicrocentsPerMTok)
	}
}

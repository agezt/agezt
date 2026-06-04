// SPDX-License-Identifier: MIT

package governor

// White-box test for the deterministic longest-prefix price match (M192).

import "testing"

func TestPriceFor_LongestPrefixWinsDeterministically(t *testing.T) {
	// Inject two overlapping fallback keys with DIFFERENT prices. A model
	// id that prefix-matches both must always resolve to the longer (more
	// specific) key — never flip based on Go's randomized map iteration.
	modelPriceTable["zztest-base"] = modelPrice{InputMicrocentsPerMTok: 100, OutputMicrocentsPerMTok: 100}
	modelPriceTable["zztest-base-pro"] = modelPrice{InputMicrocentsPerMTok: 999, OutputMicrocentsPerMTok: 999}
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

// TestCostMicrocentsCached verifies the M289 cache-aware billing path.
func TestCostMicrocentsCached(t *testing.T) {
	const (
		oneMTok = 1_000_000
		// claude-sonnet-4-6 fallback: input 300M, cache-read 30M, output 1500M /MTok.
		sonnet = "claude-sonnet-4-6"
	)

	t.Run("cached=write=0 identical to non-cached", func(t *testing.T) {
		plain := costMicrocents(sonnet, oneMTok, oneMTok)
		cached := costMicrocentsCached(sonnet, oneMTok, 0, 0, oneMTok)
		if plain != cached {
			t.Fatalf("cached(0,0) = %d, want %d (== non-cached)", cached, plain)
		}
	})

	t.Run("cached tokens bill at cache-read rate", func(t *testing.T) {
		// 1 MTok input, 90% cached, no writes, no output.
		got := costMicrocentsCached(sonnet, oneMTok, 900_000, 0, 0)
		// (100_000*300M + 900_000*30M) / 1e6 = 57_000_000 microcents.
		const want = (100_000*int64(300_000_000) + 900_000*int64(30_000_000)) / 1_000_000
		if got != want {
			t.Fatalf("cached cost = %d, want %d", got, want)
		}
		// Must be strictly cheaper than billing every input token at full rate.
		if full := costMicrocents(sonnet, oneMTok, 0); got >= full {
			t.Fatalf("cached cost %d not cheaper than full input cost %d", got, full)
		}
	})

	t.Run("write tokens bill at cache-write rate", func(t *testing.T) {
		// 1 MTok input, all cache-creation (write), no output.
		got := costMicrocentsCached(sonnet, oneMTok, 0, oneMTok, 0)
		// 1 MTok at the 375M write rate = 375_000_000 microcents.
		const want = int64(375_000_000)
		if got != want {
			t.Fatalf("write cost = %d, want %d", got, want)
		}
		// Cache-write is a premium ABOVE the plain input rate.
		if full := costMicrocents(sonnet, oneMTok, 0); got <= full {
			t.Fatalf("write cost %d not above full input cost %d", got, full)
		}
	})

	t.Run("no cache price bills subsets at input rate", func(t *testing.T) {
		// Inject a model with an input rate but NO cache-read/write price.
		modelPriceTable["zzcache-none"] = modelPrice{InputMicrocentsPerMTok: 500_000_000, OutputMicrocentsPerMTok: 0}
		defer delete(modelPriceTable, "zzcache-none")
		cached := costMicrocentsCached("zzcache-none", oneMTok, 600_000, 300_000, 0)
		plain := costMicrocents("zzcache-none", oneMTok, 0)
		if cached != plain {
			t.Fatalf("no-cache-price cost = %d, want %d (== full input)", cached, plain)
		}
	})

	t.Run("cached+write clamped to input", func(t *testing.T) {
		// cached+write > input must clamp (a hostile/buggy report can't credit the ledger).
		a := costMicrocentsCached(sonnet, 1000, 5000, 5000, 0)
		b := costMicrocentsCached(sonnet, 1000, 1000, 0, 0)
		if a != b {
			t.Fatalf("over-claimed subsets cost = %d, want %d (clamped to input, read wins)", a, b)
		}
	})
}

// SPDX-License-Identifier: MIT

package governor

import (
	"fmt"
	"testing"
)

// TestUsageIndex_AccumulatesAndReports covers the best-effort per-correlation
// usage index that backs UsageFor (the API `usage` reporting fast path): tokens
// sum across a run's calls, distinct correlations are separate, an empty
// correlation is ignored, and an unknown correlation misses (→ journal fallback).
func TestUsageIndex_AccumulatesAndReports(t *testing.T) {
	g := &Governor{}

	if _, _, ok := g.UsageFor("c1"); ok {
		t.Fatal("an unknown correlation must miss (so the caller falls back to the journal)")
	}

	// A multi-call run records several times under one correlation; UsageFor
	// returns the SUM, matching what the journal fold would compute.
	g.indexUsageTokens("c1", 10, 5)
	g.indexUsageTokens("c1", 3, 2)
	if in, out, ok := g.UsageFor("c1"); !ok || in != 13 || out != 7 {
		t.Fatalf("UsageFor(c1) = (%d,%d,%v), want (13,7,true)", in, out, ok)
	}

	// Empty correlation is never indexed.
	g.indexUsageTokens("", 1, 1)
	if _, _, ok := g.UsageFor(""); ok {
		t.Error("an empty correlation must not be indexed")
	}

	// Distinct correlations are tracked independently.
	g.indexUsageTokens("c2", 4, 0)
	if in, out, ok := g.UsageFor("c2"); !ok || in != 4 || out != 0 {
		t.Errorf("UsageFor(c2) = (%d,%d,%v), want (4,0,true)", in, out, ok)
	}
}

// TestUsageIndex_Bounded confirms the index never grows past the cap (a reset on
// overflow keeps memory bounded; an evicted correlation just misses and falls
// back to the journal — never a wrong sum).
func TestUsageIndex_Bounded(t *testing.T) {
	g := &Governor{}
	for i := 0; i < usageIndexCap+50; i++ {
		g.indexUsageTokens(fmt.Sprintf("corr-%d", i), 1, 1)
	}
	g.usageMu.Lock()
	n := len(g.usage)
	g.usageMu.Unlock()
	if n > usageIndexCap {
		t.Errorf("usage index holds %d entries, exceeds cap %d", n, usageIndexCap)
	}
	if n == 0 {
		t.Error("usage index should retain the most-recent entries after a reset")
	}
}

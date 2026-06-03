// SPDX-License-Identifier: MIT

package governor_test

// M194: BudgetSnapshot carries the strict-pricing posture so `agt budget`
// can show whether unpriced models are refused or silently charged $0.

import (
	"testing"

	"github.com/agezt/agezt/kernel/governor"
)

func TestSnapshot_ReflectsStrictPricing(t *testing.T) {
	for _, strict := range []bool{true, false} {
		r := governor.NewRegistry()
		mustRegister(t, r, &governor.ProviderInfo{
			Name: "p", Provider: &fakeProvider{name: "p", resp: okResp("m", 1, 1)}, AuthMode: governor.AuthAPIKey,
		})
		g, err := governor.New(governor.Config{Registry: r, StrictPricing: strict})
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		if got := g.Snapshot().StrictPricing; got != strict {
			t.Errorf("Snapshot().StrictPricing = %v, want %v", got, strict)
		}
	}
}

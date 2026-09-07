// SPDX-License-Identifier: MIT

package agentgw

import (
	"testing"
	"time"
)

// The subprocess mint must never grant a child a burst allowance larger than
// its parent's. The halving in CreateSubprocessToken (parent.MaxBurst / 2)
// rounds MaxBurst==1 down to 0, and CreateToken re-defaults a zero MaxBurst to
// 10 — so a parent capped at burst 1 minted children capped at burst 10: a 10x
// escalation past the operator's limit. The gateway's HTTP mint clamps the
// child to the parent (gateway.go); the library path must hold the same
// invariant.
func TestCreateSubprocessToken_BurstNeverEscalatesParent(t *testing.T) {
	tm := NewTokenManager([]byte("test-secret-key-32-chars-minimum!!"))
	parent := &TokenClaims{
		RunID:     "run_parent",
		Caps:      []string{"memory.read"},
		MaxRate:   1,
		MaxBurst:  1,
		ExpiresAt: time.Now().Add(time.Hour),
	}
	parentToken, err := tm.CreateToken(parent)
	if err != nil {
		t.Fatalf("CreateToken(parent): %v", err)
	}
	// Round-trip the parent through validation, the way a real holder presents it.
	parentClaims, err := tm.ValidateToken(parentToken)
	if err != nil {
		t.Fatalf("ValidateToken(parent): %v", err)
	}

	childToken, err := tm.CreateSubprocessToken(parentClaims, "sub_1", parentClaims.Caps, 10*time.Minute)
	if err != nil {
		t.Fatalf("CreateSubprocessToken: %v", err)
	}
	child, err := tm.ValidateToken(childToken)
	if err != nil {
		t.Fatalf("ValidateToken(child): %v", err)
	}

	if child.MaxBurst > parentClaims.MaxBurst {
		t.Errorf("child MaxBurst = %d exceeds parent MaxBurst = %d: the halving rounded 1 to 0 "+
			"and CreateToken re-defaulted the zero to 10, escalating past the operator's limit",
			child.MaxBurst, parentClaims.MaxBurst)
	}
	if child.MaxRate > parentClaims.MaxRate {
		t.Errorf("child MaxRate = %d exceeds parent MaxRate = %d", child.MaxRate, parentClaims.MaxRate)
	}
}

// TestCreateSubprocessToken_BurstHalvingAndFloor: the halving intent is kept
// for parents above the floor (10 → 5), and a child burst is floored at 1 —
// never 0 (which CreateToken would re-default to 10), and never above the
// parent.
func TestCreateSubprocessToken_BurstHalvingAndFloor(t *testing.T) {
	tm := NewTokenManager([]byte("test-secret-key-32-chars-minimum!!"))
	for _, tc := range []struct {
		parentBurst int
		wantChild   int
	}{
		{parentBurst: 10, wantChild: 5},
		{parentBurst: 3, wantChild: 1},
		{parentBurst: 2, wantChild: 1},
		{parentBurst: 1, wantChild: 1},
	} {
		parent := &TokenClaims{
			RunID:     "run_parent",
			Caps:      []string{"memory.read"},
			MaxRate:   tc.parentBurst,
			MaxBurst:  tc.parentBurst,
			ExpiresAt: time.Now().Add(time.Hour),
		}
		childToken, err := tm.CreateSubprocessToken(parent, "sub_1", parent.Caps, 10*time.Minute)
		if err != nil {
			t.Fatalf("parent burst %d: CreateSubprocessToken: %v", tc.parentBurst, err)
		}
		child, err := tm.ValidateToken(childToken)
		if err != nil {
			t.Fatalf("parent burst %d: ValidateToken(child): %v", tc.parentBurst, err)
		}
		if child.MaxBurst != tc.wantChild {
			t.Errorf("parent burst %d: child MaxBurst = %d, want %d", tc.parentBurst, child.MaxBurst, tc.wantChild)
		}
		if child.MaxBurst > parent.MaxBurst {
			t.Errorf("parent burst %d: child MaxBurst = %d exceeds the parent", tc.parentBurst, child.MaxBurst)
		}
	}
}

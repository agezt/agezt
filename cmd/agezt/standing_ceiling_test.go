// SPDX-License-Identifier: MIT

package main

import (
	"testing"

	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/standing"
)

// TestStandingTrustCeiling verifies the M999 fire-path ceiling: the effective cap
// is the MORE restrictive of the order's max_trust and its initiative mode.
func TestStandingTrustCeiling(t *testing.T) {
	cases := []struct {
		name      string
		in        standing.Initiative
		wantLevel edict.TrustLevel
		wantOK    bool
	}{
		{"empty → uncapped (pre-M999 default)", standing.Initiative{}, edict.LevelAllow, false},
		{"max_trust only", standing.Initiative{MaxTrust: "L3"}, edict.LevelAskScoped, true},
		{"inform_only forces L0 even with high max_trust", standing.Initiative{Mode: standing.InitiativeInformOnly, MaxTrust: "L4"}, edict.LevelDeny, true},
		{"ask forces L1", standing.Initiative{Mode: standing.InitiativeAsk}, edict.LevelAsk, true},
		{"ask vs lower max_trust → most restrictive (L0)", standing.Initiative{Mode: standing.InitiativeAsk, MaxTrust: "L0"}, edict.LevelDeny, true},
		{"act_or_ask keeps max_trust", standing.Initiative{Mode: standing.InitiativeActOrAsk, MaxTrust: "L2"}, edict.LevelAskFirst, true},
		{"act_or_ask with no max_trust → uncapped", standing.Initiative{Mode: standing.InitiativeActOrAsk}, edict.LevelAllow, false},
	}
	for _, c := range cases {
		gotLevel, gotOK := standingTrustCeiling(c.in)
		if gotOK != c.wantOK || (gotOK && gotLevel != c.wantLevel) {
			t.Errorf("%s: got (%v,%v) want (%v,%v)", c.name, gotLevel, gotOK, c.wantLevel, c.wantOK)
		}
	}
}

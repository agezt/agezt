// SPDX-License-Identifier: MIT

package edict

import "testing"

// TestOverlayToChanges_RoundTrip — ProjectPolicyChanges(o.ToChanges()) == o,
// the invariant that makes snapshot-resumed replay equivalent to full replay (M95).
func TestOverlayToChanges_RoundTrip(t *testing.T) {
	deny := AskDeny
	orig := PolicyOverlay{
		Mode:   &deny,
		Levels: map[Capability]TrustLevel{"shell": LevelAskScoped, "net": LevelDeny},
		DenyRules: []HardDenyRule{
			{Name: "r1", Substring: "rm -rf", AppliesTo: []Capability{"shell"}},
			{Name: "r2", Substring: "secret"},
		},
	}
	got := ProjectPolicyChanges(orig.ToChanges())

	if got.Mode == nil || *got.Mode != *orig.Mode {
		t.Errorf("mode round-trip: got %v want %v", got.Mode, orig.Mode)
	}
	if len(got.Levels) != 2 || got.Levels["shell"] != LevelAskScoped || got.Levels["net"] != LevelDeny {
		t.Errorf("levels round-trip: got %v", got.Levels)
	}
	if len(got.DenyRules) != 2 || got.DenyRules[0].Name != "r1" || got.DenyRules[1].Name != "r2" {
		t.Errorf("deny round-trip: got %v", got.DenyRules)
	}
}

// TestOverlaySnapshot_SaveLoad — round-trips through disk; absent file is (nil,nil).
func TestOverlaySnapshot_SaveLoad(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/snap.json"
	if snap, err := LoadOverlaySnapshot(path); snap != nil || err != nil {
		t.Fatalf("absent snapshot: got (%v,%v) want (nil,nil)", snap, err)
	}
	in := &OverlaySnapshot{ThroughSeq: 42, Changes: []PolicyChange{{Action: "mode.set", To: "deny"}}}
	if err := SaveOverlaySnapshot(path, in); err != nil {
		t.Fatalf("save: %v", err)
	}
	out, err := LoadOverlaySnapshot(path)
	if err != nil || out == nil {
		t.Fatalf("load: (%v,%v)", out, err)
	}
	if out.ThroughSeq != 42 || len(out.Changes) != 1 || out.Changes[0].Action != "mode.set" {
		t.Errorf("loaded = %+v", out)
	}
}

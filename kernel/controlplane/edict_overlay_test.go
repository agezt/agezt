// SPDX-License-Identifier: MIT

package controlplane_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/plugins/providers/mock"
)

// TestEdictOverlay_FoldsPolicyChanges — `agt edict overlay` folds policy.changed
// into the net overlay (last-wins level, surviving deny rules, mode) (M94).
func TestEdictOverlay_FoldsPolicyChanges(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	pc := func(payload map[string]any) {
		k.Bus().Publish(event.Spec{
			Subject: "edict", Kind: event.KindPolicyChanged, Actor: "operator",
			Payload: payload,
		})
	}
	pc(map[string]any{"action": "level.set", "capability": "shell", "to": "L1"})
	pc(map[string]any{"action": "level.set", "capability": "shell", "to": "L3"}) // last-wins
	pc(map[string]any{"action": "mode.set", "to": "deny"})
	pc(map[string]any{"action": "deny.add", "name": "r1", "substring": "rm -rf", "applies_to": []string{"shell"}})
	pc(map[string]any{"action": "deny.add", "name": "r2", "substring": "secret"})
	pc(map[string]any{"action": "deny.rm", "name": "r2"}) // removed → should not survive

	res, err := c.Call(context.Background(), controlplane.CmdEdictOverlay, nil)
	if err != nil {
		t.Fatal(err)
	}
	if empty, _ := res["empty"].(bool); empty {
		t.Fatal("overlay reported empty despite changes")
	}
	levels, _ := res["levels"].(map[string]any)
	if levels["shell"] != "L3" {
		t.Errorf("shell level = %v want L3 (last-wins)", levels["shell"])
	}
	if mode, _ := res["mode"].(string); mode == "" {
		t.Errorf("mode override missing")
	}
	denies, _ := res["deny_rules"].([]any)
	if len(denies) != 1 {
		t.Fatalf("deny_rules = %d want 1 (r2 removed)", len(denies))
	}
	d0, _ := denies[0].(map[string]any)
	if d0["name"] != "r1" {
		t.Errorf("surviving deny = %v want r1", d0["name"])
	}
}

// TestEdictCompact_EquivalentToFullReplay — the core M95 guarantee: folding
// {snapshot + only post-snapshot changes} yields the SAME overlay as folding the
// entire policy.changed history. Compact writes the snapshot; we then add a
// post-snapshot change and replicate the boot resumption, comparing to a full fold.
func TestEdictCompact_EquivalentToFullReplay(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	pc := func(payload map[string]any) {
		k.Bus().Publish(event.Spec{
			Subject: "edict", Kind: event.KindPolicyChanged, Actor: "operator", Payload: payload,
		})
	}
	pc(map[string]any{"action": "level.set", "capability": "shell", "to": "L1"})
	pc(map[string]any{"action": "level.set", "capability": "shell", "to": "L3"})
	pc(map[string]any{"action": "deny.add", "name": "r1", "substring": "rm -rf"})

	// Compact → snapshot persisted at the kernel's runtime dir.
	if _, err := c.Call(context.Background(), controlplane.CmdEdictCompact, nil); err != nil {
		t.Fatalf("compact: %v", err)
	}
	snap, err := edict.LoadOverlaySnapshot(filepath.Join(k.BaseDir(), "runtime", edict.OverlaySnapshotFile))
	if err != nil || snap == nil {
		t.Fatalf("snapshot load: (%v,%v)", snap, err)
	}

	// A post-snapshot change (seq > through_seq).
	pc(map[string]any{"action": "level.set", "capability": "net", "to": "L0"})

	// Full fold: every policy.changed in the journal.
	var all []edict.PolicyChange
	_ = k.Journal().Range(func(e *event.Event) error {
		if e.Kind == event.KindPolicyChanged {
			var ch edict.PolicyChange
			if json.Unmarshal(e.Payload, &ch) == nil {
				all = append(all, ch)
			}
		}
		return nil
	})
	full := edict.ProjectPolicyChanges(all)

	// Boot resumption: snapshot.Changes + only post-snapshot journal changes.
	resumed := append([]edict.PolicyChange{}, snap.Changes...)
	_ = k.Journal().Range(func(e *event.Event) error {
		if e.Kind == event.KindPolicyChanged && e.Seq > snap.ThroughSeq {
			var ch edict.PolicyChange
			if json.Unmarshal(e.Payload, &ch) == nil {
				resumed = append(resumed, ch)
			}
		}
		return nil
	})
	got := edict.ProjectPolicyChanges(resumed)

	if full.Levels["shell"] != got.Levels["shell"] || full.Levels["net"] != got.Levels["net"] {
		t.Errorf("levels differ: full=%v resumed=%v", full.Levels, got.Levels)
	}
	if len(full.DenyRules) != len(got.DenyRules) {
		t.Errorf("deny rules differ: full=%d resumed=%d", len(full.DenyRules), len(got.DenyRules))
	}
	if got.Levels["net"] != edict.LevelDeny {
		t.Errorf("post-snapshot change lost: net=%v want L0", got.Levels["net"])
	}
}

// TestEdictCompact_JournalsBindingHash — M176 integrity binding: after compaction
// the on-disk snapshot is authoritative at boot, so it must be bound to the
// tamper-evident journal. Compact must emit a policy.compacted event whose
// content_hash equals the snapshot's ContentHash(); a snapshot file edited to
// loosen policy then no longer matches the journaled hash and is rejected at boot.
func TestEdictCompact_JournalsBindingHash(t *testing.T) {
	k, _, c, _ := startPair(t, mock.New(mock.FinalText("ok")))
	pc := func(payload map[string]any) {
		k.Bus().Publish(event.Spec{
			Subject: "edict", Kind: event.KindPolicyChanged, Actor: "operator", Payload: payload,
		})
	}
	pc(map[string]any{"action": "level.set", "capability": "shell", "to": "L1"})
	pc(map[string]any{"action": "deny.add", "name": "r1", "substring": "rm -rf"})

	if _, err := c.Call(context.Background(), controlplane.CmdEdictCompact, nil); err != nil {
		t.Fatalf("compact: %v", err)
	}
	snap, err := edict.LoadOverlaySnapshot(filepath.Join(k.BaseDir(), "runtime", edict.OverlaySnapshotFile))
	if err != nil || snap == nil {
		t.Fatalf("snapshot load: (%v,%v)", snap, err)
	}

	// The latest journaled policy.compacted hash must match the on-disk snapshot.
	var journaledHash string
	_ = k.Journal().Range(func(e *event.Event) error {
		if e.Kind == event.KindPolicyCompacted {
			var p struct {
				ContentHash string `json:"content_hash"`
			}
			if json.Unmarshal(e.Payload, &p) == nil && p.ContentHash != "" {
				journaledHash = p.ContentHash
			}
		}
		return nil
	})
	if journaledHash == "" {
		t.Fatal("no policy.compacted event journaled by compact")
	}
	if got := snap.ContentHash(); got != journaledHash {
		t.Errorf("binding hash mismatch: snapshot=%s journaled=%s", got, journaledHash)
	}

	// Tamper: an edited snapshot (loosened level) no longer matches the journal.
	if len(snap.Changes) == 0 {
		t.Fatal("snapshot has no changes to tamper")
	}
	tampered := *snap
	tampered.Changes = append([]edict.PolicyChange{}, snap.Changes...)
	tampered.Changes[0].To = "L4"
	if tampered.ContentHash() == journaledHash {
		t.Error("tampered snapshot still matches journaled hash; binding ineffective")
	}
}

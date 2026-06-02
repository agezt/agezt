// SPDX-License-Identifier: MIT

package edict

// Policy-overlay snapshot (M95) — durable-policy compaction. The durable
// overlay (M20) is rebuilt at boot by folding every policy.changed event in the
// journal (ProjectPolicyChanges). Over a long-lived system with much runtime
// tuning that history grows unbounded, and every superseded change is replayed.
// A snapshot collapses the net overlay back into a MINIMAL change list plus the
// journal seq it covers, so boot folds {snapshot changes + only the changes
// AFTER it} — the same result, bounded replay. The journal stays the immutable
// source of truth; the snapshot is a derived, regenerable projection.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
)

// OverlaySnapshotFile is the snapshot's filename, under a kernel's
// <baseDir>/runtime/. Shared by the boot replay and the compact handler so the
// two never disagree on where the snapshot lives.
const OverlaySnapshotFile = "edict_overlay_snapshot.json"

// OverlaySnapshot is the compacted form of the durable policy overlay: the
// minimal PolicyChange list that reproduces the net overlay, plus the journal
// head seq it was taken at. Boot replays snapshot.Changes then folds only
// policy.changed events with Seq > ThroughSeq.
type OverlaySnapshot struct {
	ThroughSeq int64          `json:"through_seq"`
	Changes    []PolicyChange `json:"changes"`
}

// ToChanges renders the overlay as the minimal ordered PolicyChange list that
// reproduces it: one mode.set (if any), one level.set per capability (sorted for
// determinism), one deny.add per surviving rule (in their preserved order). The
// round-trip invariant holds: ProjectPolicyChanges(o.ToChanges()) == o — which
// is exactly what makes snapshot-resumed replay equivalent to full replay.
func (o PolicyOverlay) ToChanges() []PolicyChange {
	var out []PolicyChange
	if o.Mode != nil {
		out = append(out, PolicyChange{Action: "mode.set", To: o.Mode.String()})
	}
	caps := make([]string, 0, len(o.Levels))
	for c := range o.Levels {
		caps = append(caps, string(c))
	}
	sort.Strings(caps)
	for _, c := range caps {
		out = append(out, PolicyChange{Action: "level.set", Capability: c, To: o.Levels[Capability(c)].String()})
	}
	for _, r := range o.DenyRules {
		applies := make([]string, 0, len(r.AppliesTo))
		for _, a := range r.AppliesTo {
			applies = append(applies, string(a))
		}
		out = append(out, PolicyChange{Action: "deny.add", Name: r.Name, Substring: r.Substring, AppliesTo: applies})
	}
	return out
}

// ContentHash is a deterministic SHA-256 (hex) over the snapshot's meaning
// (through_seq + changes). It binds the on-disk snapshot to a journaled
// policy.compacted event (M176): the daemon trusts the snapshot at boot only if
// this hash matches the latest journaled value, so an attacker who edits the
// snapshot file to loosen policy changes the hash and is rejected. Deterministic
// because OverlaySnapshot marshals with no maps (ThroughSeq + an ordered slice of
// PolicyChange, each with an ordered AppliesTo) and ToChanges sorts its output.
func (o *OverlaySnapshot) ContentHash() string {
	b, err := json.Marshal(o)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// LoadOverlaySnapshot reads a snapshot from path. Returns (nil, nil) when the
// file is absent (the common case — no compaction has run), so a caller treats
// "no snapshot" and "error" distinctly. A corrupt snapshot returns an error; the
// boot path is expected to fall back to a full journal fold rather than fail.
func LoadOverlaySnapshot(path string) (*OverlaySnapshot, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var snap OverlaySnapshot
	if err := json.Unmarshal(b, &snap); err != nil {
		return nil, err
	}
	return &snap, nil
}

// SaveOverlaySnapshot writes a snapshot to path atomically (write-temp-rename),
// creating the parent directory if needed.
func SaveOverlaySnapshot(path string, snap *OverlaySnapshot) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

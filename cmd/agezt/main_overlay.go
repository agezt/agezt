// SPDX-License-Identifier: MIT

// Policy-overlay + snapshot-path helpers (replayPolicyOverlay,
// overlaySnapshotPath). Extracted from main_overlay.go during Day 211
// god-file refactor (#43, #56). Public API unchanged.
package main

import (
	"encoding/json"
	"path/filepath"

	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/event"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
)

func replayPolicyOverlay(k *kernelruntime.Kernel) (edict.PolicyOverlay, error) {
	// Compaction (M95): if a snapshot exists, seed the fold with its collapsed
	// changes and replay only the journal events recorded AFTER it. ProjectPolicyChanges
	// is resumable (snapshot.ToChanges + later changes folds to the same overlay as the
	// full history), so this is equivalent to the uncompacted replay.
	//
	// Integrity (M176): the snapshot is trusted ONLY when its content hash equals the
	// latest journaled policy.compacted hash, binding it to the tamper-evident journal.
	// A corrupt snapshot, one edited on disk to loosen policy, or one predating the
	// binding fails this check and is ignored — the journal (the source of truth) is
	// folded in full instead.
	snap, serr := edict.LoadOverlaySnapshot(overlaySnapshotPath(k))
	if serr != nil {
		snap = nil
	}

	type seqChange struct {
		seq int64
		ch  edict.PolicyChange
	}
	var all []seqChange
	var journaledHash string // latest policy.compacted content hash
	err := k.Journal().Range(func(ev *event.Event) error {
		switch ev.Kind {
		case event.KindPolicyChanged:
			var ch edict.PolicyChange
			// A single malformed historical payload must not wedge boot; skip it
			// (ProjectPolicyChanges also skips malformed content).
			if json.Unmarshal(ev.Payload, &ch) == nil {
				all = append(all, seqChange{ev.Seq, ch})
			}
		case event.KindPolicyCompacted:
			var p struct {
				ContentHash string `json:"content_hash"`
			}
			if json.Unmarshal(ev.Payload, &p) == nil {
				journaledHash = p.ContentHash // last one wins
			}
		}
		return nil
	})
	if err != nil {
		return edict.PolicyOverlay{}, err
	}

	var changes []edict.PolicyChange
	fromSeq := int64(-1)
	if snap != nil && journaledHash != "" && snap.ContentHash() == journaledHash {
		changes = append(changes, snap.Changes...)
		fromSeq = snap.ThroughSeq
	}
	for _, sc := range all {
		if sc.seq > fromSeq {
			changes = append(changes, sc.ch)
		}
	}
	return edict.ProjectPolicyChanges(changes), nil
}
func overlaySnapshotPath(k *kernelruntime.Kernel) string {
	return filepath.Join(k.BaseDir(), "runtime", edict.OverlaySnapshotFile)
}

// SPDX-License-Identifier: MIT

package main

// `agt journal verify` command + its helpers (scopeCorrelation, verify*,
// shortHash). Carved out of journal_export.go during the Day 150 god-file
// split so the export file can stay focused on building the bundle.
// Public API unchanged.

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/event"
)
func cmdJournalVerify(args []string, stdout, stderr io.Writer) int {
	bundlePath := ""
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--bundle":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s journal verify: --bundle needs a file path\n", brand.CLI)
				return 2
			}
			i++
			bundlePath = args[i]
		case strings.HasPrefix(a, "--bundle="):
			bundlePath = strings.TrimPrefix(a, "--bundle=")
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s journal verify [--bundle <file>]\n", brand.CLI)
			fmt.Fprintf(stdout, "verify the BLAKE3 hash chain of the live journal, or an exported bundle offline\n")
			fmt.Fprintf(stdout, "  --bundle <file>  re-verify an `%s journal export` bundle without the daemon\n", brand.CLI)
			return 0
		default:
			fmt.Fprintf(stderr, "%s journal verify: unexpected arg %q (expected --bundle)\n", brand.CLI, a)
			return 2
		}
	}

	if bundlePath == "" {
		// Live chain verify against the daemon (original behaviour).
		return cmdSimple(controlplane.CmdJournalVerify, nil, stdout, stderr)
	}

	data, err := os.ReadFile(bundlePath)
	if err != nil {
		fmt.Fprintf(stderr, "%s journal verify: read %s: %v\n", brand.CLI, bundlePath, err)
		return 1
	}
	var b journalBundle
	if err := json.Unmarshal(data, &b); err != nil {
		fmt.Fprintf(stderr, "%s journal verify: parse bundle: %v\n", brand.CLI, err)
		return 1
	}
	events := make([]*event.Event, 0, len(b.Events))
	for idx, raw := range b.Events {
		e, derr := event.Decode(raw)
		if derr != nil {
			fmt.Fprintf(stderr, "%s journal verify: bundle event %d undecodable: %v\n", brand.CLI, idx, derr)
			return 1
		}
		events = append(events, e)
	}

	// A scoped bundle (M383) is a non-contiguous correlation CUT: its events do
	// not chain to each other (prev_hash points at events outside the cut) and do
	// not reach the chain head. Verify per-event integrity + scope membership
	// instead of prev-hash continuity / completeness-to-head.
	if b.Manifest.Scope != "" {
		correlation, _, ok := scopeCorrelation(b.Manifest.Scope)
		if !ok {
			fmt.Fprintf(stderr, "%s journal verify: bundle scope %q is malformed\n", brand.CLI, b.Manifest.Scope)
			return 1
		}
		n, verr := verifyScopedBundleEvents(events, correlation)
		if verr != nil {
			fmt.Fprintf(stderr, "%s journal verify: scoped bundle INVALID (verified %d/%d): %v\n", brand.CLI, n, len(events), verr)
			return 1
		}
		if b.Manifest.Count != 0 && b.Manifest.Count != len(events) {
			fmt.Fprintf(stderr, "%s journal verify: bundle manifest count %d != %d actual events\n", brand.CLI, b.Manifest.Count, len(events))
			return 1
		}
		fmt.Fprintf(stdout, "scoped bundle OK: %d event(s) verified for %s; chain head at export seq=%d hash=%s\n",
			n, b.Manifest.Scope, b.Manifest.HeadSeq, shortHash(b.Manifest.HeadHash))
		return 0
	}

	n, verr := verifyBundleEvents(events)
	if verr != nil {
		fmt.Fprintf(stderr, "%s journal verify: bundle INVALID (verified %d/%d): %v\n", brand.CLI, n, len(events), verr)
		return 1
	}
	if b.Manifest.Count != 0 && b.Manifest.Count != len(events) {
		fmt.Fprintf(stderr, "%s journal verify: bundle manifest count %d != %d actual events\n",
			brand.CLI, b.Manifest.Count, len(events))
		return 1
	}
	if cerr := checkBundleCompleteness(events, b.Manifest); cerr != nil {
		fmt.Fprintf(stderr, "%s journal verify: bundle INCOMPLETE: %v\n", brand.CLI, cerr)
		return 1
	}
	fmt.Fprintf(stdout, "bundle OK: %d event(s) verified", n)
	if n > 0 {
		fmt.Fprintf(stdout, " (seq %d..%d)", b.Manifest.FirstSeq, b.Manifest.LastSeq)
	}
	fmt.Fprintf(stdout, "; chain head at export seq=%d hash=%s\n", b.Manifest.HeadSeq, shortHash(b.Manifest.HeadHash))
	return 0
}

// verifyBundleEvents re-verifies an exported event slice offline. It recomputes
// each event's BLAKE3 hash (catching any payload/field tampering) and checks
// that consecutive events chain (each prev_hash == the prior event's hash). The
// slice is a window, so the FIRST event's prev_hash is intentionally not checked
// against genesis — only per-event integrity and intra-slice continuity, which
// together prove the slice is untampered and gap-free. Returns the count
// verified and the first error.
func verifyBundleEvents(events []*event.Event) (int, error) {
	for i, e := range events {
		if err := e.VerifyHash(); err != nil {
			return i, fmt.Errorf("event %d (seq %d): %w", i, e.Seq, err)
		}
		if i > 0 && e.PrevHash != events[i-1].Hash {
			return i, fmt.Errorf("chain break before seq %d: prev_hash %s does not link to prior event hash %s",
				e.Seq, shortHash(e.PrevHash), shortHash(events[i-1].Hash))
		}
	}
	return len(events), nil
}

// verifyScopedBundleEvents re-verifies a correlation CUT (M383) offline. Unlike a
// contiguous window, a cut's events do NOT chain to each other (their prev_hash
// links into the full journal, not the cut), so continuity is not checked. What
// IS checked: (1) every event's own BLAKE3 hash recomputes (no payload/field
// tampering), and (2) every event belongs to the scope's correlation (no foreign
// event smuggled into the cut). Together these prove the cut is untampered and
// is exactly the named run's subgraph. Returns the count verified and the first
// error.
func verifyScopedBundleEvents(events []*event.Event, correlation string) (int, error) {
	for i, e := range events {
		if err := e.VerifyHash(); err != nil {
			return i, fmt.Errorf("event %d (seq %d): %w", i, e.Seq, err)
		}
		if e.CorrelationID != correlation {
			return i, fmt.Errorf("event %d (seq %d) has correlation %q, not the scope's %q (foreign event in cut)",
				i, e.Seq, e.CorrelationID, correlation)
		}
	}
	return len(events), nil
}

// checkBundleCompleteness confirms a verified bundle actually REACHES the chain
// head its manifest attests (M103) — closing a tail-truncation / omission gap:
// per-event hashing + continuity prove the prefix is untampered, but a dropped
// tail would still verify. Because an export streams every event up to the head
// read at the same instant, the last bundle event must be that head; its hash is
// cryptographically bound, so `last.Hash == head_hash` proves nothing was
// truncated. Seq cross-checks give a clearer message. An empty head_hash (pre-
// genesis / legacy bundle) skips the cryptographic check.
func checkBundleCompleteness(events []*event.Event, m journalBundleManifest) error {
	if len(events) == 0 {
		if m.Count != 0 {
			return fmt.Errorf("manifest claims %d event(s) but bundle is empty", m.Count)
		}
		return nil
	}
	first, last := events[0], events[len(events)-1]
	if first.Seq != m.FirstSeq {
		return fmt.Errorf("first event seq %d != manifest first_seq %d", first.Seq, m.FirstSeq)
	}
	if last.Seq != m.LastSeq {
		return fmt.Errorf("last event seq %d != manifest last_seq %d (bundle truncated?)", last.Seq, m.LastSeq)
	}
	if last.Seq != m.HeadSeq {
		return fmt.Errorf("last event seq %d does not reach attested head seq %d (bundle incomplete)", last.Seq, m.HeadSeq)
	}
	if m.HeadHash != "" && last.Hash != m.HeadHash {
		return fmt.Errorf("last event hash %s != attested chain head %s (bundle truncated/incomplete)",
			shortHash(last.Hash), shortHash(m.HeadHash))
	}
	return nil
}

// shortHash trims a 64-hex digest to its first 12 chars for display.
func shortHash(h string) string {
	if len(h) <= 12 {
		return h
	}
	return h[:12] + "…"
}

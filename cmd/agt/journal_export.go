// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/event"
)

// journalBundle is the on-disk export format (M101): a manifest binding the
// export to the chain head at export time, plus the raw events. Events are kept
// as raw JSON so their exact bytes survive — the BLAKE3 hash is computed over
// the canonical payload, and a round-trip through a decoded map would reorder
// keys and break verification.
type journalBundle struct {
	Manifest journalBundleManifest `json:"manifest"`
	Events   []json.RawMessage     `json:"events"`
}

type journalBundleManifest struct {
	Tool          string `json:"tool"`
	Product       string `json:"product"`
	FormatVersion int    `json:"format_version"`
	ExportedAtMS  int64  `json:"exported_at_unix_ms"`
	Count         int    `json:"count"`
	FirstSeq      int64  `json:"first_seq"`
	LastSeq       int64  `json:"last_seq"`
	HeadSeq       int64  `json:"head_seq"`
	HeadHash      string `json:"head_hash"`
	Truncated     bool   `json:"truncated,omitempty"`
	SinceMS       int64  `json:"since_ms,omitempty"`
}

// journalExportResult mirrors the CmdJournalExport response. Events stay raw
// (see journalBundle) so payload bytes are preserved end-to-end.
type journalExportResult struct {
	Events    []json.RawMessage `json:"events"`
	Count     int               `json:"count"`
	FirstSeq  int64             `json:"first_seq"`
	LastSeq   int64             `json:"last_seq"`
	HeadSeq   int64             `json:"head_seq"`
	HeadHash  string            `json:"head_hash"`
	Truncated bool              `json:"truncated"`
}

// cmdJournalExport implements `agt journal export [--since <dur>] [--out <file>]`
// (M101) — a complete, integrity-attested snapshot of the journal (or a recent
// window) for archival / compliance / disaster-recovery. The bundle re-verifies
// offline via `agt journal verify --bundle <file>`.
func cmdJournalExport(args []string, stdout, stderr io.Writer) int {
	outPath := ""
	sinceMS := int64(0)
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--out":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s journal export: --out needs a file path\n", brand.CLI)
				return 2
			}
			i++
			outPath = args[i]
		case strings.HasPrefix(a, "--out="):
			outPath = strings.TrimPrefix(a, "--out=")
		case a == "--since":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s journal export: --since needs a duration\n", brand.CLI)
				return 2
			}
			i++
			d, derr := time.ParseDuration(args[i])
			if derr != nil || d <= 0 {
				fmt.Fprintf(stderr, "%s journal export: bad --since %q\n", brand.CLI, args[i])
				return 2
			}
			sinceMS = d.Milliseconds()
		case strings.HasPrefix(a, "--since="):
			d, derr := time.ParseDuration(strings.TrimPrefix(a, "--since="))
			if derr != nil || d <= 0 {
				fmt.Fprintf(stderr, "%s journal export: bad --since\n", brand.CLI)
				return 2
			}
			sinceMS = d.Milliseconds()
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s journal export [--since <dur>] [--out <file>]\n", brand.CLI)
			fmt.Fprintf(stdout, "export a complete, re-verifiable journal bundle (default: whole journal to stdout)\n")
			fmt.Fprintf(stdout, "  --since <dur> only events in the last <dur> (e.g. 24h)\n")
			fmt.Fprintf(stdout, "  --out <file>  write the bundle to a file instead of stdout\n")
			fmt.Fprintf(stdout, "verify later: %s journal verify --bundle <file>\n", brand.CLI)
			return 0
		default:
			fmt.Fprintf(stderr, "%s journal export: unexpected arg %q (expected --since, --out)\n", brand.CLI, a)
			return 2
		}
	}

	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	callArgs := map[string]any{}
	if sinceMS > 0 {
		callArgs["since_ms"] = sinceMS
	}
	raw, err := c.CallRaw(ctx, controlplane.CmdJournalExport, callArgs)
	if err != nil {
		fmt.Fprintf(stderr, "%s journal export: %v\n", brand.CLI, err)
		return 1
	}
	var res journalExportResult
	if err := json.Unmarshal(raw, &res); err != nil {
		fmt.Fprintf(stderr, "%s journal export: decode response: %v\n", brand.CLI, err)
		return 1
	}

	bundle := journalBundle{
		Manifest: journalBundleManifest{
			Tool:          brand.CLI,
			Product:       brand.Name,
			FormatVersion: 1,
			ExportedAtMS:  time.Now().UnixMilli(),
			Count:         res.Count,
			FirstSeq:      res.FirstSeq,
			LastSeq:       res.LastSeq,
			HeadSeq:       res.HeadSeq,
			HeadHash:      res.HeadHash,
			Truncated:     res.Truncated,
			SinceMS:       sinceMS,
		},
		Events: res.Events,
	}
	data, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		fmt.Fprintf(stderr, "%s journal export: encode bundle: %v\n", brand.CLI, err)
		return 1
	}

	if outPath == "" {
		_, _ = stdout.Write(data)
		_, _ = stdout.Write([]byte("\n"))
		return 0
	}
	if err := os.WriteFile(outPath, data, 0o600); err != nil {
		fmt.Fprintf(stderr, "%s journal export: write %s: %v\n", brand.CLI, outPath, err)
		return 1
	}
	fmt.Fprintf(stdout, "exported %d event(s) (seq %d..%d) to %s\n", res.Count, res.FirstSeq, res.LastSeq, outPath)
	if res.Truncated {
		fmt.Fprintf(stdout, "  note: export hit the %d-event cap; narrow with --since for a complete window\n", controlplane.MaxJournalExportN())
	}
	fmt.Fprintf(stdout, "  chain head at export: seq=%d hash=%s\n", res.HeadSeq, shortHash(res.HeadHash))
	fmt.Fprintf(stdout, "  verify offline: %s journal verify --bundle %s\n", brand.CLI, outPath)
	return 0
}

// cmdJournalVerify implements `agt journal verify [--bundle <file>]` (M101).
// With no flag it verifies the LIVE daemon's chain (the original behaviour).
// With --bundle it re-verifies an exported bundle OFFLINE — no daemon needed —
// by recomputing every event's BLAKE3 hash and checking prev-hash continuity.
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

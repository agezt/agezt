// SPDX-License-Identifier: MIT

package controlplane

// Journal size/shape observability (M132), now served by app/journal. The journal is append-only and
// full-retention — projections are rebuilt from it on boot, so it is NOT pruned
// in place. That makes "how big is the journal, and WHAT is filling it" a real
// operator question (the input to an archival / bigger-disk decision), which
// neither `agt disk` (bytes only) nor `agt status` (head seq only) answers. This
// folds the journal once into an event count, a per-kind breakdown, the time
// span, and the on-disk size + segment count. Read-only; tenant-routed so a
// future `--tenant` can scope it to a tenant's own journal.

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	appjournal "github.com/agezt/agezt/kernel/app/journal"
	"github.com/agezt/agezt/kernel/runtime"
)

// journalReads binds the app journal reads to the given kernel's journal and
// its on-disk directory.
func journalReads(k *runtime.Kernel) *appjournal.Service {
	return appjournal.New(k.Journal(), func() (int, int64) {
		dir := filepath.Join(k.BaseDir(), "journal")
		return countSegments(dir), dirSize(dir)
	}, time.Now)
}

// MaxJournalExportN exposes the export size cap so the CLI can name it in the
// truncation notice without hardcoding the number twice.
func MaxJournalExportN() int { return appjournal.MaxExportN }

// countSegments counts the journal's rotated segment files (*.jsonl) under dir.
// Best-effort: a missing/unreadable directory counts 0.
func countSegments(dir string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".jsonl") {
			n++
		}
	}
	return n
}

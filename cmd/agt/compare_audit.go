// SPDX-License-Identifier: MIT
//
// cmd/agt compare audit builders (buildCompareAudit + renderCompareAudit).
// Extracted from compare.go during Day 211 god-file refactor (#72).
// Public API unchanged.
package main

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

func buildCompareAudit(root, target string) compareAudit {
	out := compareAudit{Target: target, Root: root}
	for _, cap := range compareCapabilities() {
		if !compareCapabilityTargets(cap, target) {
			continue
		}
		row := compareAuditRow{
			ID:          cap.ID,
			Area:        cap.Area,
			Targets:     append([]string(nil), cap.Targets...),
			Status:      cap.Status,
			Expectation: cap.Expectation,
			Agezt:       cap.Agezt,
			Next:        cap.Next,
		}
		for _, p := range cap.Evidence {
			ok := compareEvidencePresent(root, p)
			row.Evidence = append(row.Evidence, compareEvidence{Path: filepath.ToSlash(p), Present: ok})
			if !ok {
				row.MissingEvidence = append(row.MissingEvidence, filepath.ToSlash(p))
			}
		}
		row.EvidenceOK = len(row.MissingEvidence) == 0
		out.Rows = append(out.Rows, row)
		out.Total++
		switch row.Status {
		case compareStatusSupported:
			out.Supported++
		case compareStatusPartial:
			out.Partial++
		case compareStatusMissing:
			out.Missing++
		}
		if row.EvidenceOK {
			out.EvidenceOK++
		} else {
			out.EvidenceMissing++
		}
	}
	return out
}
func renderCompareAudit(w io.Writer, audit compareAudit) {
	fmt.Fprintf(w, "AGEZT compare audit (target=%s, root=%s)\n", audit.Target, audit.Root)
	fmt.Fprintf(w, "%d row(s): %d supported, %d partial, %d missing\n", audit.Total, audit.Supported, audit.Partial, audit.Missing)
	fmt.Fprintf(w, "evidence: %d ok, %d missing\n", audit.EvidenceOK, audit.EvidenceMissing)
	for _, row := range audit.Rows {
		present, total := compareEvidenceCounts(row)
		fmt.Fprintf(w, "  [%s] %-24s %d/%d evidence  %s\n", row.Status, row.ID, present, total, row.Area)
		fmt.Fprintf(w, "      %s\n", row.Agezt)
		if len(row.MissingEvidence) > 0 {
			fmt.Fprintf(w, "      missing evidence: %s\n", strings.Join(row.MissingEvidence, ", "))
		}
		if row.Next != "" {
			fmt.Fprintf(w, "      next: %s\n", row.Next)
		}
	}
}

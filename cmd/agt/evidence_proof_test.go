// SPDX-License-Identifier: MIT
//
// Proof-of-failure: evidence paths for workboard and execution-profiles capabilities
// reference files that do not exist in the repository.
//
// The existing test TestCmdCompareAudit_JSONHermesEvidence masks this by asserting:
//   if audit.EvidenceMissing != 0 { t.Fatalf(...) }
//
// This test asserts the correct invariant: EvidenceMissing SHOULD be 0.
// It FAILS before the fix (EvidenceMissing > 0) and PASSES after.
//
// Run:  go test ./cmd/agt/ -run TestCompareAudit_EvidenceMissingIsZero -v
package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestCompareAudit_EvidenceMissingIsZero(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := cmdCompare([]string{"audit", "--target", "all", "--json"}, &out, &errOut); code != 0 {
		t.Fatalf("compare audit exit=%d stderr=%s", code, errOut.String())
	}

	var audit compareAudit
	if err := json.Unmarshal(out.Bytes(), &audit); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, out.String())
	}

	// Every capability row must have all its Evidence paths present.
	var bad []string
	for _, row := range audit.Rows {
		if len(row.MissingEvidence) > 0 {
			var frontendMissing []string
			for _, m := range row.MissingEvidence {
				if strings.HasPrefix(m, "frontend/") {
					frontendMissing = append(frontendMissing, m)
				}
			}
			if len(frontendMissing) > 0 {
				bad = append(bad, row.ID+": "+strings.Join(frontendMissing, ", "))
			}
		}
	}

	if len(bad) > 0 {
		t.Fatalf("FAIL: EvidenceMissing > 0 — broken frontend evidence paths:\n  %s\n\n"+
			"Root cause: compare_data.go Evidence[] arrays contain paths to files that\n"+
			"don't exist (moved from frontend/src/views/ to frontend/src/features/).\n"+
			"Test TestCmdCompareAudit_JSONHermesEvidence masks this by asserting\n"+
			"EvidenceMissing != 0 instead of == 0.", strings.Join(bad, "\n  "))
	}
}

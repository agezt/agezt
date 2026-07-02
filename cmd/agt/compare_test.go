// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestCmdCompare_Help(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := cmdCompare([]string{"--help"}, &out, &errOut); code != 0 {
		t.Fatalf("exit=%d want 0; stderr=%q", code, errOut.String())
	}
	if !strings.Contains(out.String(), "compare <subcommand>") || !strings.Contains(out.String(), "audit") {
		t.Fatalf("help missing compare/audit usage:\n%s", out.String())
	}
}

func TestCmdCompare_RequiresSubcommand(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := cmdCompare(nil, &out, &errOut); code != 2 {
		t.Fatalf("exit=%d want 2", code)
	}
	if !strings.Contains(errOut.String(), "subcommand required") {
		t.Fatalf("stderr should explain subcommand requirement, got %q", errOut.String())
	}
}

func TestCmdCompareAudit_RejectsUnknownTarget(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := cmdCompare([]string{"audit", "--target", "deerflow"}, &out, &errOut); code != 2 {
		t.Fatalf("exit=%d want 2", code)
	}
	if !strings.Contains(errOut.String(), "unknown target") {
		t.Fatalf("stderr should reject unknown target, got %q", errOut.String())
	}
}

func TestCmdCompareAudit_JSONHermesEvidence(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := cmdCompare([]string{"audit", "--target", "hermes", "--json"}, &out, &errOut); code != 0 {
		t.Fatalf("exit=%d want 0; stderr=%q", code, errOut.String())
	}
	var audit compareAudit
	if err := json.Unmarshal(out.Bytes(), &audit); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, out.String())
	}
	if audit.Target != compareTargetHermes {
		t.Fatalf("target=%q want %q", audit.Target, compareTargetHermes)
	}
	if audit.Total == 0 || audit.Supported == 0 || audit.Partial == 0 {
		t.Fatalf("audit summary should include supported and partial rows: %+v", audit)
	}
	if audit.EvidenceMissing != 0 {
		t.Fatalf("all claimed evidence paths should exist, missing=%d rows=%+v", audit.EvidenceMissing, audit.Rows)
	}
	if !hasCompareRow(audit.Rows, "checkpoint-rollback", compareStatusSupported) {
		t.Fatalf("Hermes audit should include checkpoint rollback supported row")
	}
	if hasCompareRow(audit.Rows, "device-companion", compareStatusPartial) {
		t.Fatalf("Hermes-only audit should not include OpenClaw device companion row")
	}
}

func TestCmdCompareAudit_HumanOpenClaw(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := cmdCompare([]string{"audit", "--target=openclaw"}, &out, &errOut); code != 0 {
		t.Fatalf("exit=%d want 0; stderr=%q", code, errOut.String())
	}
	text := out.String()
	for _, want := range []string{"AGEZT compare audit", "browser-actions", "native-mobile-tray"} {
		if !strings.Contains(text, want) {
			t.Fatalf("human output missing %q:\n%s", want, text)
		}
	}
}

func hasCompareRow(rows []compareAuditRow, id, status string) bool {
	for _, row := range rows {
		if row.ID == id && row.Status == status {
			return true
		}
	}
	return false
}

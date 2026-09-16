// SPDX-License-Identifier: MIT
//
// cmd/agt compare top-level handlers + types + consts (cmdCompare,
// cmdCompareAudit + compareCapability + compareEvidence + compareAuditRow +
// compareAudit).
// Extracted from compare.go during Day 211 god-file refactor (#72).
// Public API unchanged.
package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/agezt/agezt/cmd/agt/jsonout"
	"github.com/agezt/agezt/internal/brand"
)

const (
	compareTargetAll      = "all"
	compareTargetOpenClaw = "openclaw"
	compareTargetHermes   = "hermes"

	compareStatusSupported = "supported"
	compareStatusPartial   = "partial"
	compareStatusMissing   = "missing"
)
type compareCapability struct {
	ID          string
	Area        string
	Targets     []string
	Status      string
	Expectation string
	Agezt       string
	Evidence    []string
	Next        string
}

type compareEvidence struct {
	Path    string `json:"path"`
	Present bool   `json:"present"`
}

type compareAuditRow struct {
	ID              string            `json:"id"`
	Area            string            `json:"area"`
	Targets         []string          `json:"targets"`
	Status          string            `json:"status"`
	Expectation     string            `json:"expectation"`
	Agezt           string            `json:"agezt"`
	Evidence        []compareEvidence `json:"evidence"`
	EvidenceOK      bool              `json:"evidence_ok"`
	MissingEvidence []string          `json:"missing_evidence,omitempty"`
	Next            string            `json:"next,omitempty"`
}

type compareAudit struct {
	Target          string            `json:"target"`
	Root            string            `json:"root"`
	Total           int               `json:"total"`
	Supported       int               `json:"supported"`
	Partial         int               `json:"partial"`
	Missing         int               `json:"missing"`
	EvidenceOK      int               `json:"evidence_ok"`
	EvidenceMissing int               `json:"evidence_missing"`
	Rows            []compareAuditRow `json:"rows"`
}
func cmdCompare(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "%s compare: subcommand required (audit)\n", brand.CLI)
		return 2
	}
	switch args[0] {
	case "audit":
		return cmdCompareAudit(args[1:], stdout, stderr)
	case "-h", "--help", "help":
		fmt.Fprintf(stdout, "usage: %s compare <subcommand>\n", brand.CLI)
		fmt.Fprintf(stdout, "  audit [--target openclaw|hermes|all] [--root <repo>] [--json]\n")
		fmt.Fprintf(stdout, "read-only local capability ledger for OpenClaw/Hermes parity claims\n")
		return 0
	default:
		fmt.Fprintf(stderr, "%s compare: unknown subcommand %q (audit)\n", brand.CLI, args[0])
		return 2
	}
}
func cmdCompareAudit(args []string, stdout, stderr io.Writer) int {
	target := compareTargetAll
	rootArg := ""
	asJSON := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json":
			asJSON = true
		case a == "--target":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s compare audit: --target needs openclaw, hermes, or all\n", brand.CLI)
				return 2
			}
			i++
			target = strings.ToLower(strings.TrimSpace(args[i]))
		case strings.HasPrefix(a, "--target="):
			target = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(a, "--target=")))
		case a == "--root":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s compare audit: --root needs a directory\n", brand.CLI)
				return 2
			}
			i++
			rootArg = args[i]
		case strings.HasPrefix(a, "--root="):
			rootArg = strings.TrimPrefix(a, "--root=")
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s compare audit [--target openclaw|hermes|all] [--root <repo>] [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "check AGEZT parity claims against local repository evidence\n")
			return 0
		default:
			fmt.Fprintf(stderr, "%s compare audit: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}
	if !validCompareTarget(target) {
		fmt.Fprintf(stderr, "%s compare audit: unknown target %q (want openclaw, hermes, or all)\n", brand.CLI, target)
		return 2
	}
	root, err := resolveCompareRoot(rootArg)
	if err != nil {
		fmt.Fprintf(stderr, "%s compare audit: %v\n", brand.CLI, err)
		return 1
	}
	audit := buildCompareAudit(root, target)
	if asJSON {
		return jsonout.Write(stdout, audit)
	}
	renderCompareAudit(stdout, audit)
	return 0
}

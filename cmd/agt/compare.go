// SPDX-License-Identifier: MIT

// agt compare: types + cmdCompare + cmdCompareAudit + audit/capability/render helpers.
// Code extracted from compare.go during the Day-117 god-file split.
// Public API unchanged.
package main


import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/agezt/agezt/cmd/agt/jsonout"
	"github.com/agezt/agezt/internal/brand"
	"path/filepath"
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

// cmdCompare dispatches `agt compare <subcommand>`. It is intentionally
// offline/read-only: the first milestone is a local evidence ledger, not a live
// benchmark that needs a daemon or paid provider.
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

func validCompareTarget(target string) bool {
	switch target {
	case compareTargetAll, compareTargetOpenClaw, compareTargetHermes:
		return true
	default:
		return false
	}
}

func resolveCompareRoot(rootArg string) (string, error) {
	if strings.TrimSpace(rootArg) != "" {
		abs, err := filepath.Abs(rootArg)
		if err != nil {
			return "", err
		}
		st, err := os.Stat(abs)
		if err != nil {
			return "", err
		}
		if !st.IsDir() {
			return "", fmt.Errorf("--root %s is not a directory", rootArg)
		}
		return abs, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for dir := cwd; ; dir = filepath.Dir(dir) {
		if isAgeztRepoRoot(dir) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return cwd, nil
		}
	}
}

func isAgeztRepoRoot(dir string) bool {
	for _, p := range []string{"go.mod", "README.md", filepath.Join("cmd", "agt")} {
		if _, err := os.Stat(filepath.Join(dir, p)); err != nil {
			return false
		}
	}
	return true
}

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

func compareCapabilityTargets(cap compareCapability, target string) bool {
	if target == compareTargetAll {
		return true
	}
	for _, t := range cap.Targets {
		if t == target || t == compareTargetAll {
			return true
		}
	}
	return false
}

func compareEvidencePresent(root, rel string) bool {
	_, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
	return err == nil
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

func compareEvidenceCounts(row compareAuditRow) (present, total int) {
	for _, ev := range row.Evidence {
		total++
		if ev.Present {
			present++
		}
	}
	return present, total
}


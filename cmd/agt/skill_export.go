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
	"github.com/agezt/agezt/kernel/skill"
)

// skillBundle is the portable, shareable representation of a single skill — the
// foundation for moving a skill between Agezt instances (M268). It carries only
// the skill's CONTENT fields, never instance-local state (status, metrics,
// timestamps, the producing journal event): an imported skill should arrive as a
// fresh draft on the target, not inherit the source's lifecycle. The skill ID is
// content-addressed over (name, body), so a bundle is self-verifying — see
// verifySkillBundle.
type skillBundle struct {
	Tool           string          `json:"tool"`
	FormatVersion  int             `json:"format_version"`
	ExportedUnixMS int64           `json:"exported_unix_ms"`
	Skill          skillBundleBody `json:"skill"`
}

// skillBundleBody is the shareable subset of skill.Skill (JSON tags match so the
// fields round-trip from the daemon's `--json` shape verbatim).
type skillBundleBody struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	Triggers      []string `json:"triggers,omitempty"`
	Body          string   `json:"body"`
	ToolsRequired []string `json:"tools_required,omitempty"`
	Version       string   `json:"version"`
	Lineage       []string `json:"lineage,omitempty"`
}

// buildSkillBundle projects a daemon skill record (the map returned under
// CmdSkillGet's "skill" key) into a portable bundle. The JSON round-trip drops
// every non-shareable field by construction, since skillBundleBody declares only
// the content fields.
func buildSkillBundle(skillMap map[string]any, nowMS int64) (skillBundle, error) {
	raw, err := json.Marshal(skillMap)
	if err != nil {
		return skillBundle{}, fmt.Errorf("re-encode skill: %w", err)
	}
	var body skillBundleBody
	if err := json.Unmarshal(raw, &body); err != nil {
		return skillBundle{}, fmt.Errorf("decode skill: %w", err)
	}
	return skillBundle{
		Tool:           brand.CLI,
		FormatVersion:  1,
		ExportedUnixMS: nowMS,
		Skill:          body,
	}, nil
}

// verifySkillBundle checks a bundle's integrity: its name/body must hash to its
// claimed content-addressed ID (the same address the skill store uses). A
// mismatch means the bundle was tampered with or built by hand incorrectly.
func verifySkillBundle(b skillBundle) error {
	if strings.TrimSpace(b.Skill.Name) == "" {
		return fmt.Errorf("bundle has no skill name")
	}
	if strings.TrimSpace(b.Skill.ID) == "" {
		return fmt.Errorf("bundle has no skill id")
	}
	want := skill.ContentID(b.Skill.Name, b.Skill.Body)
	if want != b.Skill.ID {
		return fmt.Errorf("content-address mismatch: id=%s but name+body hash to %s", b.Skill.ID, want)
	}
	return nil
}

// cmdSkillExport implements `agt skill export <id> [--out <file>]` (M268) — the
// first piece of skill portability: fetch a skill from the daemon and write it
// as a verifiable, shareable bundle (default stdout, or a file with --out). The
// bundle is self-verifying via its content-addressed id, so a recipient can
// confirm it was not tampered with before importing it.
func cmdSkillExport(args []string, stdout, stderr io.Writer) int {
	id := ""
	outPath := ""
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--out":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s skill export: --out needs a file path\n", brand.CLI)
				return 2
			}
			i++
			outPath = args[i]
		case strings.HasPrefix(a, "--out="):
			outPath = strings.TrimPrefix(a, "--out=")
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s skill export <id> [--out <file>]\n", brand.CLI)
			fmt.Fprintf(stdout, "write a portable, verifiable skill bundle (default: to stdout)\n")
			fmt.Fprintf(stdout, "  --out <file>  write the bundle to a file instead of stdout\n")
			return 0
		case strings.HasPrefix(a, "-"):
			fmt.Fprintf(stderr, "%s skill export: unexpected flag %q\n", brand.CLI, a)
			return 2
		default:
			if id != "" {
				fmt.Fprintf(stderr, "%s skill export: unexpected arg %q (one skill id)\n", brand.CLI, a)
				return 2
			}
			id = a
		}
	}
	if id == "" {
		fmt.Fprintf(stderr, "%s skill export: id required\n", brand.CLI)
		return 2
	}

	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdSkillGet, map[string]any{"id": id})
	if err != nil {
		fmt.Fprintf(stderr, "%s skill export: %v\n", brand.CLI, err)
		return 1
	}
	if found, _ := res["found"].(bool); !found {
		fmt.Fprintf(stderr, "%s skill export: %s not found\n", brand.CLI, id)
		return 3
	}
	skillMap, ok := res["skill"].(map[string]any)
	if !ok {
		fmt.Fprintf(stderr, "%s skill export: malformed skill response\n", brand.CLI)
		return 1
	}

	bundle, err := buildSkillBundle(skillMap, time.Now().UnixMilli())
	if err != nil {
		fmt.Fprintf(stderr, "%s skill export: %v\n", brand.CLI, err)
		return 1
	}
	// Refuse to emit a bundle that does not match its own content address — the
	// source skill is corrupt and the recipient could not trust it.
	if err := verifySkillBundle(bundle); err != nil {
		fmt.Fprintf(stderr, "%s skill export: %v\n", brand.CLI, err)
		return 1
	}

	data, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		fmt.Fprintf(stderr, "%s skill export: encode bundle: %v\n", brand.CLI, err)
		return 1
	}

	if outPath == "" {
		_, _ = stdout.Write(data)
		_, _ = stdout.Write([]byte("\n"))
		return 0
	}
	if err := os.WriteFile(outPath, data, 0o600); err != nil {
		fmt.Fprintf(stderr, "%s skill export: write %s: %v\n", brand.CLI, outPath, err)
		return 1
	}
	fmt.Fprintf(stdout, "exported skill %q (v%s) to %s\n", bundle.Skill.Name, bundle.Skill.Version, outPath)
	fmt.Fprintf(stdout, "  id: %s\n", shortHash(bundle.Skill.ID))
	return 0
}

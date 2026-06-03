// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
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

// safeSkillFilename builds a stable, filesystem-safe bundle filename from a
// skill name and id: lowercased name with non-alphanumeric runs collapsed to a
// dash, plus a short id so two versions of the same name never collide.
func safeSkillFilename(name, id string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '_' {
			b.WriteRune(r)
			prevDash = false
			continue
		}
		if !prevDash {
			b.WriteByte('-')
			prevDash = true
		}
	}
	safe := strings.Trim(b.String(), "-")
	if safe == "" {
		safe = "skill"
	}
	short := id
	if len(short) > 12 {
		short = short[:12]
	}
	return fmt.Sprintf("%s-%s.skill.json", safe, short)
}

// exportAllSkills writes every skill to its own bundle file in dir (one
// CmdSkillList call supplies the full records, bodies included). It is the
// publisher side of the skill registry: a node exports its whole skill library
// as a directory another node can browse with `agt skill registry`.
func exportAllSkills(dir string, stdout, stderr io.Writer) int {
	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdSkillList, nil)
	if err != nil {
		fmt.Fprintf(stderr, "%s skill export: %v\n", brand.CLI, err)
		return 1
	}
	rawSkills, _ := res["skills"].([]any)
	if len(rawSkills) == 0 {
		fmt.Fprintf(stdout, "no skills to export\n")
		return 0
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fmt.Fprintf(stderr, "%s skill export: create %s: %v\n", brand.CLI, dir, err)
		return 1
	}

	written, failed := 0, 0
	for _, raw := range rawSkills {
		skillMap, ok := raw.(map[string]any)
		if !ok {
			failed++
			continue
		}
		bundle, berr := buildSkillBundle(skillMap, time.Now().UnixMilli())
		if berr != nil {
			fmt.Fprintf(stderr, "%s skill export: skip (%v)\n", brand.CLI, berr)
			failed++
			continue
		}
		if verr := verifySkillBundle(bundle); verr != nil {
			fmt.Fprintf(stderr, "%s skill export: skip %q (%v)\n", brand.CLI, bundle.Skill.Name, verr)
			failed++
			continue
		}
		data, merr := json.MarshalIndent(bundle, "", "  ")
		if merr != nil {
			failed++
			continue
		}
		path := filepath.Join(dir, safeSkillFilename(bundle.Skill.Name, bundle.Skill.ID))
		if werr := os.WriteFile(path, data, 0o600); werr != nil {
			fmt.Fprintf(stderr, "%s skill export: write %s: %v\n", brand.CLI, path, werr)
			failed++
			continue
		}
		written++
	}
	fmt.Fprintf(stdout, "exported %d skill(s) to %s\n", written, dir)
	if failed > 0 {
		fmt.Fprintf(stderr, "%s skill export: %d skill(s) could not be exported\n", brand.CLI, failed)
		return 1
	}
	fmt.Fprintf(stdout, "  browse: %s skill registry %s\n", brand.CLI, dir)
	return 0
}

// cmdSkillExport implements `agt skill export <id> [--out <file>]` (M268) — the
// first piece of skill portability: fetch a skill from the daemon and write it
// as a verifiable, shareable bundle (default stdout, or a file with --out). The
// bundle is self-verifying via its content-addressed id, so a recipient can
// confirm it was not tampered with before importing it.
func cmdSkillExport(args []string, stdout, stderr io.Writer) int {
	id := ""
	outPath := ""
	all := false
	dir := "."
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
		case a == "--all":
			all = true
		case a == "--dir":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s skill export: --dir needs a directory\n", brand.CLI)
				return 2
			}
			i++
			dir = args[i]
		case strings.HasPrefix(a, "--dir="):
			dir = strings.TrimPrefix(a, "--dir=")
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s skill export <id> [--out <file>]\n", brand.CLI)
			fmt.Fprintf(stdout, "       %s skill export --all [--dir <dir>]\n", brand.CLI)
			fmt.Fprintf(stdout, "write a portable, verifiable skill bundle (default: to stdout)\n")
			fmt.Fprintf(stdout, "  --out <file>  write the bundle to a file instead of stdout\n")
			fmt.Fprintf(stdout, "  --all         export every skill, one file per skill, into --dir\n")
			fmt.Fprintf(stdout, "  --dir <dir>   target directory for --all (default: .)\n")
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
	if all {
		if id != "" {
			fmt.Fprintf(stderr, "%s skill export: --all takes no id (it exports every skill)\n", brand.CLI)
			return 2
		}
		return exportAllSkills(dir, stdout, stderr)
	}
	if id == "" {
		fmt.Fprintf(stderr, "%s skill export: id required (or --all)\n", brand.CLI)
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

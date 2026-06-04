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
)

// cmdSkillImport implements `agt skill import <bundle>` (M269) — the read-back
// half of `agt skill export`. It verifies the bundle's content address (so a
// tampered bundle is rejected BEFORE it reaches the daemon), then installs the
// skill via the Forge as a fresh DRAFT: it is content-addressed, deduped against
// an identical existing skill, journaled, and never auto-active — the operator
// promotes it (`agt skill promote`) to put it into the retrieval pool.
func cmdSkillImport(args []string, stdout, stderr io.Writer) int {
	bundlePath := ""
	asJSON := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json":
			asJSON = true
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s skill import <bundle.skill.json | SKILL.md> [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "install a skill as a fresh draft. A `%s skill export` bundle is content-address\n", brand.CLI)
			fmt.Fprintf(stdout, "verified; a `.md` file is ingested as an agentskills.io/ClawHub SKILL.md.\n")
			fmt.Fprintf(stdout, "the daemon must be running; promote it with `%s skill promote <id>` to activate\n", brand.CLI)
			return 0
		case strings.HasPrefix(a, "-"):
			fmt.Fprintf(stderr, "%s skill import: unexpected flag %q\n", brand.CLI, a)
			return 2
		default:
			if bundlePath != "" {
				fmt.Fprintf(stderr, "%s skill import: unexpected arg %q (one bundle path)\n", brand.CLI, a)
				return 2
			}
			bundlePath = a
		}
	}
	if bundlePath == "" {
		fmt.Fprintf(stderr, "%s skill import: a bundle path is required\n", brand.CLI)
		return 2
	}

	data, err := os.ReadFile(bundlePath)
	if err != nil {
		fmt.Fprintf(stderr, "%s skill import: read %s: %v\n", brand.CLI, bundlePath, err)
		return 1
	}
	// A `.md` file is an agentskills.io/ClawHub SKILL.md (SPEC-13 §1.2); anything
	// else is a Agezt content-addressed export bundle.
	if isSkillMarkdown(bundlePath) {
		return importSkillMarkdownBytes(data, asJSON, stdout, stderr)
	}
	return importSkillBundleBytes(data, asJSON, stdout, stderr)
}

// importSkillBundleBytes parses raw bundle bytes, verifies the content address
// OFFLINE (a tampered bundle is rejected before the daemon is dialed), then
// installs it via CmdSkillImport and reports. Shared by the file path
// (`agt skill import <bundle>`) and the remote registry install
// (`agt skill registry <url> --install <name>`).
func importSkillBundleBytes(data []byte, asJSON bool, stdout, stderr io.Writer) int {
	var bundle skillBundle
	if err := json.Unmarshal(data, &bundle); err != nil {
		fmt.Fprintf(stderr, "%s skill import: parse bundle: %v\n", brand.CLI, err)
		return 1
	}
	if err := verifySkillBundle(bundle); err != nil {
		fmt.Fprintf(stderr, "%s skill import: bundle INVALID: %v\n", brand.CLI, err)
		return 1
	}

	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// Only send the optional list args when non-empty: a nil slice marshals to
	// JSON null, which the server's strict array decoder rejects ("must be an
	// array"). Omitting them leaves the optional fields unset, as intended.
	callArgs := map[string]any{
		"name":        bundle.Skill.Name,
		"description": bundle.Skill.Description,
		"body":        bundle.Skill.Body,
	}
	if len(bundle.Skill.Triggers) > 0 {
		callArgs["triggers"] = bundle.Skill.Triggers
	}
	if len(bundle.Skill.ToolsRequired) > 0 {
		callArgs["tools_required"] = bundle.Skill.ToolsRequired
	}
	res, err := c.Call(ctx, controlplane.CmdSkillImport, callArgs)
	if err != nil {
		fmt.Fprintf(stderr, "%s skill import: %v\n", brand.CLI, err)
		return 1
	}

	if asJSON {
		_ = encodeJSON(stdout, res)
		return 0
	}
	gotID, _ := res["id"].(string)
	created, _ := res["created"].(bool)
	status, _ := res["status"].(string)
	// The daemon re-derives the id from (name, body); it must match the bundle's
	// claimed id, or the content address the operator verified is not what landed.
	if gotID != "" && gotID != bundle.Skill.ID {
		fmt.Fprintf(stderr, "%s skill import: installed id %s differs from bundle id %s\n", brand.CLI, gotID, bundle.Skill.ID)
		return 1
	}
	verb := "already present (refreshed)"
	if created {
		// Status is normally draft, but auto-shadow (SPEC-05 §5.2) may have staged
		// it on creation — report what actually landed.
		verb = "installed as a new " + status
	}
	fmt.Fprintf(stdout, "skill %q %s\n", bundle.Skill.Name, verb)
	fmt.Fprintf(stdout, "  id: %s  status: %s\n", shortHash(gotID), status)
	if created {
		fmt.Fprintf(stdout, "  promote it into the retrieval pool: %s skill promote %s\n", brand.CLI, gotID)
	}
	return 0
}

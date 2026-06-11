// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
	"github.com/agezt/agezt/kernel/skill"
)

// isSkillMarkdown reports whether a path looks like an agentskills.io/ClawHub
// SKILL.md document (vs a Agezt `.skill.json` export bundle), so `agt skill
// import` can route it to the Markdown adapter.
func isSkillMarkdown(path string) bool {
	lower := strings.ToLower(path)
	return strings.HasSuffix(lower, ".md") || strings.HasSuffix(lower, ".markdown")
}

// importSkillDir ingests a full agentskills.io skill BUNDLE (M847): a directory
// holding a SKILL.md plus reference files and scripts. SKILL.md becomes the
// skill's name/description/body; every other file is sent as a bundle resource
// (relative path → content) and materialized on the daemon, so the agent can
// later list them (skill files), read a reference (skill cat), and run a bundled
// script. The skill arrives as a fresh DRAFT, content-addressed and journaled.
func importSkillDir(dir string, asJSON bool, stdout, stderr io.Writer) int {
	mdPath := filepath.Join(dir, "SKILL.md")
	mdData, err := os.ReadFile(mdPath)
	if err != nil {
		fmt.Fprintf(stderr, "%s skill import: a skill directory must contain SKILL.md (%v)\n", brand.CLI, err)
		return 1
	}
	md, err := skill.ParseSkillMD(mdData)
	if err != nil {
		fmt.Fprintf(stderr, "%s skill import: %v\n", brand.CLI, err)
		return 1
	}

	// Collect every file except SKILL.md as a resource, preserving the relative
	// layout (reference/..., scripts/...). Enforce the same caps the bundle store
	// applies, but fail early here with a clear message.
	resources := map[string]any{}
	total := 0
	walkErr := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, rerr := filepath.Rel(dir, path)
		if rerr != nil {
			return rerr
		}
		if filepath.ToSlash(rel) == "SKILL.md" {
			return nil // the body, not a resource
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		if len(data) > skill.MaxBundleFile {
			return fmt.Errorf("resource %q is %d bytes (max %d)", rel, len(data), skill.MaxBundleFile)
		}
		total += len(data)
		if total > skill.MaxBundleTotal {
			return fmt.Errorf("bundle exceeds %d bytes total", skill.MaxBundleTotal)
		}
		resources[filepath.ToSlash(rel)] = string(data)
		return nil
	})
	if walkErr != nil {
		fmt.Fprintf(stderr, "%s skill import: %v\n", brand.CLI, walkErr)
		return 1
	}

	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	callArgs := map[string]any{
		"name":        md.Name,
		"description": md.Description,
		"body":        md.Body,
	}
	if len(md.Triggers) > 0 {
		callArgs["triggers"] = md.Triggers
	}
	if len(md.ToolsRequired) > 0 {
		callArgs["tools_required"] = md.ToolsRequired
	}
	if len(resources) > 0 {
		callArgs["resources"] = resources
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
	verb := "already present (refreshed)"
	if created {
		verb = "installed as a new " + status
	}
	fmt.Fprintf(stdout, "skill bundle %q %s\n", md.Name, verb)
	fmt.Fprintf(stdout, "  id: %s  status: %s  resources: %d\n", shortHash(gotID), status, len(resources))
	if created {
		fmt.Fprintf(stdout, "  inspect its files: %s skill files %s\n", brand.CLI, shortHash(gotID))
		fmt.Fprintf(stdout, "  promote it into the retrieval pool: %s skill promote %s\n", brand.CLI, gotID)
	}
	return 0
}

// importSkillMarkdownBytes ingests an agentskills.io/ClawHub SKILL.md (SPEC-13
// §1.2): parse the frontmatter + body, then install it via the same CmdSkillImport
// path the JSON-bundle importer uses — a fresh DRAFT, content-addressed by the
// daemon, journaled, never auto-active. Unlike a Agezt bundle there is no
// content address to verify offline (a SKILL.md is plain Markdown); the daemon
// derives the id from (name, body) on install.
func importSkillMarkdownBytes(data []byte, asJSON bool, stdout, stderr io.Writer) int {
	md, err := skill.ParseSkillMD(data)
	if err != nil {
		fmt.Fprintf(stderr, "%s skill import: %v\n", brand.CLI, err)
		return 1
	}

	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// Only send non-empty optional list args (a nil slice marshals to JSON null,
	// which the server's strict array decoder rejects).
	callArgs := map[string]any{
		"name":        md.Name,
		"description": md.Description,
		"body":        md.Body,
	}
	if len(md.Triggers) > 0 {
		callArgs["triggers"] = md.Triggers
	}
	if len(md.ToolsRequired) > 0 {
		callArgs["tools_required"] = md.ToolsRequired
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
	verb := "already present (refreshed)"
	if created {
		// Status is normally draft, but auto-shadow (SPEC-05 §5.2) may have staged
		// it on creation — report what actually landed.
		verb = "installed as a new " + status
	}
	fmt.Fprintf(stdout, "SKILL.md %q %s\n", md.Name, verb)
	fmt.Fprintf(stdout, "  id: %s  status: %s\n", shortHash(gotID), status)
	if created {
		fmt.Fprintf(stdout, "  promote it into the retrieval pool: %s skill promote %s\n", brand.CLI, gotID)
	}
	return 0
}

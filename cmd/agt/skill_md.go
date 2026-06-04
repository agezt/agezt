// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"io"
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

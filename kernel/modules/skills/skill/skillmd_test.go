// SPDX-License-Identifier: MIT

package skill

import (
	"reflect"
	"strings"
	"testing"
)

// TestParseSkillMD_InlineLists: scalars + inline `[a, b]` lists + a Markdown body.
func TestParseSkillMD_InlineLists(t *testing.T) {
	src := `---
name: diagnose-failing-ci
description: Diagnose and fix a red CI build
triggers: [ci, build, failure]
tools_required: [shell, http]
version: 1.2.0
---
# Diagnose CI

1. Pull the failing logs.
2. Identify the failing test.
`
	md, err := ParseSkillMD([]byte(src))
	if err != nil {
		t.Fatalf("ParseSkillMD: %v", err)
	}
	if md.Name != "diagnose-failing-ci" || md.Version != "1.2.0" {
		t.Errorf("name/version wrong: %+v", md)
	}
	if md.Description != "Diagnose and fix a red CI build" {
		t.Errorf("description = %q", md.Description)
	}
	if !reflect.DeepEqual(md.Triggers, []string{"ci", "build", "failure"}) {
		t.Errorf("triggers = %v", md.Triggers)
	}
	if !reflect.DeepEqual(md.ToolsRequired, []string{"shell", "http"}) {
		t.Errorf("tools_required = %v", md.ToolsRequired)
	}
	if !strings.HasPrefix(md.Body, "# Diagnose CI") || !strings.Contains(md.Body, "failing test") {
		t.Errorf("body wrong: %q", md.Body)
	}
}

// TestParseSkillMD_BlockListsAndQuotes: block (`- item`) lists, quoted scalars,
// the `tools` alias, and CRLF line endings.
func TestParseSkillMD_BlockListsAndQuotes(t *testing.T) {
	src := "---\r\n" +
		"name: \"summarize-thread\"\r\n" +
		"description: 'Summarize a long thread'\r\n" +
		"triggers:\r\n" +
		"  - summary\r\n" +
		"  - tldr\r\n" +
		"tools:\r\n" +
		"  - http\r\n" +
		"---\r\n" +
		"Body line one.\r\n"
	md, err := ParseSkillMD([]byte(src))
	if err != nil {
		t.Fatalf("ParseSkillMD: %v", err)
	}
	if md.Name != "summarize-thread" {
		t.Errorf("name = %q (quotes should be stripped)", md.Name)
	}
	if md.Description != "Summarize a long thread" {
		t.Errorf("description = %q (single quotes should be stripped)", md.Description)
	}
	if !reflect.DeepEqual(md.Triggers, []string{"summary", "tldr"}) {
		t.Errorf("block-list triggers = %v", md.Triggers)
	}
	if !reflect.DeepEqual(md.ToolsRequired, []string{"http"}) {
		t.Errorf("tools alias = %v", md.ToolsRequired)
	}
	if md.Body != "Body line one." {
		t.Errorf("body = %q", md.Body)
	}
}

// TestParseSkillMD_Errors: missing frontmatter, missing name, empty body,
// unterminated frontmatter all fail closed.
func TestParseSkillMD_Errors(t *testing.T) {
	cases := map[string]string{
		"no frontmatter": "# Just markdown, no frontmatter\nbody",
		"missing name":   "---\ndescription: x\n---\nbody",
		"empty body":     "---\nname: x\n---\n   \n",
		"unterminated":   "---\nname: x\ndescription: y\nbody with no closing fence",
	}
	for label, src := range cases {
		if _, err := ParseSkillMD([]byte(src)); err == nil {
			t.Errorf("%s: expected an error, got nil", label)
		}
	}
}

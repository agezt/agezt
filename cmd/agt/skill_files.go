// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)

// cmdSkillFiles implements `agt skill files <id>` (M847): list the on-disk
// bundle resources (reference files + scripts) that travel with a skill, plus
// the directory they live in — the working directory a bundled script runs from.
func cmdSkillFiles(args []string, stdout, stderr io.Writer) int {
	id, asJSON := "", false
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s skill files <id> [--json]\n", brand.CLI)
			return 0
		default:
			if id == "" {
				id = a
			}
		}
	}
	if id == "" {
		fmt.Fprintf(stderr, "usage: %s skill files <id> [--json]\n", brand.CLI)
		return 2
	}
	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdSkillFiles, map[string]any{"id": id})
	if err != nil {
		fmt.Fprintf(stderr, "%s skill files: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return encodeJSON(stdout, res)
	}
	name, _ := res["name"].(string)
	dir, _ := res["dir"].(string)
	files, _ := res["files"].([]any)
	if len(files) == 0 {
		fmt.Fprintf(stdout, "skill %q has no bundle resources\n", name)
		return 0
	}
	fmt.Fprintf(stdout, "skill %q bundle (%d file(s)):\n", name, len(files))
	for _, f := range files {
		fmt.Fprintf(stdout, "  %s\n", str(f))
	}
	if dir != "" {
		fmt.Fprintf(stdout, "dir: %s\n", dir)
		fmt.Fprintf(stdout, "read one: %s skill cat %s <path>\n", brand.CLI, shortHash(str(res["id"])))
	}
	return 0
}

// cmdSkillCat implements `agt skill cat <id> <path>` (M847): print one bundle
// resource — a reference file or a script. The path is one of those listed by
// `agt skill files`; the daemon rejects any path that escapes the bundle.
func cmdSkillCat(args []string, stdout, stderr io.Writer) int {
	var id, path string
	for _, a := range args {
		switch {
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s skill cat <id> <path>\n", brand.CLI)
			return 0
		case id == "":
			id = a
		case path == "":
			path = a
		}
	}
	if id == "" || path == "" {
		fmt.Fprintf(stderr, "usage: %s skill cat <id> <path>\n", brand.CLI)
		return 2
	}
	c := dial(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdSkillReadFile, map[string]any{"id": id, "path": path})
	if err != nil {
		fmt.Fprintf(stderr, "%s skill cat: %v\n", brand.CLI, err)
		return 1
	}
	content, _ := res["content"].(string)
	fmt.Fprint(stdout, content)
	if content != "" && content[len(content)-1] != '\n' {
		fmt.Fprintln(stdout)
	}
	return 0
}

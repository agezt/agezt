// SPDX-License-Identifier: MIT
//
// cmd/agt `plugin new` top-level dispatch (cmdPluginNew).
// Extracted from plugin_new.go during Day 211 god-file refactor (#101).
// Public API unchanged.
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/agezt/agezt/internal/brand"
)

func cmdPluginNew(args []string, stdout, stderr io.Writer) int {
	var name, dir, module string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-h" || a == "--help":
			pluginNewUsage(stdout)
			return 0
		case a == "--dir":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s plugin new: --dir requires a value\n", brand.CLI)
				return 2
			}
			i++
			dir = args[i]
		case a == "--module":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s plugin new: --module requires a value\n", brand.CLI)
				return 2
			}
			i++
			module = args[i]
		case strings.HasPrefix(a, "-"):
			fmt.Fprintf(stderr, "%s plugin new: unknown flag %q\n", brand.CLI, a)
			return 2
		default:
			if name != "" {
				fmt.Fprintf(stderr, "%s plugin new: unexpected extra argument %q\n", brand.CLI, a)
				return 2
			}
			name = a
		}
	}

	if name == "" {
		fmt.Fprintf(stderr, "%s plugin new: a plugin name is required\n", brand.CLI)
		pluginNewUsage(stderr)
		return 2
	}

	// The tool identifier the plugin advertises is derived from the
	// name, sanitised to the conservative set agezt tool names use.
	tool := sanitizeToolName(name)
	if tool == "" {
		fmt.Fprintf(stderr, "%s plugin new: name %q has no usable letters/digits for a tool name\n", brand.CLI, name)
		return 2
	}
	if dir == "" {
		dir = name
	}
	if module == "" {
		module = "agezt-plugin-" + tool
	}

	// Refuse to write into a non-empty directory.
	if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
		fmt.Fprintf(stderr, "%s plugin new: directory %q is not empty — refusing to overwrite\n", brand.CLI, dir)
		return 1
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fmt.Fprintf(stderr, "%s plugin new: create %q: %v\n", brand.CLI, dir, err)
		return 1
	}

	mainSrc, err := renderPluginMain(name, tool)
	if err != nil {
		// format.Source failing means the template is broken — a
		// programming error, not user input. Surface it loudly.
		fmt.Fprintf(stderr, "%s plugin new: internal: render main.go: %v\n", brand.CLI, err)
		return 1
	}

	files := map[string]string{
		"main.go":    mainSrc,
		"go.mod":     renderPluginGoMod(module),
		"README.md":  renderPluginReadme(name, tool, module),
		".gitignore": "/" + tool + "\n/" + tool + ".exe\n",
	}
	for fname, content := range files {
		if err := os.WriteFile(filepath.Join(dir, fname), []byte(content), 0o644); err != nil {
			fmt.Fprintf(stderr, "%s plugin new: write %s: %v\n", brand.CLI, fname, err)
			return 1
		}
	}

	fmt.Fprintf(stdout, "Scaffolded plugin %q in %s\n\n", name, dir)
	fmt.Fprintf(stdout, "Next steps:\n")
	fmt.Fprintf(stdout, "  cd %s\n", dir)
	fmt.Fprintf(stdout, "  go mod tidy            # resolve the agezt SDK dependency\n")
	fmt.Fprintf(stdout, "  go build -o %s .\n", tool)
	fmt.Fprintf(stdout, "  %s=%q %s\n", "AGEZT_PLUGINS", tool+"=./"+tool, brand.Binary)
	fmt.Fprintf(stdout, "\nThe %q tool is then available to the agent. Edit main.go to add your own.\n", tool)
	return 0
}

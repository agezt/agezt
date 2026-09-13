// SPDX-License-Identifier: MIT
//
// cmd/agt plugin-registry surface: the const block + the pluginIndex +
// indexPlugin + indexBinary types + cmdPluginRegistry (the entry dispatcher).
// The index read/install path lives in plugin_registry_index.go; the
// helpers live in plugin_registry_helpers.go.
// Extracted from plugin_registry.go during the Day-210 god-file split.
// Public API unchanged.
package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/agezt/agezt/internal/brand"
)

// pluginRegistryIndexName is the manifest a plugin registry serves, mirroring the
// skill registry's index.json: a static host offers no directory listing, so the
// index enumerates what's available.
const pluginRegistryIndexName = "index.json"

// maxPluginIndexFetch bounds the index.json body. Plugin metadata is small text.
const maxPluginIndexFetch = 8 << 20 // 8 MiB

// maxPluginBinaryFetch bounds a downloaded plugin binary. Generous (plugins are
// native executables) while still refusing a runaway body.
const maxPluginBinaryFetch = 256 << 20 // 256 MiB

// pluginIndex is the manifest of a plugin registry.
type pluginIndex struct {
	Tool            string        `json:"tool"`
	FormatVersion   int           `json:"format_version"`
	GeneratedUnixMS int64         `json:"generated_unix_ms,omitempty"`
	Plugins         []indexPlugin `json:"plugins"`
}

// indexPlugin is one entry: shareable metadata plus the per-platform binaries it
// ships, each pinned by its BLAKE3-256 digest (the same pin the daemon enforces
// via AGEZT_PLUGIN_PINS).
type indexPlugin struct {
	Name        string        `json:"name"`
	Version     string        `json:"version"`
	Description string        `json:"description,omitempty"`
	Prefix      string        `json:"prefix,omitempty"` // suggested AGEZT_PLUGINS prefix (defaults to Name)
	Args        string        `json:"args,omitempty"`   // optional extra args after the path
	Binaries    []indexBinary `json:"binaries"`
}

// indexBinary is one platform build of a plugin.
type indexBinary struct {
	OS     string `json:"os"`
	Arch   string `json:"arch"`
	File   string `json:"file"`
	BLAKE3 string `json:"blake3"`
}

// cmdPluginRegistry implements `agt plugin registry <dir|url> [--install <name>]`.
// It lists the plugins a registry offers, or downloads one, verifies its BLAKE3
// pin, and writes it locally — then prints the exact AGEZT_PLUGINS / _PINS lines
// to enable it. It NEVER edits the daemon's environment or loads anything: the
// daemon runs a plugin only when the operator wires it in, so "install" stays
// "fetch + verify + stage", under the operator's authority.
func cmdPluginRegistry(args []string, stdout, stderr io.Writer) int {
	source := ""
	install := ""
	installDir := ""
	asJSON := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json":
			asJSON = true
		case a == "--install":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s plugin registry: --install needs a plugin name\n", brand.CLI)
				return 2
			}
			i++
			install = args[i]
		case strings.HasPrefix(a, "--install="):
			install = strings.TrimPrefix(a, "--install=")
		case a == "--dir":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s plugin registry: --dir needs a directory\n", brand.CLI)
				return 2
			}
			i++
			installDir = args[i]
		case strings.HasPrefix(a, "--dir="):
			installDir = strings.TrimPrefix(a, "--dir=")
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s plugin registry <dir|url> [--json] [--install <name>] [--dir <installdir>]\n", brand.CLI)
			fmt.Fprintf(stdout, "list the plugins a registry offers, or install one (download + BLAKE3-verify)\n")
			fmt.Fprintf(stdout, "  --install <name>  download the named plugin's binary for this OS/arch,\n")
			fmt.Fprintf(stdout, "                    verify its pin, and stage it (does NOT load it — prints\n")
			fmt.Fprintf(stdout, "                    the AGEZT_PLUGINS/_PINS lines to enable it yourself)\n")
			fmt.Fprintf(stdout, "  --dir <installdir>  where to write the binary (default <base>/plugins)\n")
			fmt.Fprintf(stdout, "a remote registry is an http(s) URL serving index.json + binary files\n")
			return 0
		case strings.HasPrefix(a, "-"):
			fmt.Fprintf(stderr, "%s plugin registry: unknown flag %q\n", brand.CLI, a)
			return 2
		default:
			if source != "" {
				fmt.Fprintf(stderr, "%s plugin registry: unexpected extra argument %q\n", brand.CLI, a)
				return 2
			}
			source = a
		}
	}
	if source == "" {
		fmt.Fprintf(stderr, "%s plugin registry: a directory or http(s) URL is required\n", brand.CLI)
		return 2
	}

	idx, ok := loadPluginIndex(source, stderr)
	if !ok {
		return 1
	}
	if install != "" {
		return installPluginFromRegistry(source, idx, install, installDir, asJSON, stdout, stderr)
	}
	return listPluginRegistry(source, idx, asJSON, stdout, stderr)
}

// loadPluginIndex reads index.json from a directory or an http(s) URL.

// SPDX-License-Identifier: MIT
//
// cmd/agt plugin-registry index path: loadPluginIndex (the index fetcher) +
// listPluginRegistry (the list sub-command) + installPluginFromRegistry
// (the install sub-command).
// Extracted from plugin_registry.go during the Day-210 god-file split.
// Public API unchanged.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/internal/paths"
	"github.com/agezt/agezt/kernel/plugin"
)

func loadPluginIndex(source string, stderr io.Writer) (pluginIndex, bool) {
	var raw []byte
	var ok bool
	if isHTTPURL(source) {
		raw, ok = httpGetBounded(strings.TrimRight(source, "/")+"/"+pluginRegistryIndexName, maxPluginIndexFetch, "index", stderr)
	} else {
		raw, ok = readRegistryFile(source, pluginRegistryIndexName, maxPluginIndexFetch, stderr)
	}
	if !ok {
		return pluginIndex{}, false
	}
	var idx pluginIndex
	if err := json.Unmarshal(raw, &idx); err != nil {
		fmt.Fprintf(stderr, "%s plugin registry: parse %s: %v\n", brand.CLI, pluginRegistryIndexName, err)
		return pluginIndex{}, false
	}
	return idx, true
}

// listPluginRegistry prints the registry's plugins (and whether a build exists
// for the running OS/arch).
func listPluginRegistry(source string, idx pluginIndex, asJSON bool, stdout, stderr io.Writer) int {
	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(map[string]any{"source": source, "plugins": idx.Plugins, "count": len(idx.Plugins)})
		return 0
	}
	if len(idx.Plugins) == 0 {
		fmt.Fprintf(stdout, "registry at %s lists no plugins\n", source)
		return 0
	}
	fmt.Fprintf(stdout, "%d plugin(s) at %s:\n", len(idx.Plugins), source)
	for _, p := range idx.Plugins {
		fmt.Fprintf(stdout, "\n  %-20s v%s\n", p.Name, p.Version)
		if p.Description != "" {
			fmt.Fprintf(stdout, "      %s\n", p.Description)
		}
		fmt.Fprintf(stdout, "      platforms: %s\n", strings.Join(platformList(p), ", "))
		if _, ok := selectBinary(p); ok {
			fmt.Fprintf(stdout, "      install: %s plugin registry %s --install %s\n", brand.CLI, source, p.Name)
		} else {
			fmt.Fprintf(stdout, "      (no build for this host, %s/%s)\n", runtime.GOOS, runtime.GOARCH)
		}
	}
	return 0
}

// installPluginFromRegistry resolves a name to one plugin, downloads the binary
// for this host, verifies its pin, stages it, and prints the enabling env lines.
func installPluginFromRegistry(source string, idx pluginIndex, name, installDir string, asJSON bool, stdout, stderr io.Writer) int {
	var match *indexPlugin
	count := 0
	for i := range idx.Plugins {
		if idx.Plugins[i].Name == name {
			match = &idx.Plugins[i]
			count++
		}
	}
	if count == 0 {
		fmt.Fprintf(stderr, "%s plugin registry: no plugin named %q at %s\n", brand.CLI, name, source)
		return 1
	}
	if count > 1 {
		fmt.Fprintf(stderr, "%s plugin registry: %q is ambiguous (%d entries) at %s\n", brand.CLI, name, count, source)
		return 1
	}

	bin, ok := selectBinary(*match)
	if !ok {
		fmt.Fprintf(stderr, "%s plugin registry: %q has no build for this host (%s/%s)\n", brand.CLI, name, runtime.GOOS, runtime.GOARCH)
		return 1
	}
	if !safeRegistryFilename(bin.File) {
		fmt.Fprintf(stderr, "%s plugin registry: refusing unsafe binary filename %q\n", brand.CLI, bin.File)
		return 1
	}
	if !plugin.LooksLikePin(bin.BLAKE3) {
		fmt.Fprintf(stderr, "%s plugin registry: %q has no valid BLAKE3 pin in the index — refusing\n", brand.CLI, name)
		return 1
	}

	// Resolve the install directory (default <base>/plugins) and ensure it exists.
	dir := installDir
	if dir == "" {
		base, err := paths.BaseDir()
		if err != nil {
			fmt.Fprintf(stderr, "%s plugin registry: resolve install dir: %v\n", brand.CLI, err)
			return 1
		}
		dir = filepath.Join(base, "plugins")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fmt.Fprintf(stderr, "%s plugin registry: create %s: %v\n", brand.CLI, dir, err)
		return 1
	}
	dest := filepath.Join(dir, bin.File)

	// Fetch the binary into memory (bounded), verify the pin BEFORE writing it to
	// the final path, then write + chmod. Verifying first means a tampered binary
	// never lands on disk under the operator's plugin dir.
	var data []byte
	if isHTTPURL(source) {
		data, ok = httpGetBounded(strings.TrimRight(source, "/")+"/"+bin.File, maxPluginBinaryFetch, "binary", stderr)
	} else {
		data, ok = readRegistryFile(source, bin.File, maxPluginBinaryFetch, stderr)
	}
	if !ok {
		return 1
	}
	got := plugin.HashBytes(data)
	if got != strings.ToLower(strings.TrimSpace(bin.BLAKE3)) {
		fmt.Fprintf(stderr, "%s plugin registry: BLAKE3 mismatch for %q — refusing to install\n", brand.CLI, name)
		fmt.Fprintf(stderr, "  expected: %s\n  got:      %s\n", strings.ToLower(bin.BLAKE3), got)
		return 1
	}
	if err := os.WriteFile(dest, data, 0o755); err != nil {
		fmt.Fprintf(stderr, "%s plugin registry: write %s: %v\n", brand.CLI, dest, err)
		return 1
	}

	prefix := match.Prefix
	if prefix == "" {
		prefix = match.Name
	}
	pluginsVal := prefix + "=" + dest
	if strings.TrimSpace(match.Args) != "" {
		pluginsVal += " " + strings.TrimSpace(match.Args)
	}

	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(map[string]any{
			"installed": name,
			"version":   match.Version,
			"path":      dest,
			"blake3":    got,
			"env": map[string]string{
				"AGEZT_PLUGINS":     pluginsVal,
				"AGEZT_PLUGIN_PINS": prefix + "=" + got,
			},
		})
		return 0
	}

	fmt.Fprintf(stdout, "installed %s v%s → %s\n", name, match.Version, dest)
	fmt.Fprintf(stdout, "  verified blake3:%s\n", got)
	fmt.Fprintf(stdout, "\nTo enable it, add to your daemon environment (it does not load until you do):\n")
	// Plain shell double-quotes around the value (not %q Go-quoting, which would
	// double-escape backslashes in a Windows path).
	fmt.Fprintf(stdout, "  AGEZT_PLUGINS=\"%s\"\n", pluginsVal)
	fmt.Fprintf(stdout, "  AGEZT_PLUGIN_PINS=\"%s\"\n", prefix+"="+got)
	return 0
}

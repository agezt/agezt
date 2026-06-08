// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/internal/paths"
	"github.com/agezt/agezt/kernel/plugin"
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

// selectBinary picks the registry binary matching the running OS/arch.
func selectBinary(p indexPlugin) (indexBinary, bool) {
	for _, b := range p.Binaries {
		if b.OS == runtime.GOOS && b.Arch == runtime.GOARCH {
			return b, true
		}
	}
	return indexBinary{}, false
}

// platformList returns the sorted "os/arch" labels a plugin ships, for listing.
func platformList(p indexPlugin) []string {
	out := make([]string, 0, len(p.Binaries))
	for _, b := range p.Binaries {
		out = append(out, b.OS+"/"+b.Arch)
	}
	sort.Strings(out)
	return out
}

// safeRegistryFilename rejects anything but a plain filename — no path separators,
// no traversal — since the name comes from an untrusted index.
func safeRegistryFilename(name string) bool {
	if name == "" {
		return false
	}
	if strings.ContainsAny(name, "/\\") || strings.Contains(name, "..") {
		return false
	}
	return true
}

// readRegistryFile reads relPath from a local directory registry, bounded.
func readRegistryFile(dir, relPath string, max int64, stderr io.Writer) ([]byte, bool) {
	if !safeRegistryFilename(relPath) {
		fmt.Fprintf(stderr, "%s plugin registry: refusing unsafe index file %q\n", brand.CLI, relPath)
		return nil, false
	}
	f, err := os.Open(filepath.Join(dir, relPath))
	if err != nil {
		fmt.Fprintf(stderr, "%s plugin registry: open %s: %v\n", brand.CLI, relPath, err)
		return nil, false
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, max+1))
	if err != nil {
		fmt.Fprintf(stderr, "%s plugin registry: read %s: %v\n", brand.CLI, relPath, err)
		return nil, false
	}
	if int64(len(data)) > max {
		fmt.Fprintf(stderr, "%s plugin registry: %s exceeds the %d-byte cap\n", brand.CLI, relPath, max)
		return nil, false
	}
	return data, true
}

// httpGetBounded GETs a URL with a bounded body and timeout.
func httpGetBounded(url string, max int64, what string, stderr io.Writer) ([]byte, bool) {
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		fmt.Fprintf(stderr, "%s plugin registry: fetch %s: %v\n", brand.CLI, url, err)
		return nil, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(stderr, "%s plugin registry: fetch %s: HTTP %d\n", brand.CLI, url, resp.StatusCode)
		return nil, false
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, max+1))
	if err != nil {
		fmt.Fprintf(stderr, "%s plugin registry: read %s %s: %v\n", brand.CLI, what, url, err)
		return nil, false
	}
	if int64(len(data)) > max {
		fmt.Fprintf(stderr, "%s plugin registry: %s %s exceeds the %d-byte cap\n", brand.CLI, what, url, max)
		return nil, false
	}
	return data, true
}

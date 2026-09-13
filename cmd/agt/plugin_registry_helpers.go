// SPDX-License-Identifier: MIT
//
// cmd/agt plugin-registry helpers: selectBinary + platformList +
// safeRegistryFilename + readRegistryFile + httpGetBounded.
// Extracted from plugin_registry.go during the Day-210 god-file split.
// Public API unchanged.
package main

import (
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
)

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

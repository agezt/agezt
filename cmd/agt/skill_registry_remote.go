// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
)

// maxRegistryFetch bounds a single registry HTTP response (index or bundle).
// Skills are small text; this is generous while still refusing a runaway body.
const maxRegistryFetch = 8 << 20 // 8 MiB

// isHTTPURL reports whether s addresses a remote registry over HTTP(S).
func isHTTPURL(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

// fetchRegistryFile GETs one file from a base registry URL with a bounded body
// and timeout. relPath must be a plain filename (no traversal, no scheme) — it
// comes from an untrusted index, so it is validated before being joined.
func fetchRegistryFile(baseURL, relPath string, stderr io.Writer) ([]byte, bool) {
	if relPath != "" {
		if strings.ContainsAny(relPath, "/\\") || strings.Contains(relPath, "..") {
			fmt.Fprintf(stderr, "%s skill registry: refusing unsafe index file %q\n", brand.CLI, relPath)
			return nil, false
		}
	}
	url := strings.TrimRight(baseURL, "/")
	if relPath != "" {
		url += "/" + relPath
	}
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		fmt.Fprintf(stderr, "%s skill registry: fetch %s: %v\n", brand.CLI, url, err)
		return nil, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(stderr, "%s skill registry: fetch %s: HTTP %d\n", brand.CLI, url, resp.StatusCode)
		return nil, false
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxRegistryFetch+1))
	if err != nil {
		fmt.Fprintf(stderr, "%s skill registry: read %s: %v\n", brand.CLI, url, err)
		return nil, false
	}
	if len(data) > maxRegistryFetch {
		fmt.Fprintf(stderr, "%s skill registry: %s exceeds the %d-byte cap\n", brand.CLI, url, maxRegistryFetch)
		return nil, false
	}
	return data, true
}

// remoteRegistry fetches and lists (or installs from) a registry served over
// HTTP — the consumer side of the index.json a publisher writes with
// `agt skill export --all` (M274). Discovery uses the index manifest since a
// static host offers no directory listing; install fetches the named bundle and
// runs it through the same content-address verification as a local import.
func remoteRegistry(baseURL, install string, asJSON bool, stdout, stderr io.Writer) int {
	raw, ok := fetchRegistryFile(baseURL, registryIndexName, stderr)
	if !ok {
		return 1
	}
	var idx registryIndex
	if err := json.Unmarshal(raw, &idx); err != nil {
		fmt.Fprintf(stderr, "%s skill registry: parse %s: %v\n", brand.CLI, registryIndexName, err)
		return 1
	}

	if install != "" {
		return remoteInstall(baseURL, idx, install, asJSON, stdout, stderr)
	}

	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(map[string]any{"url": baseURL, "bundles": idx.Skills, "count": len(idx.Skills)})
		return 0
	}
	if len(idx.Skills) == 0 {
		fmt.Fprintf(stdout, "registry at %s lists no skills\n", baseURL)
		return 0
	}
	fmt.Fprintf(stdout, "%d skill(s) at %s:\n", len(idx.Skills), baseURL)
	for _, s := range idx.Skills {
		fmt.Fprintf(stdout, "  %-24s v%-8s %s\n", s.Name, s.Version, shortHash(s.ID))
		if s.Description != "" {
			fmt.Fprintf(stdout, "      %s\n", s.Description)
		}
		fmt.Fprintf(stdout, "      install: %s skill registry %s --install %s\n", brand.CLI, baseURL, s.Name)
	}
	return 0
}

// remoteInstall resolves a name in the index to one entry, fetches its bundle,
// and installs it through the shared verify-then-import path.
func remoteInstall(baseURL string, idx registryIndex, name string, asJSON bool, stdout, stderr io.Writer) int {
	var matches []indexSkill
	for _, s := range idx.Skills {
		if s.Name == name {
			matches = append(matches, s)
		}
	}
	switch len(matches) {
	case 0:
		fmt.Fprintf(stderr, "%s skill registry: no skill named %q at %s\n", brand.CLI, name, baseURL)
		return 1
	case 1:
		// proceed
	default:
		fmt.Fprintf(stderr, "%s skill registry: %q is ambiguous (%d entries) at %s\n", brand.CLI, name, len(matches), baseURL)
		return 1
	}
	data, ok := fetchRegistryFile(baseURL, matches[0].File, stderr)
	if !ok {
		return 1
	}
	return importSkillBundleBytes(data, asJSON, stdout, stderr)
}

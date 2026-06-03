// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/agezt/agezt/internal/brand"
)

// registryIndexName is the manifest file `agt skill export --all` writes into a
// registry directory and a remote consumer fetches to discover it.
const registryIndexName = "index.json"

// registryIndex is the manifest of a skill registry — what bundles it holds,
// each pointing at its file. It lets a consumer browse a registry served over
// plain static HTTP, where no directory listing is available.
type registryIndex struct {
	Tool            string       `json:"tool"`
	FormatVersion   int          `json:"format_version"`
	GeneratedUnixMS int64        `json:"generated_unix_ms"`
	Skills          []indexSkill `json:"skills"`
}

// indexSkill is one entry in a registryIndex: the shareable metadata plus the
// bundle's filename within the registry.
type indexSkill struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	ID          string `json:"id"`
	Description string `json:"description,omitempty"`
	File        string `json:"file"`
}

// registryEntry is one discovered skill bundle in a directory "registry" — the
// discovery layer over the portable bundle format (M270). Verified is false when
// the bundle's content address does not match its (name, body), so a tampered or
// hand-edited bundle is visible at a glance; Err carries a parse failure.
type registryEntry struct {
	Path        string   `json:"path"`
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	ID          string   `json:"id"`
	Description string   `json:"description"`
	Triggers    []string `json:"triggers,omitempty"`
	Verified    bool     `json:"verified"`
	Err         string   `json:"error,omitempty"`
}

// scanSkillRegistry reads every *.skill.json file in dir as a skill bundle and
// returns one entry per file, sorted by name then path. A file that does not
// parse as a bundle (or has no skill name) is reported with Err set rather than
// dropped — the operator should see a malformed file, not have it vanish. The
// scan itself only fails if the directory cannot be listed.
func scanSkillRegistry(dir string) ([]registryEntry, error) {
	matches, err := filepath.Glob(filepath.Join(dir, "*.skill.json"))
	if err != nil {
		return nil, err
	}
	entries := make([]registryEntry, 0, len(matches))
	for _, path := range matches {
		entry := registryEntry{Path: path}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			entry.Err = rerr.Error()
			entries = append(entries, entry)
			continue
		}
		var b skillBundle
		if uerr := json.Unmarshal(data, &b); uerr != nil {
			entry.Err = "not a skill bundle: " + uerr.Error()
			entries = append(entries, entry)
			continue
		}
		if strings.TrimSpace(b.Skill.Name) == "" {
			entry.Err = "not a skill bundle: no skill name"
			entries = append(entries, entry)
			continue
		}
		entry.Name = b.Skill.Name
		entry.Version = b.Skill.Version
		entry.ID = b.Skill.ID
		entry.Description = b.Skill.Description
		entry.Triggers = b.Skill.Triggers
		entry.Verified = verifySkillBundle(b) == nil
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Name != entries[j].Name {
			return entries[i].Name < entries[j].Name
		}
		return entries[i].Path < entries[j].Path
	})
	return entries, nil
}

// installFromRegistry resolves a skill name within a scanned registry to exactly
// one verified bundle and installs it by delegating to cmdSkillImport (which
// re-verifies and dials the daemon). It refuses an ambiguous name (several
// bundles share it — e.g. different versions) so the operator imports the one
// they mean by path, and ignores unverified/malformed candidates (M271).
func installFromRegistry(entries []registryEntry, name string, stdout, stderr io.Writer) int {
	var matches []registryEntry
	tampered := 0
	for _, e := range entries {
		if e.Name != name {
			continue
		}
		if e.Err != "" || !e.Verified {
			tampered++
			continue
		}
		matches = append(matches, e)
	}
	switch len(matches) {
	case 0:
		if tampered > 0 {
			fmt.Fprintf(stderr, "%s skill registry: %q has only malformed/tampered bundle(s) — refusing to install\n", brand.CLI, name)
		} else {
			fmt.Fprintf(stderr, "%s skill registry: no verified bundle named %q\n", brand.CLI, name)
		}
		return 1
	case 1:
		return cmdSkillImport([]string{matches[0].Path}, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "%s skill registry: %q is ambiguous (%d bundles) — import the one you want by path:\n", brand.CLI, name, len(matches))
		for _, e := range matches {
			fmt.Fprintf(stderr, "  %s skill import %s   (v%s, %s)\n", brand.CLI, e.Path, e.Version, shortHash(e.ID))
		}
		return 1
	}
}

// cmdSkillRegistry implements `agt skill registry <dir> [--json]` (M270) — the
// discovery layer of the skill marketplace: list the verifiable bundles in a
// directory so an operator can see what is available before importing one. Pure
// offline file read; no daemon needed.
func cmdSkillRegistry(args []string, stdout, stderr io.Writer) int {
	dir := ""
	asJSON := false
	install := ""
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json":
			asJSON = true
		case a == "--install":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s skill registry: --install needs a skill name\n", brand.CLI)
				return 2
			}
			i++
			install = args[i]
		case strings.HasPrefix(a, "--install="):
			install = strings.TrimPrefix(a, "--install=")
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s skill registry <dir|url> [--json] [--install <name>]\n", brand.CLI)
			fmt.Fprintf(stdout, "list the verifiable skill bundles in a directory or a remote registry URL\n")
			fmt.Fprintf(stdout, "  --install <name>  install the named bundle from the registry as a draft\n")
			fmt.Fprintf(stdout, "a remote registry is an http(s) URL serving index.json + bundle files\n")
			fmt.Fprintf(stdout, "import one by path with: %s skill import <path>\n", brand.CLI)
			return 0
		case strings.HasPrefix(a, "-"):
			fmt.Fprintf(stderr, "%s skill registry: unexpected flag %q\n", brand.CLI, a)
			return 2
		default:
			if dir != "" {
				fmt.Fprintf(stderr, "%s skill registry: unexpected arg %q (one directory)\n", brand.CLI, a)
				return 2
			}
			dir = a
		}
	}
	if dir == "" {
		fmt.Fprintf(stderr, "%s skill registry: a directory or URL is required\n", brand.CLI)
		return 2
	}

	// A remote registry (http/https URL) is discovered via its index.json
	// manifest, since a static host offers no directory listing.
	if isHTTPURL(dir) {
		return remoteRegistry(dir, install, asJSON, stdout, stderr)
	}

	entries, err := scanSkillRegistry(dir)
	if err != nil {
		fmt.Fprintf(stderr, "%s skill registry: %v\n", brand.CLI, err)
		return 1
	}

	if install != "" {
		return installFromRegistry(entries, install, stdout, stderr)
	}

	if asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(map[string]any{"dir": dir, "bundles": entries, "count": len(entries)})
		return 0
	}

	if len(entries) == 0 {
		fmt.Fprintf(stdout, "no skill bundles (*.skill.json) in %s\n", dir)
		return 0
	}

	bad := 0
	fmt.Fprintf(stdout, "%d bundle(s) in %s:\n", len(entries), dir)
	for _, e := range entries {
		if e.Err != "" {
			bad++
			fmt.Fprintf(stdout, "  (!) %s — %s\n", filepath.Base(e.Path), e.Err)
			continue
		}
		mark := "ok"
		if !e.Verified {
			mark = "TAMPERED"
			bad++
		}
		fmt.Fprintf(stdout, "  %-24s v%-8s %s [%s]\n", e.Name, e.Version, shortHash(e.ID), mark)
		if e.Description != "" {
			fmt.Fprintf(stdout, "      %s\n", e.Description)
		}
		fmt.Fprintf(stdout, "      import: %s skill import %s\n", brand.CLI, e.Path)
	}
	if bad > 0 {
		fmt.Fprintf(stderr, "%s skill registry: %d bundle(s) malformed or tampered — `%s skill import` will reject them\n",
			brand.CLI, bad, brand.CLI)
		return 1
	}
	return 0
}

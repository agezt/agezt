// SPDX-License-Identifier: MIT

package main

// CLI wrappers for the `agt peers artifacts` and `agt peers
// artifact-get` subcommands. Each parses argv, calls the matching
// fetch primitive in peers_fetch.go, and renders the result as JSON
// or human-readable text. Carved out of peers.go during the Day 167
// god-file split. Public API unchanged.

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/plugins/tools/peer"
)

func peersArtifacts(peers map[string]peer.Peer, name, corr string, asJSON bool, stdout, stderr io.Writer) int {
	p, ok := peers[name]
	if !ok {
		fmt.Fprintf(stderr, "%s peers artifacts: unknown peer %q\n", brand.CLI, name)
		return 1
	}
	corr = strings.TrimSpace(corr)
	if corr == "" {
		fmt.Fprintf(stderr, "%s peers artifacts: correlation id is required\n", brand.CLI)
		return 2
	}
	list := fetchPeerArtifacts(p, corr)
	if asJSON {
		b, _ := json.MarshalIndent(list, "", "  ")
		fmt.Fprintln(stdout, string(b))
		if list.Error != "" {
			return 1
		}
		return 0
	}
	if list.Error != "" {
		fmt.Fprintf(stdout, "  %-14s %s  UNREACHABLE: %s\n", list.Peer, list.URL, list.Error)
		return 1
	}
	fmt.Fprintf(stdout, "peer %s artifacts for %s: %d artifact(s)", list.Peer, list.CorrelationID, list.Count)
	if list.Truncated {
		fmt.Fprintf(stdout, " (truncated from %d)", list.TotalCount)
	}
	fmt.Fprintln(stdout)
	for _, a := range list.Entries {
		ref := a.Ref
		if len(ref) > 12 {
			ref = ref[:12]
		}
		name := a.Name
		if name == "" {
			name = "(unnamed)"
		}
		kind := a.Kind
		if kind == "" {
			kind = "-"
		}
		fmt.Fprintf(stdout, "  %-22s %-12s %8d  %-24s %s\n", a.ID, kind, a.Size, name, ref)
	}
	return 0
}

func peersArtifactGet(peers map[string]peer.Peer, name, id, outPath string, asJSON bool, stdout, stderr io.Writer) int {
	p, ok := peers[name]
	if !ok {
		fmt.Fprintf(stderr, "%s peers artifact-get: unknown peer %q\n", brand.CLI, name)
		return 1
	}
	id = strings.TrimSpace(id)
	outPath = strings.TrimSpace(outPath)
	if id == "" || outPath == "" {
		fmt.Fprintf(stderr, "%s peers artifact-get: artifact id and out_file are required\n", brand.CLI)
		return 2
	}
	meta, data := fetchPeerArtifactBytes(p, id)
	if meta.Error == "" {
		if err := os.WriteFile(outPath, data, 0o600); err != nil {
			meta.Error = "write " + outPath + ": " + err.Error()
		}
	}
	if asJSON {
		b, _ := json.MarshalIndent(meta, "", "  ")
		fmt.Fprintln(stdout, string(b))
		if meta.Error != "" {
			return 1
		}
		return 0
	}
	if meta.Error != "" {
		fmt.Fprintf(stdout, "  %-14s %s  UNREACHABLE: %s\n", meta.Peer, meta.URL, meta.Error)
		return 1
	}
	fmt.Fprintf(stdout, "wrote %d byte(s) from peer %s artifact %s to %s\n", meta.Size, meta.Peer, meta.ID, outPath)
	return 0
}

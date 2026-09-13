// SPDX-License-Identifier: MIT

package main

// agt peers remote-exec helpers: peersRoute + peersRun.
// Carved out of peers.go during the Day 188 god-file split so the
// main file can stay focused on cmdPeers dispatch + the inspect file
// can stay focused on health + models.
// Public API unchanged.

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/plugins/tools/peer"
)

func peersRoute(peers map[string]peer.Peer, model string, asJSON bool, stdout, stderr io.Writer) int {
	names := make([]string, 0, len(peers))
	for n := range peers {
		names = append(names, n)
	}
	sort.Strings(names)

	results := make([]peerRoute, 0, len(names))
	chosen := ""
	for _, n := range names {
		pm := fetchPeerModels(peers[n])
		e := peerRoute{Name: n, URL: peers[n].URL, Reachable: pm.Reachable, Error: pm.Error}
		if pm.Reachable {
			for _, m := range pm.Models {
				if m == model {
					e.Serves = true
					break
				}
			}
			if e.Serves && chosen == "" {
				e.Chosen = true
				chosen = n
			}
		}
		results = append(results, e)
	}

	if asJSON {
		b, _ := json.MarshalIndent(results, "", "  ")
		fmt.Fprintln(stdout, string(b))
		if chosen == "" {
			return 1
		}
		return 0
	}

	if chosen != "" {
		fmt.Fprintf(stdout, "model %q — would route to: %s\n", model, chosen)
	} else {
		fmt.Fprintf(stdout, "model %q — no reachable peer serves it\n", model)
	}
	for _, e := range results {
		switch {
		case !e.Reachable:
			fmt.Fprintf(stdout, "  %-14s %s  UNREACHABLE: %s\n", e.Name, e.URL, e.Error)
		case e.Chosen:
			fmt.Fprintf(stdout, "  %-14s %s  serves (chosen)\n", e.Name, e.URL)
		case e.Serves:
			fmt.Fprintf(stdout, "  %-14s %s  serves (fallback)\n", e.Name, e.URL)
		default:
			fmt.Fprintf(stdout, "  %-14s %s  does not serve\n", e.Name, e.URL)
		}
	}
	if chosen == "" {
		return 1
	}
	return 0
}

type peerRunEvent struct {
	ID            string `json:"id"`
	Seq           int64  `json:"seq"`
	TSUnixMS      int64  `json:"ts_unix_ms"`
	Subject       string `json:"subject"`
	Actor         string `json:"actor"`
	Kind          string `json:"kind"`
	CorrelationID string `json:"correlation_id"`
	Hash          string `json:"hash,omitempty"`
}

type peerRunArc struct {
	Peer          string         `json:"peer"`
	URL           string         `json:"url"`
	CorrelationID string         `json:"correlation_id"`
	Count         int            `json:"count"`
	Events        []peerRunEvent `json:"events"`
	Error         string         `json:"error,omitempty"`
}

func peersRun(peers map[string]peer.Peer, name, corr string, asJSON bool, stdout, stderr io.Writer) int {
	p, ok := peers[name]
	if !ok {
		fmt.Fprintf(stderr, "%s peers run: unknown peer %q\n", brand.CLI, name)
		return 1
	}
	corr = strings.TrimSpace(corr)
	if corr == "" {
		fmt.Fprintf(stderr, "%s peers run: correlation id is required\n", brand.CLI)
		return 2
	}
	arc := fetchPeerRun(p, corr)
	if asJSON {
		b, _ := json.MarshalIndent(arc, "", "  ")
		fmt.Fprintln(stdout, string(b))
		if arc.Error != "" {
			return 1
		}
		return 0
	}
	if arc.Error != "" {
		fmt.Fprintf(stdout, "  %-14s %s  UNREACHABLE: %s\n", arc.Peer, arc.URL, arc.Error)
		return 1
	}
	fmt.Fprintf(stdout, "peer %s run %s: %d event(s)\n", arc.Peer, arc.CorrelationID, arc.Count)
	for _, ev := range arc.Events {
		hash := ev.Hash
		if len(hash) > 12 {
			hash = hash[:12]
		}
		fmt.Fprintf(stdout, "  #%d %-22s %-28s %s\n", ev.Seq, ev.Kind, ev.Subject, hash)
	}
	return 0
}



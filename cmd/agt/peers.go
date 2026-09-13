// SPDX-License-Identifier: MIT

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/plugins/tools/peer"
)


// cmdPeers implements `agt peers` (M8 mesh): list the peer Agezt nodes
// configured via AGEZT_PEERS and check each one's health over its native REST
// surface (GET /api/v1/health). It is a self-contained client command — it reads
// the same AGEZT_PEERS spec the daemon's remote_run tool uses and pings the
// peers directly, so an operator can verify the mesh wiring without a local
// daemon. Tokens are never printed.
func cmdPeers(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	verb := "list"
	var pos []string
	for _, a := range args {
		switch {
		case a == "--json":
			asJSON = true
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s peers [list | models [<name>] | route <model> | run <peer> <corr> | artifacts <peer> <corr> | artifact-get <peer> <artifact_id> <out_file>] [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "  list    configured peers (AGEZT_PEERS) + each one's REST /api/v1/health\n")
			fmt.Fprintf(stdout, "  models  the models each peer can route (GET /api/v1/models); <name> filters to one\n")
			fmt.Fprintf(stdout, "  route   which peer remote_run would auto-route a <model> to, and the fallback order\n")
			fmt.Fprintf(stdout, "  run     metadata-only remote run event arc (GET /api/v1/runs/<corr>); payloads are not printed\n")
			fmt.Fprintf(stdout, "  artifacts metadata-only remote artifact index entries (GET /api/v1/artifacts?corr=<corr>); bytes are not printed\n")
			fmt.Fprintf(stdout, "  artifact-get policy-gated remote artifact bytes (GET /api/v1/artifacts/<id>/bytes); writes <out_file>\n")
			return 0
		case a == "list" || a == "models" || a == "route" || a == "run" || a == "artifacts" || a == "artifact-get":
			verb = a
		case strings.HasPrefix(a, "-"):
			fmt.Fprintf(stderr, "%s peers: unknown flag %q\n", brand.CLI, a)
			return 2
		default:
			pos = append(pos, a)
		}
	}
	switch verb {
	case "list":
		if len(pos) != 0 {
			fmt.Fprintf(stderr, "%s peers list: unexpected argument %q\n", brand.CLI, pos[0])
			return 2
		}
	case "models":
		if len(pos) > 1 {
			fmt.Fprintf(stderr, "%s peers models: unexpected argument %q\n", brand.CLI, pos[1])
			return 2
		}
	case "route":
		if len(pos) != 1 {
			fmt.Fprintf(stderr, "%s peers route: a <model> is required\n", brand.CLI)
			return 2
		}
	case "run":
		if len(pos) != 2 {
			fmt.Fprintf(stderr, "%s peers run: <peer> and <correlation_id> are required\n", brand.CLI)
			return 2
		}
	case "artifacts":
		if len(pos) != 2 {
			fmt.Fprintf(stderr, "%s peers artifacts: <peer> and <correlation_id> are required\n", brand.CLI)
			return 2
		}
	case "artifact-get":
		if len(pos) != 3 {
			fmt.Fprintf(stderr, "%s peers artifact-get: <peer>, <artifact_id>, and <out_file> are required\n", brand.CLI)
			return 2
		}
	default:
		fmt.Fprintf(stderr, "%s peers: unknown subcommand %q\n", brand.CLI, verb)
		return 2
	}

	peers, err := peer.ParsePeers(os.Getenv(brand.EnvPrefix + "PEERS"))
	if err != nil {
		fmt.Fprintf(stderr, "%s peers: %v\n", brand.CLI, err)
		return 1
	}
	if len(peers) == 0 {
		if verb == "run" || verb == "models" || verb == "route" || verb == "artifacts" || verb == "artifact-get" {
			fmt.Fprintf(stderr, "%s peers %s: no peers configured (set AGEZT_PEERS=\"name=url|token,…\")\n", brand.CLI, verb)
			return 1
		}
		if asJSON {
			fmt.Fprintln(stdout, "[]")
		} else {
			fmt.Fprintf(stdout, "No peers configured. Set AGEZT_PEERS=\"name=url|token,…\".\n")
		}
		return 0
	}

	if verb == "models" {
		name := ""
		if len(pos) == 1 {
			name = pos[0]
		}
		return peersModels(peers, name, asJSON, stdout, stderr)
	}
	if verb == "route" {
		return peersRoute(peers, pos[0], asJSON, stdout, stderr)
	}
	if verb == "run" {
		return peersRun(peers, pos[0], pos[1], asJSON, stdout, stderr)
	}
	if verb == "artifacts" {
		return peersArtifacts(peers, pos[0], pos[1], asJSON, stdout, stderr)
	}
	if verb == "artifact-get" {
		return peersArtifactGet(peers, pos[0], pos[1], pos[2], asJSON, stdout, stderr)
	}

	names := make([]string, 0, len(peers))
	for n := range peers {
		names = append(names, n)
	}
	sort.Strings(names)

	results := make([]peerHealth, 0, len(names))
	for _, n := range names {
		results = append(results, checkPeer(peers[n]))
	}

	if asJSON {
		b, _ := json.MarshalIndent(results, "", "  ")
		fmt.Fprintln(stdout, string(b))
		return 0
	}

	allOK := true
	for _, r := range results {
		if r.Reachable {
			fmt.Fprintf(stdout, "  %-14s %s  OK (version %s, %d model(s))\n", r.Name, r.URL, r.Version, r.ModelCount)
		} else {
			allOK = false
			fmt.Fprintf(stdout, "  %-14s %s  UNREACHABLE: %s\n", r.Name, r.URL, r.Error)
		}
	}
	if !allOK {
		return 1
	}
	return 0
}

// maxPeerResponseBytes caps a peer's REST response (health / models). A legitimate
// reply is a few bytes; the cap matches the remote_run tool's 1 MiB peer-response
// limit so a hostile/misconfigured peer can't exhaust the CLI's memory (M200/M201).
const maxPeerResponseBytes = 1 << 20
const maxPeerRunResponseBytes = 4 << 20
const maxPeerArtifactResponseBytes = 1 << 20
const maxPeerArtifactBytesResponseBytes = 96 << 20
const maxPeerArtifactEntries = 200

type peerHealth struct {
	Name       string `json:"name"`
	URL        string `json:"url"`
	Reachable  bool   `json:"reachable"`
	Version    string `json:"version,omitempty"`
	ModelCount int    `json:"model_count,omitempty"`
	Error      string `json:"error,omitempty"`
}


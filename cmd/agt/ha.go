// SPDX-License-Identifier: MIT
//
// cmd/agt `ha` top-level dispatch (cmdHA) + states/services verbs
// (haStates, haServices) + usage helper (haUsage).
// Extracted from ha.go during Day 211 god-file refactor (#75).
// Public API unchanged.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"

	"github.com/agezt/agezt/internal/brand"
)

func haUsage(w io.Writer) {
	fmt.Fprintf(w, "usage: %s ha <command>\n", brand.CLI)
	fmt.Fprintf(w, "operator-facing Home Assistant client (reads %sHOMEASSISTANT_URL/_TOKEN)\n\n", brand.EnvPrefix)
	fmt.Fprintf(w, "commands:\n")
	fmt.Fprintf(w, "  states [entity_id] [--json]      list all entity states, or one entity\n")
	fmt.Fprintf(w, "  services [--json]                list the service registry (domain.service)\n")
	fmt.Fprintf(w, "  call <domain.service> [opts]     call a service\n")
	fmt.Fprintf(w, "      --entity <id>                target entity (e.g. light.living_room)\n")
	fmt.Fprintf(w, "      --data '<json>'              extra service data (e.g. '{\"brightness\":128}')\n")
	fmt.Fprintf(w, "      --json                       print the raw JSON response\n")
}
func cmdHA(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		haUsage(stdout)
		return 0
	}
	base := strings.TrimRight(strings.TrimSpace(os.Getenv(brand.EnvPrefix+"HOMEASSISTANT_URL")), "/")
	token := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "HOMEASSISTANT_TOKEN"))
	if base == "" || token == "" {
		fmt.Fprintf(stderr, "%s ha: set %sHOMEASSISTANT_URL and %sHOMEASSISTANT_TOKEN\n", brand.CLI, brand.EnvPrefix, brand.EnvPrefix)
		return 2
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "states":
		return haStates(base, token, rest, stdout, stderr)
	case "services":
		return haServices(base, token, rest, stdout, stderr)
	case "call":
		return haCall(base, token, rest, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "%s ha: unknown command %q\n", brand.CLI, sub)
		haUsage(stderr)
		return 2
	}
}
func haStates(base, token string, args []string, stdout, stderr io.Writer) int {
	var entity string
	raw := false
	for _, a := range args {
		switch {
		case a == "--json":
			raw = true
		case strings.HasPrefix(a, "-"):
			fmt.Fprintf(stderr, "%s ha states: unknown flag %q\n", brand.CLI, a)
			return 2
		case entity == "":
			entity = a
		default:
			fmt.Fprintf(stderr, "%s ha states: too many arguments\n", brand.CLI)
			return 2
		}
	}

	if entity != "" {
		status, body, err := haRequest(http.MethodGet, base, token, "/api/states/"+url.PathEscape(entity), nil)
		if code := haCheck(status, body, err, stderr); code != 0 {
			return code
		}
		return printBodyJSON(body, raw, stdout)
	}

	status, body, err := haRequest(http.MethodGet, base, token, "/api/states", nil)
	if code := haCheck(status, body, err, stderr); code != 0 {
		return code
	}
	if raw {
		return printBodyJSON(body, true, stdout)
	}
	var states []struct {
		EntityID string `json:"entity_id"`
		State    string `json:"state"`
	}
	if err := json.Unmarshal(body, &states); err != nil {
		fmt.Fprintf(stderr, "%s ha states: parse response: %v\n", brand.CLI, err)
		return 1
	}
	sort.Slice(states, func(i, j int) bool { return states[i].EntityID < states[j].EntityID })
	for _, s := range states {
		fmt.Fprintf(stdout, "%s = %s\n", s.EntityID, s.State)
	}
	fmt.Fprintf(stdout, "(%d entities)\n", len(states))
	return 0
}
func haServices(base, token string, args []string, stdout, stderr io.Writer) int {
	raw := false
	for _, a := range args {
		if a == "--json" {
			raw = true
			continue
		}
		fmt.Fprintf(stderr, "%s ha services: unknown argument %q\n", brand.CLI, a)
		return 2
	}
	status, body, err := haRequest(http.MethodGet, base, token, "/api/services", nil)
	if code := haCheck(status, body, err, stderr); code != 0 {
		return code
	}
	if raw {
		return printBodyJSON(body, true, stdout)
	}
	var domains []struct {
		Domain   string                     `json:"domain"`
		Services map[string]json.RawMessage `json:"services"`
	}
	if err := json.Unmarshal(body, &domains); err != nil {
		fmt.Fprintf(stderr, "%s ha services: parse response: %v\n", brand.CLI, err)
		return 1
	}
	var names []string
	for _, d := range domains {
		for svc := range d.Services {
			names = append(names, d.Domain+"."+svc)
		}
	}
	sort.Strings(names)
	for _, n := range names {
		fmt.Fprintln(stdout, n)
	}
	fmt.Fprintf(stdout, "(%d services)\n", len(names))
	return 0
}

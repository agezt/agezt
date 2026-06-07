// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
)

// cmdHA implements `agt ha …` — an operator-facing Home Assistant client that
// reads entity state, lists the service registry, and calls services directly
// against the HA instance configured by AGEZT_HOMEASSISTANT_URL/_TOKEN.
//
// This is the operator complement to the agent-facing `homeassistant` TOOL
// (M575): the tool is fail-closed behind read/service allowlists so a
// prompt-injected agent is constrained, whereas this CLI is the OPERATOR acting
// with their own authority — so it has full access and needs no daemon running.
// `agt ha services` is also the introspection the operator uses to discover
// which `domain.service` names to put in AGEZT_HOMEASSISTANT_TOOL_SERVICES.
const (
	haCLITimeout = 30 * time.Second
	haMaxBody    = 1 << 20 // cap the response we read for display (1 MiB)
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

// haStates reads all entity states (sorted "entity_id = state" lines) or one
// entity (pretty JSON). --json prints the raw response.
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

// haServices lists the service registry as sorted "domain.service" lines (the
// names an operator puts in AGEZT_HOMEASSISTANT_TOOL_SERVICES). --json is raw.
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

// haCall calls a service: `agt ha call <domain.service> [--entity id] [--data json]`.
func haCall(base, token string, args []string, stdout, stderr io.Writer) int {
	var target, entity, data string
	raw := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json":
			raw = true
		case a == "--entity":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s ha call: --entity needs a value\n", brand.CLI)
				return 2
			}
			i++
			entity = args[i]
		case strings.HasPrefix(a, "--entity="):
			entity = strings.TrimPrefix(a, "--entity=")
		case a == "--data":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s ha call: --data needs a value\n", brand.CLI)
				return 2
			}
			i++
			data = args[i]
		case strings.HasPrefix(a, "--data="):
			data = strings.TrimPrefix(a, "--data=")
		case strings.HasPrefix(a, "-"):
			fmt.Fprintf(stderr, "%s ha call: unknown flag %q\n", brand.CLI, a)
			return 2
		case target == "":
			target = a
		default:
			fmt.Fprintf(stderr, "%s ha call: unexpected argument %q\n", brand.CLI, a)
			return 2
		}
	}
	domain, service, ok := strings.Cut(target, ".")
	if !ok || domain == "" || service == "" {
		fmt.Fprintf(stderr, "usage: %s ha call <domain.service> [--entity id] [--data json]\n", brand.CLI)
		return 2
	}

	payload := map[string]any{}
	if strings.TrimSpace(data) != "" {
		if err := json.Unmarshal([]byte(data), &payload); err != nil {
			fmt.Fprintf(stderr, "%s ha call: --data is not valid JSON: %v\n", brand.CLI, err)
			return 2
		}
	}
	if entity != "" {
		payload["entity_id"] = entity
	}
	enc, err := json.Marshal(payload)
	if err != nil {
		fmt.Fprintf(stderr, "%s ha call: %v\n", brand.CLI, err)
		return 1
	}

	path := "/api/services/" + url.PathEscape(domain) + "/" + url.PathEscape(service)
	status, body, err := haRequest(http.MethodPost, base, token, path, enc)
	if code := haCheck(status, body, err, stderr); code != 0 {
		return code
	}
	fmt.Fprintf(stdout, "called %s.%s ok\n", domain, service)
	return printBodyJSON(body, raw, stdout)
}

// haRequest performs one bearer-authenticated request with a size-capped read.
func haRequest(method, base, token, path string, body []byte) (int, []byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), haCLITimeout)
	defer cancel()
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, base+path, r)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	out, err := io.ReadAll(io.LimitReader(resp.Body, haMaxBody+1))
	if err != nil {
		return resp.StatusCode, nil, err
	}
	if len(out) > haMaxBody {
		out = out[:haMaxBody]
	}
	return resp.StatusCode, out, nil
}

// haCheck turns a transport error or non-2xx status into a CLI error (exit 1),
// or returns 0 to proceed.
func haCheck(status int, body []byte, err error, stderr io.Writer) int {
	if err != nil {
		fmt.Fprintf(stderr, "%s ha: %v\n", brand.CLI, err)
		return 1
	}
	if status/100 != 2 {
		msg := strings.TrimSpace(string(body))
		if msg == "" {
			msg = http.StatusText(status)
		}
		fmt.Fprintf(stderr, "%s ha: HTTP %d: %s\n", brand.CLI, status, msg)
		return 1
	}
	return 0
}

// printBodyJSON pretty-prints a JSON body (or echoes it raw when it isn't JSON,
// or when raw is requested).
func printBodyJSON(body []byte, raw bool, stdout io.Writer) int {
	if raw {
		fmt.Fprintln(stdout, strings.TrimRight(string(body), "\n"))
		return 0
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, body, "", "  "); err != nil {
		// Not JSON — echo as-is rather than failing.
		fmt.Fprintln(stdout, strings.TrimRight(string(body), "\n"))
		return 0
	}
	fmt.Fprintln(stdout, buf.String())
	return 0
}

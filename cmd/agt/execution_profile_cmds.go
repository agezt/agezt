// SPDX-License-Identifier: MIT
//
// cmd/agt `exec-profile list` + `show` + `check` subcommands
// (cmdExecProfileList, cmdExecProfileShow, cmdExecProfileCheck).
// Extracted from execution_profile.go during Day 211 god-file refactor (#94).
// Public API unchanged.
package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/agezt/agezt/cmd/agt/jsonout"
	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
)

func cmdExecProfileList(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	tenant := ""
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json":
			asJSON = true
		case a == "--tenant":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s exec-profile list: --tenant needs an id\n", brand.CLI)
				return 2
			}
			i++
			tenant = args[i]
		case strings.HasPrefix(a, "--tenant="):
			tenant = strings.TrimPrefix(a, "--tenant=")
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s exec-profile list [--tenant <id>] [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "list named execution profiles and their requested vs effective isolation\n")
			return 0
		default:
			fmt.Fprintf(stderr, "%s exec-profile list: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}
	res, ok := callExecProfile(controlplane.CmdExecutionProfiles, tenantArg(tenant), stderr)
	if !ok {
		return 1
	}
	if asJSON {
		return jsonout.Write(stdout, res)
	}
	fmt.Fprintf(stdout, "%d execution profile(s) on %s/%s:\n", intOfStatus(res["count"]), str(res["host_os"]), str(res["host_arch"]))
	rows, _ := res["profiles"].([]any)
	for _, raw := range rows {
		p, _ := raw.(map[string]any)
		route := "not routed"
		if p["routed"] == true {
			route = "routed"
		}
		iso := fmt.Sprintf("%s -> %s", str(p["requested_isolation"]), str(p["effective_isolation"]))
		if p["degraded"] == true {
			iso += " (degraded)"
		}
		tools := strings.Join(toStringSlice(p["tools"]), ",")
		if tools == "" {
			tools = "-"
		}
		fmt.Fprintf(stdout, "  %-16s %-9s %-10s %-28s tools: %s\n", str(p["id"]), str(p["status"]), route, iso, tools)
	}
	return 0
}
func cmdExecProfileShow(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	tenant := ""
	id := ""
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json":
			asJSON = true
		case a == "--tenant":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s exec-profile show: --tenant needs an id\n", brand.CLI)
				return 2
			}
			i++
			tenant = args[i]
		case strings.HasPrefix(a, "--tenant="):
			tenant = strings.TrimPrefix(a, "--tenant=")
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s exec-profile show <id> [--tenant <id>] [--json]\n", brand.CLI)
			return 0
		default:
			if id == "" {
				id = a
			} else {
				fmt.Fprintf(stderr, "%s exec-profile show: unexpected arg %q\n", brand.CLI, a)
				return 2
			}
		}
	}
	if strings.TrimSpace(id) == "" {
		fmt.Fprintf(stderr, "%s exec-profile show: id required\n", brand.CLI)
		return 2
	}
	argsMap := tenantArg(tenant)
	argsMap["id"] = id
	res, ok := callExecProfile(controlplane.CmdExecutionProfileShow, argsMap, stderr)
	if !ok {
		return 1
	}
	if asJSON {
		return jsonout.Write(stdout, res)
	}
	p, _ := res["profile"].(map[string]any)
	if len(p) == 0 {
		fmt.Fprintf(stderr, "%s exec-profile show: empty profile\n", brand.CLI)
		return 1
	}
	fmt.Fprintf(stdout, "%s (%s)\n", str(p["name"]), str(p["id"]))
	fmt.Fprintf(stdout, "  status     : %s\n", str(p["status"]))
	fmt.Fprintf(stdout, "  routed     : %v\n", p["routed"] == true)
	fmt.Fprintf(stdout, "  isolation  : %s -> %s\n", str(p["requested_isolation"]), str(p["effective_isolation"]))
	if p["degraded"] == true {
		fmt.Fprintf(stdout, "  degraded   : %s\n", str(p["degrade_reason"]))
	}
	fmt.Fprintf(stdout, "  tools      : %s\n", dashJoin(toStringSlice(p["tools"])))
	fmt.Fprintf(stdout, "  backends   : %s\n", dashJoin(toStringSlice(p["backends"])))
	fmt.Fprintf(stdout, "  filesystem : %s\n", str(p["filesystem"]))
	fmt.Fprintf(stdout, "  network    : %s\n", str(p["network"]))
	fmt.Fprintf(stdout, "  env        : %s\n", str(p["environment"]))
	fmt.Fprintf(stdout, "  secrets    : %s\n", str(p["secrets"]))
	if sp, ok := p["secret_policy"].(map[string]any); ok && len(sp) > 0 {
		fmt.Fprintf(stdout, "  secret_policy: %s (values_forwarded=%v, metadata_forwarded=%v)\n",
			str(sp["mode"]), sp["values_forwarded"] == true, sp["metadata_forwarded"] == true)
	}
	fmt.Fprintf(stdout, "  limits     : %s\n", dashJoin(toStringSlice(p["limits"])))
	fmt.Fprintf(stdout, "  browser    : %s\n", str(p["browser_access"]))
	fmt.Fprintf(stdout, "  cleanup    : %s\n", str(p["cleanup"]))
	if cap := str(p["policy_capability"]); cap != "" {
		fmt.Fprintf(stdout, "  policy     : %s\n", cap)
	}
	return 0
}
func cmdExecProfileCheck(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	tenant := ""
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json":
			asJSON = true
		case a == "--tenant":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s exec-profile check: --tenant needs an id\n", brand.CLI)
				return 2
			}
			i++
			tenant = args[i]
		case strings.HasPrefix(a, "--tenant="):
			tenant = strings.TrimPrefix(a, "--tenant=")
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s exec-profile check [--tenant <id>] [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "check execution-profile routing, policy, downgrade, and backend availability\n")
			return 0
		default:
			fmt.Fprintf(stderr, "%s exec-profile check: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}
	res, ok := callExecProfile(controlplane.CmdExecutionProfileCheck, tenantArg(tenant), stderr)
	if !ok {
		return 1
	}
	if asJSON {
		return jsonout.Write(stdout, res)
	}
	fmt.Fprintf(stdout, "execution profile check on %s/%s: %d ok, %d warning, %d fail\n",
		str(res["host_os"]), str(res["host_arch"]), intOfStatus(res["ok_count"]), intOfStatus(res["warning_count"]), intOfStatus(res["fail_count"]))
	fmt.Fprintf(stdout, "selectable run profiles: %s\n", dashJoin(toStringSlice(res["routable_run_profiles"])))
	checks, _ := res["checks"].([]any)
	for _, raw := range checks {
		c, _ := raw.(map[string]any)
		status := str(c["status"])
		if status == "" {
			status = "unknown"
		}
		fmt.Fprintf(stdout, "  [%-7s] %-16s %s\n", status, str(c["profile_id"]), str(c["title"]))
		if detail := str(c["detail"]); detail != "" {
			fmt.Fprintf(stdout, "           %s\n", detail)
		}
		if next := str(c["next"]); next != "" {
			fmt.Fprintf(stdout, "           next: %s\n", next)
		}
	}
	if intOfStatus(res["fail_count"]) > 0 {
		return 1
	}
	return 0
}

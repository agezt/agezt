// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
	dialpkg "github.com/agezt/agezt/cmd/agt/dial"
	"github.com/agezt/agezt/cmd/agt/jsonout"
)

// cmdMemory dispatches `agt memory <subcommand>`. Memory-lite is the
// content-addressed, journaled knowledge store the agent reads as injected
// context; this is the operator's read/write path into it.
func cmdMemory(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintf(stderr, "%s memory: subcommand required (add|list|log|search|get|forget|audit|clean|consolidate|profile|prune|bulk-forget|find-related)\n", brand.CLI)
		return 2
	}
	switch args[0] {
	case "add":
		return cmdMemoryAdd(args[1:], stdout, stderr)
	case "list", "ls":
		return cmdMemoryList(args[1:], stdout, stderr)
	case "log":
		return cmdMemoryLog(args[1:], stdout, stderr)
	case "search":
		return cmdMemorySearch(args[1:], stdout, stderr)
	case "get":
		return cmdMemoryGet(args[1:], stdout, stderr)
	case "forget", "rm":
		return cmdMemoryForget(args[1:], stdout, stderr)
	case "promote":
		return cmdMemoryPromote(args[1:], stdout, stderr)
	case "audit":
		return cmdMemoryAudit(args[1:], stdout, stderr)
	case "clean":
		return cmdMemoryClean(args[1:], stdout, stderr)
	case "consolidate":
		return cmdMemoryConsolidate(args[1:], stdout, stderr)
	case "profile":
		return cmdMemoryProfile(args[1:], stdout, stderr)
	case "prune":
		return cmdMemoryPrune(args[1:], stdout, stderr)
	case "bulk-forget":
		return cmdMemoryBulkForget(args[1:], stdout, stderr)
	case "find-related":
		return cmdMemoryFindRelated(args[1:], stdout, stderr)
	case "-h", "--help", "help":
		fmt.Fprintf(stdout, "usage: %s memory <subcommand>\n", brand.CLI)
		fmt.Fprintf(stdout, "  add <subject> <content> [--type T] [--tag k=v] [--conf F] [--json]\n")
		fmt.Fprintf(stdout, "  list [--json]\n")
		fmt.Fprintf(stdout, "  log [N] [--op written|forgotten|superseded|promoted] [--since <dur>] [--json]\n")
		fmt.Fprintf(stdout, "  search <query> [N] [--json]\n")
		fmt.Fprintf(stdout, "  get <id> [--json]      (exit 3 = absent)\n")
		fmt.Fprintf(stdout, "  forget <id> [--json]\n")
		fmt.Fprintf(stdout, "  promote <id> [--json]  share a private (agent-scoped) record with every agent\n")
		fmt.Fprintf(stdout, "  audit [--json]         report expired, suspended, and competing memories\n")
		fmt.Fprintf(stdout, "  clean [--execute]      hard-delete low-value log/transient memory records (dry-run by default)\n")
		fmt.Fprintf(stdout, "  consolidate [--json]   one brain-distillation pass: merge related records, supersede originals\n")
		fmt.Fprintf(stdout, "  profile [--json]       rebuild the operator profile from accumulated memory (M1000)\n")
		fmt.Fprintf(stdout, "  prune [--days N] [--execute]   hard-delete soft-deleted records (dry-run by default)\n")
		fmt.Fprintf(stdout, "  bulk-forget <id>...    soft-delete multiple records in one call\n")
		fmt.Fprintf(stdout, "  find-related --id <id> [--limit N] [--json]\n")
		return 0
	default:
		fmt.Fprintf(stderr, "%s memory: unknown subcommand %q (add|list|log|search|get|forget|promote|audit|clean|consolidate|profile|prune|bulk-forget|find-related)\n", brand.CLI, args[0])
		return 2
	}
}

// cmdMemoryAdd implements `agt memory add <subject> <content> [flags]`.
func cmdMemoryAdd(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	typ := ""
	conf := 0.0
	evidence := ""
	halfLifeMS := int64(0)
	tags := map[string]any{}
	var positional []string

	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json":
			asJSON = true
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s memory add <subject> <content> [--type T] [--evidence E] [--half-life D] [--tag k=v] [--conf F] [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "types: FACT (default) | SUMMARY | RELATION | PREFERENCE | OBSERVATION\n")
			fmt.Fprintf(stdout, "evidence: observed | inferred | curated | constraint\n")
			return 0
		case a == "--type":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s memory add: --type needs a value\n", brand.CLI)
				return 2
			}
			i++
			typ = strings.ToUpper(args[i])
		case a == "--evidence":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s memory add: --evidence needs a value\n", brand.CLI)
				return 2
			}
			i++
			evidence = strings.ToLower(args[i])
			if !validMemoryEvidence(evidence) {
				fmt.Fprintf(stderr, "%s memory add: bad --evidence %q\n", brand.CLI, args[i])
				return 2
			}
		case a == "--half-life":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s memory add: --half-life needs a duration\n", brand.CLI)
				return 2
			}
			i++
			d, err := time.ParseDuration(args[i])
			if err != nil || d <= 0 {
				fmt.Fprintf(stderr, "%s memory add: bad --half-life %q\n", brand.CLI, args[i])
				return 2
			}
			halfLifeMS = d.Milliseconds()
		case a == "--conf":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s memory add: --conf needs a value\n", brand.CLI)
				return 2
			}
			i++
			f, err := strconv.ParseFloat(args[i], 64)
			if err != nil {
				fmt.Fprintf(stderr, "%s memory add: --conf must be a number: %v\n", brand.CLI, err)
				return 2
			}
			conf = f
		case a == "--tag":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s memory add: --tag needs k=v\n", brand.CLI)
				return 2
			}
			i++
			k, v, ok := strings.Cut(args[i], "=")
			if !ok || k == "" {
				fmt.Fprintf(stderr, "%s memory add: --tag must be k=v, got %q\n", brand.CLI, args[i])
				return 2
			}
			tags[k] = v
		default:
			positional = append(positional, a)
		}
	}

	// Grammar: <subject> <content>. A single positional is treated as
	// content with an empty subject (subjects are optional in the store).
	var subject, content string
	switch len(positional) {
	case 1:
		content = positional[0]
	case 2:
		subject, content = positional[0], positional[1]
	default:
		fmt.Fprintf(stderr, "%s memory add: expected <subject> <content> (or just <content>)\n", brand.CLI)
		return 2
	}

	callArgs := map[string]any{"subject": subject, "content": content}
	if typ != "" {
		callArgs["type"] = typ
	}
	if conf > 0 {
		callArgs["confidence"] = conf
	}
	if evidence != "" {
		callArgs["evidence"] = evidence
	}
	if halfLifeMS > 0 {
		callArgs["half_life_ms"] = halfLifeMS
	}
	if len(tags) > 0 {
		callArgs["tags"] = tags
	}

	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdMemoryAdd, callArgs)
	if err != nil {
		fmt.Fprintf(stderr, "%s memory add: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return jsonout.Write(stdout, res)
	}
	id, _ := res["id"].(string)
	created, _ := res["created"].(bool)
	verb := "reinforced"
	if created {
		verb = "stored"
	}
	fmt.Fprintf(stdout, "%s %s\n", verb, id)
	return 0
}

func validMemoryEvidence(v string) bool {
	switch v {
	case "observed", "inferred", "curated", "constraint":
		return true
	default:
		return false
	}
}

// cmdMemoryList implements `agt memory list [--json]`.
func cmdMemoryList(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s memory list [--json]\n", brand.CLI)
			return 0
		default:
			fmt.Fprintf(stderr, "%s memory list: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdMemoryList, nil)
	if err != nil {
		fmt.Fprintf(stderr, "%s memory list: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return jsonout.Write(stdout, res)
	}
	recs, _ := res["records"].([]any)
	if len(recs) == 0 {
		fmt.Fprintln(stdout, "no memory records")
		return 0
	}
	fmt.Fprintf(stdout, "%d record(s):\n", len(recs))
	for _, raw := range recs {
		if r, ok := raw.(map[string]any); ok {
			fmt.Fprintln(stdout, renderRecordLine(r))
		}
	}
	return 0
}

// cmdMemorySearch implements `agt memory search <query> [N] [--json]`.
func cmdMemorySearch(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	var query string
	limit := 0
	for _, a := range args {
		switch {
		case a == "--json":
			asJSON = true
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s memory search <query> [N] [--json]\n", brand.CLI)
			return 0
		case query == "":
			query = a
		default:
			if n, err := strconv.Atoi(a); err == nil {
				limit = n
				continue
			}
			// Allow multi-word queries: fold extra words into the query.
			query += " " + a
		}
	}
	if query == "" {
		fmt.Fprintf(stderr, "%s memory search: query required\n", brand.CLI)
		return 2
	}
	callArgs := map[string]any{"query": query}
	if limit > 0 {
		callArgs["limit"] = limit
	}
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdMemorySearch, callArgs)
	if err != nil {
		fmt.Fprintf(stderr, "%s memory search: %v\n", brand.CLI, err)
		return 1
	}
	if asJSON {
		return jsonout.Write(stdout, res)
	}
	results, _ := res["results"].([]any)
	if len(results) == 0 {
		fmt.Fprintf(stdout, "no records match %q\n", query)
		return 0
	}
	fmt.Fprintf(stdout, "%d match(es) for %q:\n", len(results), query)
	for _, raw := range results {
		m, _ := raw.(map[string]any)
		rec, _ := m["record"].(map[string]any)
		score, _ := m["score"].(float64)
		fmt.Fprintf(stdout, "  (%.2f) %s\n", score, renderRecordLine(rec))
	}
	return 0
}

// cmdMemoryGet implements `agt memory get <id> [--json]`. Exit 3 = absent.
func cmdMemoryGet(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	var id string
	for _, a := range args {
		switch {
		case a == "--json":
			asJSON = true
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s memory get <id> [--json]\n", brand.CLI)
			fmt.Fprintf(stdout, "exit 0 = found, 3 = absent, 1 = error\n")
			return 0
		case id == "":
			id = a
		default:
			fmt.Fprintf(stderr, "%s memory get: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}
	if id == "" {
		fmt.Fprintf(stderr, "%s memory get: id required\n", brand.CLI)
		return 2
	}
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdMemoryGet, map[string]any{"id": id})
	if err != nil {
		fmt.Fprintf(stderr, "%s memory get: %v\n", brand.CLI, err)
		return 1
	}
	found, _ := res["found"].(bool)
	if asJSON {
		_ = jsonout.Write(stdout, res)
		if !found {
			return 3
		}
		return 0
	}
	if !found {
		fmt.Fprintf(stderr, "%s memory get: %s not found\n", brand.CLI, id)
		return 3
	}
	rec, _ := res["record"].(map[string]any)
	return jsonout.Write(stdout, rec)
}

// cmdMemoryForget implements `agt memory forget <id> [--json]`.

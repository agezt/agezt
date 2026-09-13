// SPDX-License-Identifier: MIT

package main

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/controlplane"
	dialpkg "github.com/agezt/agezt/cmd/agt/dial"
	"github.com/agezt/agezt/cmd/agt/jsonout"
)

// cmdSkillWorkshop is the Forge Workshop operator surface: proposals are still
// ordinary Forge skills, but the commands speak in review verbs instead of raw
// lifecycle verbs.
func cmdSkillWorkshop(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return skillWorkshopUsage(stderr)
	}
	switch args[0] {
	case "list", "ls":
		return cmdSkillWorkshopList(args[1:], stdout, stderr)
	case "inspect", "show", "get":
		return cmdSkillWorkshopInspect(args[1:], stdout, stderr)
	case "scan":
		return cmdSkillWorkshopScan(args[1:], stdout, stderr)
	case "diff":
		return cmdSkillDiff(args[1:], stdout, stderr)
	case "curate", "curator":
		return cmdSkillWorkshopCurate(args[1:], stdout, stderr)
	case "apply", "promote":
		return cmdSkillWorkshopApply(args[1:], stdout, stderr)
	case "reject":
		return cmdSkillWorkshopReject(args[1:], stdout, stderr)
	case "quarantine":
		return cmdSkillWorkshopQuarantine(args[1:], stdout, stderr)
	case "propose", "import":
		return cmdSkillImport(args[1:], stdout, stderr)
	case "propose-create", "create":
		return cmdSkillWorkshopProposeCreate(args[1:], stdout, stderr)
	case "propose-update", "update":
		return cmdSkillWorkshopProposeUpdate(args[1:], stdout, stderr)
	case "-h", "--help", "help":
		return skillWorkshopUsage(stdout)
	default:
		fmt.Fprintf(stderr, "%s skill workshop: unknown subcommand %q\n", brand.CLI, args[0])
		return skillWorkshopUsage(stderr)
	}
}

func skillWorkshopUsage(w io.Writer) int {
	fmt.Fprintf(w, "usage: %s skill workshop <list|inspect|scan|diff|curate|apply|reject|quarantine|propose|propose-create|propose-update>\n", brand.CLI)
	fmt.Fprintf(w, "  list [--json]                                      pending draft/shadow proposals\n")
	fmt.Fprintf(w, "  inspect <id> [--json]                              proposal, lineage, resources, and lifecycle history\n")
	fmt.Fprintf(w, "  scan <id> [--json]                                 deterministic risk scan for a proposal\n")
	fmt.Fprintf(w, "  diff <id> [<id2>]                                  diff a proposal against its parent, or old->new\n")
	fmt.Fprintf(w, "  curate [--idle-days N] [--execute] [--json]        deterministic stale-skill cleanup (dry-run by default)\n")
	fmt.Fprintf(w, "  apply <id> [--json]                                advance one gate: draft->shadow or shadow->active\n")
	fmt.Fprintf(w, "  reject <id> [--reason R] [--json]                  archive a proposal with a journaled reason\n")
	fmt.Fprintf(w, "  quarantine <id> [--reason R] [--json]              pull a live/shadow skill from production\n")
	fmt.Fprintf(w, "  propose <bundle|dir|SKILL.md> [--json]             import a portable skill as a draft proposal\n")
	fmt.Fprintf(w, "  propose-create --name N (--body TEXT|--body-file P) [--desc D] [--triggers csv] [--tools csv] [--agent S] [--json]\n")
	fmt.Fprintf(w, "  propose-update <id> (--body TEXT|--body-file P) [--desc D] [--triggers csv] [--tools csv] [--agent S] [--json]\n")
	return 0
}

func cmdSkillWorkshopList(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Fprintf(stdout, "usage: %s skill workshop list [--json]\n", brand.CLI)
			return 0
		default:
			fmt.Fprintf(stderr, "%s skill workshop list: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := c.Call(ctx, controlplane.CmdSkillList, nil)
	if err != nil {
		fmt.Fprintf(stderr, "%s skill workshop list: %v\n", brand.CLI, err)
		return 1
	}
	proposals := workshopProposals(res["skills"])
	if asJSON {
		return jsonout.Write(stdout, map[string]any{"proposals": proposals, "count": len(proposals)})
	}
	if len(proposals) == 0 {
		fmt.Fprintln(stdout, "no pending workshop proposals")
		return 0
	}
	fmt.Fprintf(stdout, "%d workshop proposal(s):\n", len(proposals))
	for _, sk := range proposals {
		fmt.Fprintln(stdout, renderSkillLine(sk))
	}
	return 0
}

func cmdSkillWorkshopInspect(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	id := ""
	for _, a := range args {
		switch {
		case a == "--json":
			asJSON = true
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s skill workshop inspect <id> [--json]\n", brand.CLI)
			return 0
		case strings.HasPrefix(a, "-"):
			fmt.Fprintf(stderr, "%s skill workshop inspect: unexpected flag %q\n", brand.CLI, a)
			return 2
		case id == "":
			id = a
		default:
			fmt.Fprintf(stderr, "%s skill workshop inspect: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}
	if id == "" {
		fmt.Fprintf(stderr, "%s skill workshop inspect: id required\n", brand.CLI)
		return 2
	}
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sk, found, err := workshopFetchSkill(ctx, c, id)
	if err != nil {
		fmt.Fprintf(stderr, "%s skill workshop inspect: %v\n", brand.CLI, err)
		return 1
	}
	if !found {
		if asJSON {
			_ = jsonout.Write(stdout, map[string]any{"found": false, "id": id})
		} else {
			fmt.Fprintf(stderr, "%s skill workshop inspect: %s not found\n", brand.CLI, id)
		}
		return 3
	}
	history, err := c.Call(ctx, controlplane.CmdSkillHistory, map[string]any{"id": str(sk["id"])})
	if err != nil {
		fmt.Fprintf(stderr, "%s skill workshop inspect: history: %v\n", brand.CLI, err)
		return 1
	}
	events, _ := history["events"].([]any)
	if asJSON {
		scan := workshopScanSkill(sk)
		return jsonout.Write(stdout, map[string]any{
			"found": true, "skill": sk, "history": events, "history_count": len(events), "scan": scan,
		})
	}
	renderWorkshopInspect(stdout, sk, events)
	return 0
}

func cmdSkillWorkshopScan(args []string, stdout, stderr io.Writer) int {
	asJSON := false
	id := ""
	for _, a := range args {
		switch {
		case a == "--json":
			asJSON = true
		case a == "-h" || a == "--help":
			fmt.Fprintf(stdout, "usage: %s skill workshop scan <id> [--json]\n", brand.CLI)
			return 0
		case strings.HasPrefix(a, "-"):
			fmt.Fprintf(stderr, "%s skill workshop scan: unexpected flag %q\n", brand.CLI, a)
			return 2
		case id == "":
			id = a
		default:
			fmt.Fprintf(stderr, "%s skill workshop scan: unexpected arg %q\n", brand.CLI, a)
			return 2
		}
	}
	if id == "" {
		fmt.Fprintf(stderr, "%s skill workshop scan: id required\n", brand.CLI)
		return 2
	}
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	sk, found, err := workshopFetchSkill(ctx, c, id)
	if err != nil {
		fmt.Fprintf(stderr, "%s skill workshop scan: %v\n", brand.CLI, err)
		return 1
	}
	if !found {
		if asJSON {
			_ = jsonout.Write(stdout, map[string]any{"found": false, "id": id})
		} else {
			fmt.Fprintf(stderr, "%s skill workshop scan: %s not found\n", brand.CLI, id)
		}
		return 3
	}
	report := workshopScanSkill(sk)
	if asJSON {
		return jsonout.Write(stdout, map[string]any{"found": true, "id": str(sk["id"]), "scan": report})
	}
	renderWorkshopScan(stdout, report)
	return 0
}

func cmdSkillWorkshopCurate(args []string, stdout, stderr io.Writer) int {
	if workshopHelpRequested(args) {
		fmt.Fprintf(stdout, "usage: %s skill workshop curate [--idle-days N] [--execute] [--json]\n", brand.CLI)
		return 0
	}
	idleDays, execute, asJSON, ok := parseWorkshopCurateArgs(args, stdout, stderr)
	if !ok {
		return 2
	}
	c := dialpkg.New(stderr)
	if c == nil {
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	callArgs := map[string]any{}
	if idleDays > 0 {
		callArgs["idle_days"] = idleDays
	}
	res, err := c.Call(ctx, controlplane.CmdSkillHygiene, callArgs)
	if err != nil {
		fmt.Fprintf(stderr, "%s skill workshop curate: %v\n", brand.CLI, err)
		return 1
	}
	idle, _ := res["idle"].([]any)
	quarantined := make([]any, 0, len(idle))
	if execute {
		for _, raw := range idle {
			sk, _ := raw.(map[string]any)
			if sk == nil {
				continue
			}
			id := str(sk["id"])
			if id == "" {
				continue
			}
			reason := fmt.Sprintf("workshop curator: idle for %d+ days", intNumber(res["idle_days"]))
			if _, cerr := saveSkillStatusRollbackCheckpoint(ctx, c, "curate.quarantine", id, reason); cerr != nil {
				fmt.Fprintf(stderr, "%s skill workshop curate: checkpoint %s: %v\n", brand.CLI, id, cerr)
				return 1
			}
			qres, qerr := c.Call(ctx, controlplane.CmdSkillQuarantine, map[string]any{"id": id, "reason": reason})
			if qerr != nil {
				fmt.Fprintf(stderr, "%s skill workshop curate: quarantine %s: %v\n", brand.CLI, id, qerr)
				return 1
			}
			quarantined = append(quarantined, qres)
		}
	}
	out := map[string]any{
		"idle_days":   intNumber(res["idle_days"]),
		"candidates":  idle,
		"count":       len(idle),
		"execute":     execute,
		"quarantined": quarantined,
	}
	if asJSON {
		return jsonout.Write(stdout, out)
	}
	renderWorkshopCurate(stdout, out)
	return 0
}

func parseWorkshopCurateArgs(args []string, stdout, stderr io.Writer) (idleDays int, execute bool, asJSON bool, ok bool) {
	ok = true
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json":
			asJSON = true
		case a == "--execute":
			execute = true
		case a == "--idle-days":
			if i+1 >= len(args) {
				fmt.Fprintf(stderr, "%s skill workshop curate: --idle-days needs a value\n", brand.CLI)
				return 0, false, false, false
			}
			i++
			n, err := strconv.Atoi(args[i])
			if err != nil || n <= 0 {
				fmt.Fprintf(stderr, "%s skill workshop curate: bad --idle-days %q\n", brand.CLI, args[i])
				return 0, false, false, false
			}
			idleDays = n
		case strings.HasPrefix(a, "--idle-days="):
			n, err := strconv.Atoi(strings.TrimPrefix(a, "--idle-days="))
			if err != nil || n <= 0 {
				fmt.Fprintf(stderr, "%s skill workshop curate: bad --idle-days\n", brand.CLI)
				return 0, false, false, false
			}
			idleDays = n
		default:
			fmt.Fprintf(stderr, "%s skill workshop curate: unexpected arg %q\n", brand.CLI, a)
			return 0, false, false, false
		}
	}
	return idleDays, execute, asJSON, true
}

func renderWorkshopCurate(w io.Writer, out map[string]any) {
	idle, _ := out["candidates"].([]any)
	days := intNumber(out["idle_days"])
	execute, _ := out["execute"].(bool)
	if len(idle) == 0 {
		fmt.Fprintf(w, "curator: no active skills idle for %d+ days\n", days)
		return
	}
	action := "would quarantine"
	if execute {
		action = "quarantined"
	}
	fmt.Fprintf(w, "curator: %s %d active skill(s) idle for %d+ days\n", action, len(idle), days)
	for _, raw := range idle {
		sk, _ := raw.(map[string]any)
		if sk == nil {
			continue
		}
		uses := intNumber(sk["uses"])
		detail := "never used"
		if last := intNumber(sk["last_used_ms"]); last > 0 {
			detail = "last used " + time.UnixMilli(int64(last)).Format(time.RFC3339)
		}
		fmt.Fprintf(w, "  %s  %d use(s), %s\n", renderSkillLine(sk), uses, detail)
	}
	if !execute {
		fmt.Fprintf(w, "run with --execute to quarantine these candidates with a journaled curator reason\n")
	}
}

type workshopScanReport struct {
	Findings    []workshopScanFinding `json:"findings"`
	Count       int                   `json:"count"`
	MaxSeverity string                `json:"max_severity"`
}

type workshopScanFinding struct {
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Message  string `json:"message"`
	Evidence string `json:"evidence,omitempty"`
}

var workshopURLPattern = regexp.MustCompile(`https?://[^\s)'"]+`)

func workshopScanSkill(sk map[string]any) workshopScanReport {
	body := str(sk["body"])
	lower := strings.ToLower(body)
	report := workshopScanReport{MaxSeverity: "none"}
	seen := map[string]bool{}
	add := func(severity, code, message, evidence string) {
		if seen[code] {
			return
		}
		seen[code] = true
		report.Findings = append(report.Findings, workshopScanFinding{
			Severity: severity, Code: code, Message: message, Evidence: evidence,
		})
		if severityRank(severity) > severityRank(report.MaxSeverity) {
			report.MaxSeverity = severity
		}
	}

	for _, phrase := range []string{"ignore previous", "ignore all previous", "system prompt", "developer message", "jailbreak", "prompt injection"} {
		if strings.Contains(lower, phrase) {
			add("high", "prompt-injection", "contains language commonly used to override higher-priority instructions", phrase)
			break
		}
	}
	for _, tool := range workshopStringSlice(sk["tools_required"]) {
		t := strings.ToLower(tool)
		if t == "shell" || t == "code_exec" || t == "codeexec" || strings.Contains(t, "browser") {
			add("medium", "effectful-tool", "requires an effectful tool; review authority, sandbox, and egress policy", tool)
			break
		}
	}
	for _, phrase := range []string{"rm -rf", "sudo ", "chmod 777", "powershell -enc", "invoke-webrequest", "curl ", "wget ", "ssh "} {
		if strings.Contains(lower, phrase) {
			add("medium", "shell-network-effect", "mentions shell, network, or host-control operations", strings.TrimSpace(phrase))
			break
		}
	}
	for _, phrase := range []string{".env", "api_key", "apikey", "secret", "token", "password", "credential"} {
		if strings.Contains(lower, phrase) {
			add("high", "secret-handling", "references secrets or credential material; verify it does not read or disclose them", phrase)
			break
		}
	}
	for _, phrase := range []string{"exfiltrate", "pastebin", "webhook", "upload ", "send to http"} {
		if strings.Contains(lower, phrase) {
			add("high", "exfiltration-hint", "contains wording consistent with sending data to an external sink", strings.TrimSpace(phrase))
			break
		}
	}
	if strings.Contains(lower, "curl") && strings.Contains(lower, "| sh") {
		add("high", "curl-pipe-shell", "downloads and executes remote code in one step", "curl ... | sh")
	}
	if unpinnedInstall(lower) {
		add("medium", "unpinned-install", "installs dependencies without an obvious version pin", "install")
	}
	if urls := workshopURLPattern.FindAllString(body, -1); len(urls) > 0 {
		url := urls[0]
		severity := "medium"
		if strings.HasPrefix(strings.ToLower(url), "http://") {
			severity = "high"
		}
		add(severity, "external-url", "contains an external URL; review provenance and egress need", url)
	}
	for _, phrase := range []string{"../", "~/.ssh", "$home", "%userprofile%", "c:\\", "/etc/", "/var/", "/usr/bin"} {
		if strings.Contains(lower, phrase) {
			add("medium", "cross-workspace-path", "mentions paths that may escape the active workspace", phrase)
			break
		}
	}

	report.Count = len(report.Findings)
	return report
}

func unpinnedInstall(lower string) bool {
	for _, marker := range []string{"pip install ", "npm install ", "pnpm add ", "yarn add "} {
		idx := strings.Index(lower, marker)
		if idx < 0 {
			continue
		}
		line := lower[idx:]
		if nl := strings.IndexByte(line, '\n'); nl >= 0 {
			line = line[:nl]
		}
		if strings.Contains(line, "==") || strings.Contains(line, "@") || strings.Contains(line, "-r ") || strings.Contains(line, "package-lock") {
			continue
		}
		return true
	}
	return false
}

func severityRank(s string) int {
	switch s {
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	default:
		return 0
	}
}

func renderWorkshopScan(w io.Writer, report workshopScanReport) {
	if report.Count == 0 {
		fmt.Fprintln(w, "scanner: no deterministic findings")
		return
	}
	fmt.Fprintf(w, "scanner: %d finding(s), max=%s\n", report.Count, report.MaxSeverity)
	for _, f := range report.Findings {
		fmt.Fprintf(w, "  %-6s %-22s %s", strings.ToUpper(f.Severity), f.Code, f.Message)
		if f.Evidence != "" {
			fmt.Fprintf(w, " (evidence: %s)", f.Evidence)
		}
		fmt.Fprintln(w)
	}
}

func renderWorkshopInspect(w io.Writer, sk map[string]any, events []any) {
	fmt.Fprintf(w, "name:        %s\n", str(sk["name"]))
	fmt.Fprintf(w, "id:          %s\n", str(sk["id"]))
	fmt.Fprintf(w, "status:      %s\n", str(sk["status"]))
	fmt.Fprintf(w, "version:     %s\n", str(sk["version"]))
	agent := str(sk["agent"])
	if agent == "" {
		agent = "shared"
	}
	fmt.Fprintf(w, "agent:       %s\n", agent)
	if v := str(sk["description"]); v != "" {
		fmt.Fprintf(w, "description: %s\n", v)
	}
	if xs := workshopStringSlice(sk["triggers"]); len(xs) > 0 {
		fmt.Fprintf(w, "triggers:    %s\n", strings.Join(xs, ", "))
	}
	if xs := workshopStringSlice(sk["tools_required"]); len(xs) > 0 {
		fmt.Fprintf(w, "tools:       %s\n", strings.Join(xs, ", "))
	}
	if xs := workshopStringSlice(sk["resources"]); len(xs) > 0 {
		fmt.Fprintf(w, "resources:   %s\n", strings.Join(xs, ", "))
	}
	if xs := workshopStringSlice(sk["lineage"]); len(xs) > 0 {
		fmt.Fprintf(w, "lineage:     %s\n", strings.Join(xs, " -> "))
	}
	if ev := str(sk["source_event"]); ev != "" {
		fmt.Fprintf(w, "source:      %s why %s\n", brand.CLI, ev)
	}
	fmt.Fprintf(w, "history:     %d event(s)\n", len(events))
	renderWorkshopScan(w, workshopScanSkill(sk))
	if body := str(sk["body"]); body != "" {
		fmt.Fprintf(w, "body:\n  %s\n", strings.ReplaceAll(body, "\n", "\n  "))
	}
}

func workshopStringSlice(v any) []string {
	var out []string
	switch xs := v.(type) {
	case []any:
		for _, raw := range xs {
			if s := strings.TrimSpace(str(raw)); s != "" {
				out = append(out, s)
			}
		}
	case []string:
		for _, raw := range xs {
			if s := strings.TrimSpace(raw); s != "" {
				out = append(out, s)
			}
		}
	}
	return out
}

func stringsToAny(xs []string) []any {
	out := make([]any, 0, len(xs))
	for _, x := range xs {
		out = append(out, x)
	}
	return out
}

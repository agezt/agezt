// SPDX-License-Identifier: MIT

package main

// Skill workshop scan helpers + render helpers + string utilities:
// workshopScanReport + workshopScanFinding + workshopURLPattern +
// workshopScanSkill + unpinnedInstall + severityRank +
// renderWorkshopScan + renderWorkshopInspect + workshopStringSlice +
// stringsToAny. Carved out of skill_workshop.go during the Day 177
// god-file split so the main file can stay focused on the dispatcher
// + List/Inspect/Scan/Curate handlers + their direct parse/render
// helpers.
// Public API unchanged.

import (
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/agezt/agezt/internal/brand"
)
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

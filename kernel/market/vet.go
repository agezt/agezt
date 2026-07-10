// SPDX-License-Identifier: MIT

package market

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// Pack vetting — a static pre-install security review of pack content, in the
// spirit of ClawHub's post-ClawHavoc scanning and Hermes' install-time skill
// scan. Skill marketplaces are a supply-chain boundary: the ClawHavoc campaign
// planted 800+ malicious skills (infostealers, prompt-injection payloads,
// credential harvesters) in a public registry before scanning landed. VetPack
// looks for those shapes: injection-style instructions in skill bodies,
// download-and-execute pipelines, credential/wallet path reads, reverse shells,
// encoded payload blobs, and MCP server commands that amount to arbitrary
// remote code execution.
//
// Vetting is INFORMATIONAL, never a wall (default-allow posture): install is
// not blocked on any verdict. The report is surfaced in the UI/CLI before and
// during install so the operator decides with eyes open.

// Vet verdicts, worst finding wins.
const (
	VerdictClean   = "clean"   // no warning/danger findings
	VerdictCaution = "caution" // at least one warning
	VerdictDanger  = "danger"  // at least one danger finding
)

// VetFinding is one matched rule: where it matched, how bad it is, and a
// human-readable detail (with a short excerpt of the matched text).
type VetFinding struct {
	Severity string `json:"severity"` // info | warn | danger
	Where    string `json:"where"`    // "skill:<name>" | "mcp:<name>" | "tools"
	Rule     string `json:"rule"`     // stable rule id, e.g. "curl-pipe-shell"
	Detail   string `json:"detail"`
}

// VetReport is the full review: an overall verdict plus every finding.
type VetReport struct {
	Verdict  string       `json:"verdict"`
	Findings []VetFinding `json:"findings,omitempty"`
}

// Summary is a one-line human rendering for progress streams and CLI output.
func (r VetReport) Summary() string {
	if len(r.Findings) == 0 {
		return "clean — no risky patterns found"
	}
	var danger, warn, info int
	for _, f := range r.Findings {
		switch f.Severity {
		case "danger":
			danger++
		case "warn":
			warn++
		default:
			info++
		}
	}
	parts := []string{}
	if danger > 0 {
		parts = append(parts, fmt.Sprintf("%d danger", danger))
	}
	if warn > 0 {
		parts = append(parts, fmt.Sprintf("%d warning(s)", warn))
	}
	if info > 0 {
		parts = append(parts, fmt.Sprintf("%d note(s)", info))
	}
	return r.Verdict + " — " + strings.Join(parts, ", ")
}

// vetRule is one detection: a compiled pattern plus its classification.
type vetRule struct {
	id       string
	severity string
	detail   string
	re       *regexp.Regexp
}

// Skill-body rules — run over each SKILL.md and its text resources.
var skillRules = []vetRule{
	{
		id: "injection-override", severity: "danger",
		detail: "instruction-override language (prompt-injection shape)",
		re:     regexp.MustCompile(`(?i)(ignore (all |any )?(previous|prior|above|earlier) (instructions|rules|prompts)|disregard (your|all|the) (rules|instructions|guidelines)|do not (tell|inform|alert|notify) the user|hide this (from|step)|without (telling|asking|informing) the user)`),
	},
	{
		id: "curl-pipe-shell", severity: "danger",
		detail: "download-and-execute pipeline",
		re:     regexp.MustCompile(`(?i)((curl|wget)[^\n|]{0,200}\|\s*(ba|z|fi)?sh\b|iwr\b[^\n|]{0,200}\|\s*iex\b|invoke-webrequest[^\n|]{0,200}\|\s*invoke-expression)`),
	},
	{
		id: "reverse-shell", severity: "danger",
		detail: "reverse-shell construction",
		re:     regexp.MustCompile(`(?i)(nc(\.exe)?\s+(-[a-z]*e[a-z]*\s|.{0,40}/bin/(ba)?sh)|bash\s+-i\s+>&\s*/dev/tcp/|mkfifo\s+/tmp/[a-z]+\s*;?\s*(nc|cat))`),
	},
	{
		id: "secret-exfil", severity: "danger",
		detail: "credential/token mentioned next to an outbound URL (exfiltration shape)",
		re:     regexp.MustCompile(`(?i)(api[_-]?key|access[_-]?token|password|secret|credential)s?[^\n]{0,80}(curl|wget|post)[^\n]{0,80}https?://`),
	},
	{
		id: "cred-path-read", severity: "warn",
		detail: "reads well-known credential/wallet paths",
		re:     regexp.MustCompile(`(?i)(~/\.ssh|id_rsa|id_ed25519|\.aws/credentials|\.npmrc|\.netrc|wallet\.dat|Login Data|/Cookies\b|\.kube/config)`),
	},
	{
		id: "encoded-blob", severity: "warn",
		detail: "large base64/encoded blob (possible concealed payload)",
		re:     regexp.MustCompile(`([A-Za-z0-9+/]{240,}={0,2}|(?i)powershell[^\n]{0,40}-enc(odedcommand)?\s)`),
	},
	{
		id: "env-dump", severity: "warn",
		detail: "environment dump combined with network access",
		re:     regexp.MustCompile(`(?i)(printenv|process\.env|os\.environ)[^\n]{0,120}(curl|wget|fetch\(|https?://)`),
	},
	{
		id: "destructive-command", severity: "warn",
		detail: "destructive filesystem command",
		re:     regexp.MustCompile(`(?i)(rm\s+-rf\s+[/~]|del\s+/s\s+/q\s+[a-z]:|format\s+[a-z]:)`),
	},
}

// MCP-server rules — run over the joined command line and env var names.
var mcpCommandRules = []vetRule{
	{
		id: "mcp-remote-exec", severity: "danger",
		detail: "MCP command downloads and executes remote code",
		re:     regexp.MustCompile(`(?i)((curl|wget)[^|]{0,200}\|\s*(ba)?sh\b|iwr\b[^|]{0,200}\|\s*iex\b)`),
	},
	{
		id: "mcp-shell-host", severity: "warn",
		detail: "MCP server is a raw shell invocation (arbitrary command surface)",
		re:     regexp.MustCompile(`(?i)^(bash|sh|zsh|cmd(\.exe)?|powershell(\.exe)?|pwsh)\s+(-c|/c)\b`),
	},
}

// knownRunners are commands we recognize as ordinary MCP launchers; anything
// else gets an informational "unrecognized runner" note.
var knownRunners = map[string]bool{
	"npx": true, "node": true, "bunx": true, "deno": true,
	"uvx": true, "uv": true, "python": true, "python3": true,
	"docker": true, "podman": true, "go": true, "java": true, "dotnet": true,
}

// secretEnvRe flags env var names that carry credentials — informational: the
// operator should know a pack wants keys before installing it.
var secretEnvRe = regexp.MustCompile(`(?i)(KEY|TOKEN|SECRET|PASSWORD|CREDENTIAL)`)

// riskyTools are tool requirements that are red flags in a capability pack.
var riskyTools = map[string]string{
	"mimikatz": "credential-dumping tool",
	"psexec":   "lateral-movement tool",
	"lazagne":  "credential-recovery tool",
}

// VetPack statically reviews a pack's content and returns a report. Purely
// informational — callers must not block installs on it (default-allow).
func VetPack(p Pack) VetReport {
	var findings []VetFinding

	for _, ps := range p.Skills {
		where := "skill"
		if md, err := SkillSummary(ps); err == nil {
			name, _, _ := strings.Cut(md, " — ")
			where = "skill:" + name
		}
		findings = append(findings, scanText(ps.SkillMD, where, skillRules)...)
		for rel, body := range ps.Resources {
			if looksBinary(body) {
				continue
			}
			findings = append(findings, scanText(string(body), where+"/"+rel, skillRules)...)
		}
	}

	for _, srv := range p.MCPServers {
		where := "mcp:" + srv.Name
		cmdline := strings.TrimSpace(srv.Command + " " + strings.Join(srv.Args, " "))
		findings = append(findings, scanText(cmdline, where, mcpCommandRules)...)
		if srv.Command != "" && !knownRunners[strings.ToLower(strings.TrimSuffix(srv.Command, ".exe"))] {
			if !strings.ContainsAny(srv.Command, "/\\") { // path-launched binaries are their own review
				findings = append(findings, VetFinding{
					Severity: "info", Where: where, Rule: "mcp-nonstd-runner",
					Detail: fmt.Sprintf("unrecognized launcher %q — review before install", srv.Command),
				})
			}
		}
		for k := range srv.Env {
			if secretEnvRe.MatchString(k) {
				findings = append(findings, VetFinding{
					Severity: "info", Where: where, Rule: "mcp-secret-env",
					Detail: fmt.Sprintf("requests credential env %q — it will see that secret", k),
				})
			}
		}
	}

	for _, t := range p.ToolRequirements {
		if why, bad := riskyTools[strings.ToLower(t)]; bad {
			findings = append(findings, VetFinding{
				Severity: "danger", Where: "tools", Rule: "tool-req-risky",
				Detail: fmt.Sprintf("requires %s (%s)", t, why),
			})
		}
	}

	return VetReport{Verdict: verdictFor(findings), Findings: findings}
}

// scanText applies each rule once per location (first match wins per rule) so a
// repeated pattern doesn't flood the report.
func scanText(text, where string, rules []vetRule) []VetFinding {
	var out []VetFinding
	for _, r := range rules {
		loc := r.re.FindStringIndex(text)
		if loc == nil {
			continue
		}
		out = append(out, VetFinding{
			Severity: r.severity,
			Where:    where,
			Rule:     r.id,
			Detail:   r.detail + ": “" + excerpt(text, loc[0], loc[1]) + "”",
		})
	}
	return out
}

// excerpt trims a matched span to a short, single-line quote for the report.
func excerpt(text string, start, end int) string {
	const max = 90
	if end-start > max {
		end = start + max
	}
	s := strings.Join(strings.Fields(text[start:end]), " ")
	if len(s) > max {
		s = s[:max] + "…"
	}
	return s
}

// looksBinary reports whether resource bytes are likely non-text (skip scan).
func looksBinary(b []byte) bool {
	return slices.Contains(b[:min(len(b), 512)], 0)
}

func verdictFor(findings []VetFinding) string {
	verdict := VerdictClean
	for _, f := range findings {
		switch f.Severity {
		case "danger":
			return VerdictDanger
		case "warn":
			verdict = VerdictCaution
		}
	}
	return verdict
}

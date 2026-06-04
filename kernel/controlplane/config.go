// SPDX-License-Identifier: MIT

package controlplane

// Daemon config snapshot. The "what is this daemon actually running
// with?" command. Today the answer is scattered across the kernel's
// startup log, ad-hoc grepping for AGEZT_*, and inferring paths
// from the OS user-home — none of which are scriptable. CmdConfig
// gives operators (and CI smoke tests) one round-trip that returns
// the resolved view.
//
// Privacy: env-var VALUES are never returned. The handler reports
// only PRESENCE — `{ "AGEZT_VAULT_PASSPHRASE": true }` when the
// var is set, omitted when unset. Same rule for the system prompt:
// `system_prompt_set: bool` reports whether one is configured but
// never echoes its content (could contain proprietary instructions).

import (
	"net"
	"os"
	"path/filepath"
)

// configEnvVars is the canonical set of AGEZT_* env vars the daemon
// reads at startup. Surface PRESENCE only; the values can contain
// secrets (passphrases) or noisy details (catalog URLs that include
// auth in path). Sorted so the response key order is stable for
// snapshot tests.
//
// Source-of-truth: every Getenv("AGEZT_...") in cmd/agezt/ (plus the
// daemon's kernel/plugin reads). New vars MUST be added here when
// introduced — TestConfigEnvVars_CoversCmdAgeztReads (M127) enforces
// this by scanning cmd/agezt and failing if any read var is absent,
// so the inventory can no longer silently rot.
var configEnvVars = []string{
	"AGEZT_ACP_AGENT_CMD",
	"AGEZT_ANOMALY_MAX_TOOLCALLS",
	"AGEZT_ANOMALY_WINDOW",
	"AGEZT_API_ADDR",
	"AGEZT_APPROVAL_MODE",
	"AGEZT_APPROVAL_TIMEOUT",
	"AGEZT_ARTIFACT_THRESHOLD",
	"AGEZT_AWS_ASSUME_ROLE_ARN",
	"AGEZT_AWS_ASSUME_ROLE_DURATION_SECONDS",
	"AGEZT_AWS_ASSUME_ROLE_EXTERNAL_ID",
	"AGEZT_AWS_ASSUME_ROLE_SESSION_NAME",
	"AGEZT_AWS_SSO_PROFILE",
	"AGEZT_BROWSER_ALLOWED_HOSTS",
	"AGEZT_BROWSER_ALLOW_ALL",
	"AGEZT_BROWSER_ALLOW_LOOPBACK",
	"AGEZT_BROWSER_ALLOW_PRIVATE",
	"AGEZT_BROWSER_COOKIES",
	"AGEZT_CANCEL_ON_DISCONNECT",
	"AGEZT_CATALOG_URL",
	"AGEZT_CHANNEL_HISTORY",
	"AGEZT_CODING_CMD",
	"AGEZT_DEMO_CACHED",
	"AGEZT_DEMO_DELEGATE",
	"AGEZT_DEMO_ECHO",
	"AGEZT_DEMO_FAIL_PRIMARY",
	"AGEZT_DEMO_FILE_EDIT",
	"AGEZT_DEMO_LOOP",
	"AGEZT_DEMO_NOTIFY",
	"AGEZT_DEMO_SSRF",
	"AGEZT_DEMO_VISION",
	"AGEZT_DISCORD_ADDR",
	"AGEZT_DISCORD_API_BASE",
	"AGEZT_DISCORD_APP_ID",
	"AGEZT_DISCORD_CHANNELS",
	"AGEZT_DISCORD_PUBLIC_KEY",
	"AGEZT_DISCORD_TOKEN",
	"AGEZT_DRAIN_TIMEOUT",
	"AGEZT_EDICT_DENY",
	"AGEZT_EDICT_DURABLE",
	"AGEZT_EMAIL_FROM",
	"AGEZT_EMAIL_PASSWORD",
	"AGEZT_EMAIL_RECIPIENTS",
	"AGEZT_EMAIL_SMTP_ADDR",
	"AGEZT_EMAIL_USERNAME",
	"AGEZT_FORCE_START",
	"AGEZT_FORGE",
	"AGEZT_HOME",
	"AGEZT_HTTP_ALLOWED_HOSTS",
	"AGEZT_HTTP_ALLOW_ALL",
	"AGEZT_HTTP_ALLOW_LOOPBACK",
	"AGEZT_HTTP_ALLOW_PRIVATE",
	"AGEZT_MEMORY",
	"AGEZT_MESH_MAX_HOPS",
	"AGEZT_MODEL",
	"AGEZT_MODEL_DOWNROUTE",
	"AGEZT_MODEL_DOWNROUTE_CROSS",
	"AGEZT_MODEL_STRICT",
	"AGEZT_MULTITENANT",
	"AGEZT_OLLAMA_ENDPOINT",
	"AGEZT_PEERS",
	"AGEZT_PLUGINS",
	"AGEZT_PLUGIN_PINS",
	"AGEZT_PLUGIN_TOOLS",
	"AGEZT_PRICING_STRICT",
	"AGEZT_PROVIDER",
	"AGEZT_PULSE",
	"AGEZT_PULSE_CADENCE",
	"AGEZT_PULSE_DIAL",
	"AGEZT_PULSE_DISK",
	"AGEZT_PULSE_LLM",
	"AGEZT_PULSE_PROBE",
	"AGEZT_PULSE_QUIET_HOURS",
	"AGEZT_RATE_PER_MIN",
	"AGEZT_REDACT",
	"AGEZT_REDACT_EXTRA",
	"AGEZT_REFLECT_EVERY",
	"AGEZT_REST_ADDR",
	"AGEZT_RUN_TIMEOUT",
	"AGEZT_SCHEDULE",
	"AGEZT_SCHEDULE_NOTIFY",
	"AGEZT_SKILLS",
	"AGEZT_SKILL_AUTOQUARANTINE",
	"AGEZT_SLACK_ADDR",
	"AGEZT_SLACK_API_BASE",
	"AGEZT_SLACK_CHANNELS",
	"AGEZT_SLACK_SIGNING_SECRET",
	"AGEZT_SLACK_TOKEN",
	"AGEZT_SUBAGENT",
	"AGEZT_SUBAGENT_DEPTH",
	"AGEZT_SUBAGENT_FANOUT",
	"AGEZT_SUBAGENT_SPEND_CAP",
	"AGEZT_SYSTEM_PROMPT",
	"AGEZT_TASK_BUDGETS",
	"AGEZT_TASK_MODEL_OVERRIDES",
	"AGEZT_TASK_ROUTES",
	"AGEZT_TASK_ROUTE_REQUIRES",
	"AGEZT_TELEGRAM_API_BASE",
	"AGEZT_TELEGRAM_CHAT_ID",
	"AGEZT_TELEGRAM_TOKEN",
	"AGEZT_TENANT_DAILY_CEILING",
	"AGEZT_TENANT_PEERS",
	"AGEZT_TENANT_RATE_PER_MIN",
	"AGEZT_TOKEN",
	"AGEZT_TOOL_TIMEOUT",
	"AGEZT_VAULT_PASSPHRASE",
	"AGEZT_VAULT_PASSPHRASE_NEW",
	"AGEZT_WEBHOOKS",
	"AGEZT_WEBHOOK_ADDR",
	"AGEZT_WEBHOOK_CHANNELS",
	"AGEZT_WEBHOOK_OUTBOUND_URL",
	"AGEZT_WEBHOOK_PATH",
	"AGEZT_WEBHOOK_SECRET",
	"AGEZT_WEB_ADDR",
	"AGEZT_WORKSPACE",
	"AGEZT_WORLDMODEL",
}

func (s *Server) handleConfig(conn net.Conn, req Request) {
	base := s.k.BaseDir()
	paths := map[string]any{
		"base":    base,
		"journal": filepath.Join(base, "journal"),
		"state":   filepath.Join(base, "state"),
		"runtime": filepath.Join(base, "runtime"),
		"catalog": filepath.Join(base, "catalog"),
		"vault":   filepath.Join(base, "vault.json"),
	}

	env := map[string]any{}
	for _, name := range configEnvVars {
		if _, ok := os.LookupEnv(name); ok {
			env[name] = true
		}
	}

	result := map[string]any{
		"paths":             paths,
		"model":             s.k.Model(),
		"system_prompt_set": s.k.System() != "",
		"tool_count":        len(s.k.Tools()),
		"plugin_count":      len(s.k.Plugins()),
		"ask_policy":        askPolicyLabel(s.k.Edict().AskPolicy()),
		"env":               env,
	}

	// Effective routing tables (M108): surface what AGEZT_TASK_ROUTES /
	// _ROUTE_REQUIRES / _MODEL_OVERRIDES actually parsed to, so an operator can
	// confirm a rule loaded rather than reading the boot log. Only present when
	// the provider is the governor (the usual case) and a table is non-empty.
	if gov, ok := s.k.Provider().(interface {
		TaskRoutesView() map[string][]string
		TaskRouteRequiresView() map[string][]string
		TaskModelOverridesView() map[string]string
	}); ok {
		routing := map[string]any{}
		if r := gov.TaskRoutesView(); len(r) > 0 {
			routing["routes"] = stringSliceMapToAny(r)
		}
		if r := gov.TaskRouteRequiresView(); len(r) > 0 {
			routing["requires"] = stringSliceMapToAny(r)
		}
		if o := gov.TaskModelOverridesView(); len(o) > 0 {
			m := make(map[string]any, len(o))
			for k, v := range o {
				m[k] = v
			}
			routing["model_overrides"] = m
		}
		if len(routing) > 0 {
			result["routing"] = routing
		}
	}

	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: result})
}

func stringSliceMapToAny(in map[string][]string) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		arr := make([]any, len(v))
		for i, s := range v {
			arr[i] = s
		}
		out[k] = arr
	}
	return out
}

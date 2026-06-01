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
// Source-of-truth: every Getenv("AGEZT_...") in cmd/agezt/. New
// vars MUST be added here when introduced; the PHASE-M2 report
// references this list as the operator-facing inventory.
var configEnvVars = []string{
	"AGEZT_APPROVAL_MODE",
	"AGEZT_APPROVAL_TIMEOUT",
	"AGEZT_AWS_ASSUME_ROLE_ARN",
	"AGEZT_AWS_SSO_PROFILE",
	"AGEZT_BROWSER_ALLOWED_HOSTS",
	"AGEZT_BROWSER_ALLOW_ALL",
	"AGEZT_BROWSER_COOKIES",
	"AGEZT_CATALOG_URL",
	"AGEZT_DEMO_FAIL_PRIMARY",
	"AGEZT_HOME",
	"AGEZT_HTTP_ALLOWED_HOSTS",
	"AGEZT_HTTP_ALLOW_ALL",
	"AGEZT_MODEL",
	"AGEZT_PLUGINS",
	"AGEZT_PLUGIN_PINS",
	"AGEZT_PLUGIN_TOOLS",
	"AGEZT_PROVIDER",
	"AGEZT_SYSTEM_PROMPT",
	"AGEZT_TASK_BUDGETS",
	"AGEZT_TASK_MODEL_OVERRIDES",
	"AGEZT_TASK_ROUTES",
	"AGEZT_TASK_ROUTE_REQUIRES",
	"AGEZT_VAULT_PASSPHRASE",
	"AGEZT_VAULT_PASSPHRASE_NEW",
	"AGEZT_WORKSPACE",
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

	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			"paths":              paths,
			"model":              s.k.Model(),
			"system_prompt_set":  s.k.System() != "",
			"tool_count":         len(s.k.Tools()),
			"plugin_count":       len(s.k.Plugins()),
			"ask_policy":         askPolicyLabel(s.k.Edict().AskPolicy()),
			"env":                env,
		},
	})
}

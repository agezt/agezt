// SPDX-License-Identifier: MIT

// Config handler: handleConfig.
// Code extracted from config.go during the Day-74 god-file split. Public API unchanged.
package controlplane


import (
	"net"
	"os"
	"path/filepath"
)



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
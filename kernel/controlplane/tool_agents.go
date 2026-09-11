// SPDX-License-Identifier: MIT

// Agent permissions/capabilities handlers: handleAgentPermissions + handleAgentCapabilities + decodeAgentCapabilityPatch + decodeControlplaneArg.
// Code extracted from tool.go during the Day-59 god-file split. Public API unchanged.
package controlplane


import (
	"github.com/agezt/agezt/kernel/roster"
	"net"
	"strings"
)


func (s *Server) handleAgentPermissions(conn net.Conn, req Request) {
	ref, err := requiredArgString(req.Args, "ref")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	p, ok := s.k.Roster().Get(ref)
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown agent: " + ref})
		return
	}
	rows := s.agentPermissionRows(p)
	configRows := s.agentConfigPermissionRows(p)
	allowed := 0
	for _, row := range rows {
		if v, _ := row["allowed"].(bool); v {
			allowed++
		}
	}
	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			"slug":           p.Slug,
			"trust_ceiling":  strings.TrimSpace(p.TrustCeiling),
			"tool_allow":     append([]string(nil), p.ToolAllow...),
			"tool_deny":      append([]string(nil), p.ToolDeny...),
			"permissions":    rows,
			"config_entries": configRows,
			"wake_access":    agentWakeAccessView(p),
			"governance":     agentGovernanceView(p, rows, configRows),
			"count":          len(rows),
			"allowed_count":  allowed,
		},
	})
}

func (s *Server) handleAgentCapabilities(conn net.Conn, req Request) {
	ref, err := requiredArgString(req.Args, "ref")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	current, ok := s.k.Roster().Get(ref)
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown agent: " + ref})
		return
	}
	patch, touched, err := decodeAgentCapabilityPatch(req.Args)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	if !touched {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args capability field required"})
		return
	}
	candidate := current
	applyAgentCapabilityPatch(&candidate, patch)
	if err := s.validateAgentHierarchyRefs(candidate); err != nil {
		s.fail(conn, req, err)
		return
	}
	updated, found, err := s.k.UpdateProfile(ref, func(dst *roster.Profile) {
		applyAgentCapabilityPatch(dst, patch)
	})
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	if !found {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown agent: " + ref})
		return
	}
	rows := s.agentPermissionRows(updated)
	configRows := s.agentConfigPermissionRows(updated)
	allowed := 0
	for _, row := range rows {
		if v, _ := row["allowed"].(bool); v {
			allowed++
		}
	}
	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			"profile":        profileView(updated),
			"slug":           updated.Slug,
			"trust_ceiling":  strings.TrimSpace(updated.TrustCeiling),
			"tool_allow":     append([]string(nil), updated.ToolAllow...),
			"tool_deny":      append([]string(nil), updated.ToolDeny...),
			"permissions":    rows,
			"config_entries": configRows,
			"wake_access":    agentWakeAccessView(updated),
			"governance":     agentGovernanceView(updated, rows, configRows),
			"count":          len(rows),
			"allowed_count":  allowed,
		},
	})
}

type agentCapabilityPatch struct {
	TrustCeiling    *string
	ToolAllow       *[]string
	ToolDeny        *[]string
	NoisePolicy     *roster.NoisePolicy
	ConfigOverrides *map[string]string
	MemoryScope     *string
	Workdir         *string
	MaxCostMc       *int64
	MaxDailyMc      *int64
}

func decodeAgentCapabilityPatch(args map[string]any) (agentCapabilityPatch, bool, error) {
	var patch agentCapabilityPatch
	touched := false
	if raw, ok := args["trust_ceiling"]; ok {
		var v string
		if err := decodeControlplaneArg(raw, &v); err != nil {
			return patch, false, err
		}
		patch.TrustCeiling = &v
		touched = true
	}
	if raw, ok := args["tool_allow"]; ok {
		var v []string
		if err := decodeControlplaneArg(raw, &v); err != nil {
			return patch, false, err
		}
		patch.ToolAllow = &v
		touched = true
	}
	if raw, ok := args["tool_deny"]; ok {
		var v []string
		if err := decodeControlplaneArg(raw, &v); err != nil {
			return patch, false, err
		}
		patch.ToolDeny = &v
		touched = true
	}
	if raw, ok := args["noise_policy"]; ok {
		var v roster.NoisePolicy
		if err := decodeControlplaneArg(raw, &v); err != nil {
			return patch, false, err
		}
		patch.NoisePolicy = &v
		touched = true
	}
	if raw, ok := args["config_overrides"]; ok {
		var v map[string]string
		if err := decodeControlplaneArg(raw, &v); err != nil {
			return patch, false, err
		}
		patch.ConfigOverrides = &v
		touched = true
	}
	if raw, ok := args["memory_scope"]; ok {
		var v string
		if err := decodeControlplaneArg(raw, &v); err != nil {
			return patch, false, err
		}
		patch.MemoryScope = &v
		touched = true
	}
	if raw, ok := args["workdir"]; ok {
		var v string
		if err := decodeControlplaneArg(raw, &v); err != nil {
			return patch, false, err
		}
		patch.Workdir = &v
		touched = true
	}
	if raw, ok := args["max_cost_mc"]; ok {
		var v int64
		if err := decodeControlplaneArg(raw, &v); err != nil {
			return patch, false, err
		}
		patch.MaxCostMc = &v
		touched = true
	}
	if raw, ok := args["max_daily_mc"]; ok {
		var v int64
		if err := decodeControlplaneArg(raw, &v); err != nil {
			return patch, false, err
		}
		patch.MaxDailyMc = &v
		touched = true
	}
	return patch, touched, nil
}

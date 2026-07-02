// SPDX-License-Identifier: MIT

package controlplane

import (
	"net"
	"sort"
	"strings"

	"github.com/agezt/agezt/kernel/agent"
	"github.com/agezt/agezt/kernel/executionprofile"
)

func (s *Server) handleExecutionProfiles(conn net.Conn, req Request) {
	k, err := s.kernelFor(tenantOf(req))
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	inv := executionprofile.Build(executionprofile.Options{
		Tools:   toolNames(k.Tools()),
		Warden:  k.Warden(),
		SSH:     executionprofile.SSHConfigFromEnv(),
		K8s:     executionprofile.K8sConfigFromEnv(),
		Modal:   executionprofile.ModalConfigFromEnv(),
		Daytona: executionprofile.DaytonaConfigFromEnv(),
	})
	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			"host_os":         inv.HostOS,
			"host_arch":       inv.HostArch,
			"profiles":        profileRows(inv.Profiles),
			"count":           inv.Count,
			"routed_count":    inv.RoutedCount,
			"supported_count": inv.SupportedCount,
			"degraded_count":  inv.DegradedCount,
		},
	})
}

func (s *Server) handleExecutionProfileShow(conn net.Conn, req Request) {
	id, _ := req.Args["id"].(string)
	id = strings.TrimSpace(id)
	if id == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.id required"})
		return
	}
	k, err := s.kernelFor(tenantOf(req))
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	inv := executionprofile.Build(executionprofile.Options{
		Tools:   toolNames(k.Tools()),
		Warden:  k.Warden(),
		SSH:     executionprofile.SSHConfigFromEnv(),
		K8s:     executionprofile.K8sConfigFromEnv(),
		Modal:   executionprofile.ModalConfigFromEnv(),
		Daytona: executionprofile.DaytonaConfigFromEnv(),
	})
	p, ok := inv.Find(id)
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown execution profile: " + id})
		return
	}
	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			"profile":   profileRow(p),
			"host_os":   inv.HostOS,
			"host_arch": inv.HostArch,
		},
	})
}

func (s *Server) handleExecutionProfileCheck(conn net.Conn, req Request) {
	k, err := s.kernelFor(tenantOf(req))
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	inv := executionprofile.Build(executionprofile.Options{
		Tools:   toolNames(k.Tools()),
		Warden:  k.Warden(),
		SSH:     executionprofile.SSHConfigFromEnv(),
		K8s:     executionprofile.K8sConfigFromEnv(),
		Modal:   executionprofile.ModalConfigFromEnv(),
		Daytona: executionprofile.DaytonaConfigFromEnv(),
	})
	report := executionprofile.Diagnose(inv, executionprofile.HealthOptions{Policy: executionprofile.PolicyFromEnv()})
	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			"host_os":               report.HostOS,
			"host_arch":             report.HostArch,
			"checks":                healthCheckRows(report.Checks),
			"count":                 report.Count,
			"ok_count":              report.OKCount,
			"warning_count":         report.WarningCount,
			"fail_count":            report.FailCount,
			"routable_run_profiles": append([]string(nil), report.RoutableRunProfiles...),
		},
	})
}

func toolNames(tools map[string]agent.Tool) []string {
	names := make([]string, 0, len(tools))
	for name := range tools {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func healthCheckRows(checks []executionprofile.HealthCheck) []map[string]any {
	out := make([]map[string]any, 0, len(checks))
	for _, c := range checks {
		out = append(out, map[string]any{
			"id":                c.ID,
			"profile_id":        c.ProfileID,
			"status":            string(c.Status),
			"title":             c.Title,
			"detail":            c.Detail,
			"next":              c.Next,
			"routed":            c.Routed,
			"degraded":          c.Degraded,
			"backend_available": c.BackendAvailable,
			"backend":           c.Backend,
		})
	}
	return out
}

func profileRows(profiles []executionprofile.Profile) []map[string]any {
	out := make([]map[string]any, 0, len(profiles))
	for _, p := range profiles {
		out = append(out, profileRow(p))
	}
	return out
}

func profileRow(p executionprofile.Profile) map[string]any {
	row := map[string]any{
		"id":                  p.ID,
		"name":                p.Name,
		"summary":             p.Summary,
		"status":              string(p.Status),
		"routed":              p.Routed,
		"requested_isolation": p.RequestedIsolation,
		"effective_isolation": p.EffectiveIsolation,
		"degraded":            p.Degraded,
		"degrade_reason":      p.DegradeReason,
		"tools":               append([]string(nil), p.Tools...),
		"backends":            append([]string(nil), p.Backends...),
		"filesystem":          p.FileSystem,
		"network":             p.Network,
		"environment":         p.Environment,
		"secrets":             p.Secrets,
		"limits":              append([]string(nil), p.Limits...),
		"browser_access":      p.BrowserAccess,
		"cleanup":             p.Cleanup,
		"policy_capability":   p.PolicyCapability,
		"notes":               append([]string(nil), p.Notes...),
	}
	if p.SecretPolicy != nil {
		row["secret_policy"] = secretPolicyRow(*p.SecretPolicy)
	}
	return row
}

func secretPolicyRow(p executionprofile.SecretPolicy) map[string]any {
	return map[string]any{
		"mode":               p.Mode,
		"scope":              p.Scope,
		"values_forwarded":   p.ValuesForwarded,
		"metadata_forwarded": p.MetadataForwarded,
		"valid":              p.Valid,
		"detail":             p.Detail,
	}
}

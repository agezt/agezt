// SPDX-License-Identifier: MIT

// Control-plane skill view helpers (isSkillKind + skillView + registerSkillCommands).
// Code extracted from skill.go during the Day-88 god-file split.
// Public API unchanged.
package controlplane


import (
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/skill"
)

func isSkillKind(k event.Kind) bool {
	switch k {
	case event.KindSkillCreated, event.KindSkillPromoted, event.KindSkillQuarantined,
		event.KindSkillReverted, event.KindSkillRestored, event.KindSkillActivated:
		return true
	}
	return false
}

// skillView renders a skill.Skill as a stable JSON object for the wire.
func skillView(sk skill.Skill) map[string]any {
	v := map[string]any{
		"id":           sk.ID,
		"name":         sk.Name,
		"description":  sk.Description,
		"status":       string(sk.Status),
		"version":      sk.Version,
		"agent":        sk.Agent,
		"created_ms":   sk.CreatedMS,
		"last_seen_ms": sk.LastSeenMS,
		"metrics": map[string]any{
			"uses": sk.Metrics.Uses, "successes": sk.Metrics.Successes,
			"failures": sk.Metrics.Failures, "last_used_ms": sk.Metrics.LastUsedMS,
			"shadow_evals": sk.Metrics.ShadowEvals, "shadow_wins": sk.Metrics.ShadowWins,
		},
	}
	if len(sk.Triggers) > 0 {
		v["triggers"] = sk.Triggers
	}
	if len(sk.ToolsRequired) > 0 {
		v["tools_required"] = sk.ToolsRequired
	}
	if len(sk.Resources) > 0 {
		v["resources"] = sk.Resources
	}
	if len(sk.Lineage) > 0 {
		v["lineage"] = sk.Lineage
	}
	if sk.Body != "" {
		v["body"] = sk.Body
	}
	if sk.SourceEvent != "" {
		v["source_event"] = sk.SourceEvent
	}
	return v
}

// registerSkillCommands registers this file's protocol commands into the dispatch registry (phase 2.3).
func registerSkillCommands() {
	register(
		commandSpec{Cmd: CmdSkillList, Handler: func(dc *DispatchCtx) { dc.S.handleSkillList(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdSkillGet, Handler: func(dc *DispatchCtx) { dc.S.handleSkillGet(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdSkillHistory, Handler: func(dc *DispatchCtx) { dc.S.handleSkillHistory(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdSkillPromote, Handler: func(dc *DispatchCtx) { dc.S.handleSkillPromote(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdSkillQuarantine, Handler: func(dc *DispatchCtx) { dc.S.handleSkillQuarantine(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdSkillArchive, Handler: func(dc *DispatchCtx) { dc.S.handleSkillArchive(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdSkillRevert, Handler: func(dc *DispatchCtx) { dc.S.handleSkillRevert(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdSkillRestore, Handler: func(dc *DispatchCtx) { dc.S.handleSkillRestore(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdSkillShare, Handler: func(dc *DispatchCtx) { dc.S.handleSkillShare(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdSkillReassign, Handler: func(dc *DispatchCtx) { dc.S.handleSkillReassign(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdSkillImport, Handler: func(dc *DispatchCtx) { dc.S.handleSkillImport(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdSkillHygiene, Handler: func(dc *DispatchCtx) { dc.S.handleSkillHygiene(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdSkillFiles, Handler: func(dc *DispatchCtx) { dc.S.handleSkillFiles(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdSkillReadFile, Handler: func(dc *DispatchCtx) { dc.S.handleSkillReadFile(dc.Conn, dc.Req) }},
	)
}

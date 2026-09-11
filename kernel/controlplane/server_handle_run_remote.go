// SPDX-License-Identifier: MIT

// Remote execution profile helpers + registerCoreCommands: publishRemoteExecutionProfileRunEvent + remoteExecutionProfileAnswerPreview + remoteExecutionProfilePeerMetadata + addRemoteExecutionProfilePeerMetadata + registerCoreCommands.
// Code extracted from server_handle_run.go during the Day-52 god-file split. Public API unchanged.
package controlplane


import (
	"strings"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/runtime"
)


func publishRemoteExecutionProfileRunEvent(k *runtime.Kernel, corr string, kind event.Kind, suffix string, payload any) error {
	actor := "agent-" + corr
	_, err := k.Bus().Publish(event.Spec{
		Subject:       "agent." + actor + "." + suffix,
		Kind:          kind,
		Actor:         actor,
		CorrelationID: corr,
		Payload:       payload,
	})
	return err
}

func remoteExecutionProfileAnswerPreview(answer string) string {
	const max = 4096
	runes := []rune(answer)
	if len(runes) <= max {
		return answer
	}
	return string(runes[:max]) + "...[truncated]"
}

func remoteExecutionProfilePeerMetadata(answer string) map[string]string {
	lines := strings.Split(strings.TrimSpace(answer), "\n")
	if len(lines) == 0 {
		return nil
	}
	footer := strings.TrimSpace(lines[len(lines)-1])
	if !strings.HasPrefix(footer, "[") || !strings.HasSuffix(footer, "]") {
		return nil
	}
	fields := strings.Fields(strings.TrimSuffix(strings.TrimPrefix(footer, "["), "]"))
	meta := map[string]string{}
	for _, field := range fields {
		key, value, ok := strings.Cut(field, "=")
		if !ok || strings.TrimSpace(value) == "" {
			continue
		}
		switch key {
		case "peer":
			meta["remote_peer"] = value
		case "model":
			meta["remote_model"] = value
		case "correlation":
			meta["remote_correlation"] = value
		}
	}
	if len(meta) == 0 {
		return nil
	}
	return meta
}

func addRemoteExecutionProfilePeerMetadata(payload map[string]any, meta map[string]string) {
	for key, value := range meta {
		payload[key] = value
	}
}

// registerCoreCommands registers this file's protocol commands into the dispatch registry (phase 2.3).
func registerCoreCommands() {
	register(
		commandSpec{Cmd: CmdVersion, Handler: func(dc *DispatchCtx) { dc.S.handleVersion(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdRun, TenantAllowed: true, TenantRouted: true, Streaming: StreamEvents, Handler: func(dc *DispatchCtx) { dc.S.handleRun(dc.Ctx, dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdHalt, Handler: func(dc *DispatchCtx) { dc.S.handleHalt(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdResume, Handler: func(dc *DispatchCtx) { dc.S.handleResume(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWhy, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleWhy(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWhoami, TenantAllowed: true, Handler: func(dc *DispatchCtx) { dc.S.handleWhoami(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdJournalVerify, Handler: func(dc *DispatchCtx) { dc.S.handleVerify(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdApprovals, Handler: func(dc *DispatchCtx) { dc.S.handleApprovals(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdDecide, Handler: func(dc *DispatchCtx) { dc.S.handleDecide(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdPlan, Streaming: StreamEvents, Handler: func(dc *DispatchCtx) { dc.S.handlePlan(dc.Ctx, dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdCancelRun, TenantAllowed: true, TenantRouted: true, Handler: func(dc *DispatchCtx) { dc.S.handleCancelRun(dc.Conn, dc.Req) }},
	)
}


// SPDX-License-Identifier: MIT

// Workflow command registry: registerWorkflowCommands wires the handlers into the dispatch table.
// Code extracted from workflow.go during the Day-38 god-file split. Public API unchanged.
package controlplane





func registerWorkflowCommands() {
	register(
		commandSpec{Cmd: CmdWorkflowList, Handler: func(dc *DispatchCtx) { dc.S.handleWorkflowList(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWorkflowShow, Handler: func(dc *DispatchCtx) { dc.S.handleWorkflowShow(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWorkflowSave, Handler: func(dc *DispatchCtx) { dc.S.handleWorkflowSave(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWorkflowRestore, Handler: func(dc *DispatchCtx) { dc.S.handleWorkflowRestore(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWorkflowRemove, Handler: func(dc *DispatchCtx) { dc.S.handleWorkflowRemove(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWorkflowSetEnabled, Handler: func(dc *DispatchCtx) { dc.S.handleWorkflowSetEnabled(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWorkflowRun, Handler: func(dc *DispatchCtx) { dc.S.handleWorkflowRun(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWorkflowDraft, Handler: func(dc *DispatchCtx) { dc.S.handleWorkflowDraft(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWorkflowRefine, Handler: func(dc *DispatchCtx) { dc.S.handleWorkflowRefine(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWorkflowRuns, Handler: func(dc *DispatchCtx) { dc.S.handleWorkflowRuns(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWorkflowTemplates, Handler: func(dc *DispatchCtx) { dc.S.handleWorkflowTemplates(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWorkflowWebhook, Handler: func(dc *DispatchCtx) { dc.S.handleWorkflowWebhook(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdWorkflowTestNode, Handler: func(dc *DispatchCtx) { dc.S.handleWorkflowTestNode(dc.Conn, dc.Req) }},
	)
}

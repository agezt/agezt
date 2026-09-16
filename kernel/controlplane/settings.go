// SPDX-License-Identifier: MIT
//
// kernel/controlplane settings command registrar (registerSettingsCommands).
// Extracted from settings.go during Day 211 god-file refactor (#84).
// Public API unchanged.
package controlplane

func registerSettingsCommands() {
	register(
		commandSpec{Cmd: CmdConfigSchema, Handler: func(dc *DispatchCtx) { dc.S.handleConfigSchema(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdConfigValues, Handler: func(dc *DispatchCtx) { dc.S.handleConfigValues(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdConfigSet, Handler: func(dc *DispatchCtx) { dc.S.handleConfigSet(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdConfigSchemaRegister, Handler: func(dc *DispatchCtx) { dc.S.handleConfigSchemaRegister(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdConfigSchemaUnregister, Handler: func(dc *DispatchCtx) { dc.S.handleConfigSchemaUnregister(dc.Conn, dc.Req) }},
	)
}

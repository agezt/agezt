// SPDX-License-Identifier: MIT

package controlplane

// Provenance: Control-plane skill view helpers (isSkillKind + skillView +
//             registerSkillCommands). Code extracted from skill.go during the Day-88
//             god-file split. Public API unchanged.

// registerSkillCommands registers this file's protocol commands into the dispatch registry (phase 2.3).
func registerSkillCommands() {
	register(
		commandSpec{Cmd: CmdSkillList, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handleSkillList(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdSkillGet, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handleSkillGet(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdSkillHistory, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handleSkillHistory(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdSkillPromote, Handler: func(dc *DispatchCtx) { dc.S.handleSkillPromote(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdSkillQuarantine, Handler: func(dc *DispatchCtx) { dc.S.handleSkillQuarantine(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdSkillArchive, Handler: func(dc *DispatchCtx) { dc.S.handleSkillArchive(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdSkillRevert, Handler: func(dc *DispatchCtx) { dc.S.handleSkillRevert(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdSkillRestore, Handler: func(dc *DispatchCtx) { dc.S.handleSkillRestore(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdSkillShare, Handler: func(dc *DispatchCtx) { dc.S.handleSkillShare(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdSkillReassign, Handler: func(dc *DispatchCtx) { dc.S.handleSkillReassign(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdSkillImport, Handler: func(dc *DispatchCtx) { dc.S.handleSkillImport(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdSkillHygiene, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handleSkillHygiene(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdSkillFiles, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handleSkillFiles(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdSkillReadFile, ReadOnly: true, Handler: func(dc *DispatchCtx) { dc.S.handleSkillReadFile(dc.Conn, dc.Req) }},
	)
}

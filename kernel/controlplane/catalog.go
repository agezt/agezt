// SPDX-License-Identifier: MIT

package controlplane

// registerCatalogCommands registers this file's protocol commands into the dispatch registry (phase 2.3).
func registerCatalogCommands() {
	register(
		commandSpec{Cmd: CmdProviderReload, Handler: func(dc *DispatchCtx) { dc.S.handleProviderReload(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdProviderConnect, Handler: func(dc *DispatchCtx) { dc.S.handleProviderConnect(dc.Conn, dc.Req) }},
	)
}

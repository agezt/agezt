// SPDX-License-Identifier: MIT

package controlplane

import (
	"context"
	"fmt"
	"net"

	"github.com/agezt/agezt/kernel/runtime"
)

// Phase 2.3 (commit A): a declarative command registry that MIRRORS the
// 319-case switch in handleConn. Nothing dispatches through it yet — commit B
// swaps handleConn onto it and deletes the switch plus tenantTokenAllows.
// Until then, dispatch_registry_test.go pins the registry 1:1 against the
// protocol constants, the legacy switch, and the legacy tenant allowlist.

// DispatchCtx carries everything a command handler needs for one request.
// K is the pre-resolved kernel for the request (the primary kernel, or the
// tenant's kernel for tenant-routed commands) so handlers no longer each
// re-derive it via kernelFor(tenantOf(req)).
type DispatchCtx struct {
	Ctx     context.Context
	Conn    net.Conn
	Req     Request
	S       *Server
	K       *runtime.Kernel
	Tenant  string // authorized tenant id ("" for the primary token)
	Primary bool   // true when the request carried the primary token
}

// Handler executes one protocol command.
type Handler func(*DispatchCtx)

// StreamMode classifies how a command uses the connection after the request
// is read (drives deadline handling and disconnect-cancellation in commit B).
type StreamMode int

const (
	// StreamNone: plain request/response — one result or error, keep deadline.
	StreamNone StreamMode = iota
	// StreamEvents: emits RespEvent progress before the final result, but is
	// bounded work — the read/write deadline stays in place.
	StreamEvents
	// StreamLive: long-lived streaming (subscription or LLM stream) — the
	// deadline is cleared and the handler's context is cancelled when the
	// client disconnects.
	StreamLive
)

// commandSpec declares one protocol command: its handler plus the metadata
// that today lives implicitly in handleConn and tenantTokenAllows.
//
//   - TenantAllowed: a TENANT token may invoke it (copied verbatim from the
//     tenantTokenAllows allowlist; deny-by-default).
//   - TenantRouted: the handler resolves its kernel per-request via
//     kernelFor / edictFor / projectJournal rather than using s.k directly.
//     Invariant (TestRegistry_TenantAllowedImpliesTenantRouted): every
//     TenantAllowed command must be TenantRouted, or a tenant token would
//     read the PRIMARY kernel's data. whoami is the sole, explicit exception.
//   - Streaming: see StreamMode.
type commandSpec struct {
	Cmd           string
	Handler       Handler
	TenantAllowed bool
	TenantRouted  bool
	Streaming     StreamMode
}

// commandRegistry maps command name → spec. Populated once at init by
// registerAllCommands; read-only afterwards.
var commandRegistry = map[string]commandSpec{}

// register adds specs to commandRegistry, panicking on duplicates or empty
// command names — both are programmer errors caught at process start (and by
// every test run, since init runs first).
func register(specs ...commandSpec) {
	for _, sp := range specs {
		if sp.Cmd == "" {
			panic("controlplane: register: empty command name")
		}
		if sp.Handler == nil {
			panic(fmt.Sprintf("controlplane: register: nil handler for %q", sp.Cmd))
		}
		if _, dup := commandRegistry[sp.Cmd]; dup {
			panic(fmt.Sprintf("controlplane: register: duplicate command %q", sp.Cmd))
		}
		commandRegistry[sp.Cmd] = sp
	}
}

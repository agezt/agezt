// SPDX-License-Identifier: MIT

package main

// Provenance: The daemon's agent gateway (W3.67). The gateway is the transport
//             the agent SDK's subprocesses talk to — a capability-scoped HTTP
//             surface over the daemon's bus, memory, roster and config center.
//
//             It used to be built INSIDE the kernel (runtime.Open), which was
//             an upward import edge (L3 module -> L5 adapter) and a layering
//             lie: a kernel that holds a gateway serves an inbound transport
//             it does not own. The daemon — the composition root — builds it
//             over the kernel's own ports instead, starts it and closes it.
//
//             ONE gateway per DAEMON, not one per kernel: a tenant kernel used
//             to build its own gateway as well — over a random socket path
//             that nothing published, or, with AGEZT_AGENTGW_SOCKET set, over
//             the SAME path, so at most one of them could ever bind it and
//             the losers were only logged. A tenant's subprocess now talks to
//             the daemon's gateway.

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/agezt/agezt/kernel/agentgw"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
)

// agentgwSocketEnv overrides the gateway's socket path.
//
// The name is a LITERAL, not brand.EnvPrefix+"AGENTGW_SOCKET": the settings
// schema, the control plane's known-env list and the SDK all spell it
// AGEZT_AGENTGW_SOCKET, and a daemon that reads a different spelling would
// silently bind the default path while the operator believes it moved.
const agentgwSocketEnv = "AGEZT_AGENTGW_SOCKET"

// startAgentGateway builds the daemon's agent gateway over the kernel's ports
// and starts listening.
//
// The token secret is a per-install secret persisted under baseDir (or
// $AGEZT_AGENTGW_TOKEN_SECRET), shared with the `agt` CLI that mints the
// subprocess tokens. It is NEVER an ephemeral key: a per-process key would
// sign tokens the CLI cannot verify, and the SDK path would fail closed with
// a 401 that reads like a bug. A secret that cannot be resolved is a STARTUP
// ERROR — exactly as it was when the kernel resolved it.
//
// The listener, on the other hand, is best-effort: a gateway that cannot bind
// (the path is taken, the platform has no unix sockets) leaves the daemon
// serving everything else, and the failure is logged rather than fatal. That
// is the behaviour the kernel had, and making it fatal would turn a
// convenience transport into a boot dependency.
func startAgentGateway(k *kernelruntime.Kernel, baseDir string) (*agentgw.Gateway, error) {
	gwCfg := agentgw.DefaultGatewayConfig(baseDir)

	secret, err := agentgw.ResolveTokenSecret(baseDir)
	if err != nil {
		return nil, fmt.Errorf("resolve agentgw token secret: %w", err)
	}
	gwCfg.TokenSecret = secret

	// Override the socket path from the environment if set (useful for
	// Windows TCP testing, and for a fixed path an SDK subprocess can find).
	if sockPath := strings.TrimSpace(os.Getenv(agentgwSocketEnv)); sockPath != "" {
		gwCfg.SocketPath = sockPath
	}

	gw := agentgw.NewGateway(gwCfg)
	// The gateway speaks FOR the daemon: it publishes on the daemon's bus,
	// reads and writes the daemon's memory, lists the daemon's roster and
	// serves the daemon's config center.
	gw.Attach(k.Bus(), k.Memory(), k.Roster())
	gw.SetConfigCenter(k.ConfigCenter())
	gw.SetAuditJournal(k.Journal()) // wire the audit trail (was a nil no-op)

	if err := gw.Listen(context.Background()); err != nil {
		slog.Error("agentgw: listen", "error", err)
	}
	return gw, nil
}

// shutdownAgentGateway closes the daemon's agent gateway and then the kernel.
//
// The ORDER is the point, and it is written here rather than left to the
// order two `defer`s happen to be registered in: the gateway's audit trail
// is buffered and flushed on Close, and the flush appends to the kernel's
// journal. A kernel closed first would take the journal with it, and the
// buffered audit entries — the record of who asked for what — would be
// lost. (Inside the kernel it was the other way round: the journal closed
// before the gateway flushed.)
func shutdownAgentGateway(gw *agentgw.Gateway, k *kernelruntime.Kernel) {
	if gw != nil {
		_ = gw.Close()
	}
	if k != nil {
		_ = k.Close()
	}
}

// SPDX-License-Identifier: MIT

package controlplane

import (
	"context"
	"net"
	"os"
	"strings"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/acpcatalog"
)

// handleACPAgents returns the host's Agent Client Protocol agent inventory: which
// catalog ACP agents (Gemini CLI, Claude Code's adapter, Codex) are installed
// (+version+path), which are missing (+install hint+docs), and which command is
// the configured default (AGEZT_ACP_AGENT_CMD). Read-only — it never launches an
// agent's ACP loop, only probes presence/version. Drives the web UI picker and
// the acp_agent bridge's agent selection.
func (s *Server) handleACPAgents(ctx context.Context, conn net.Conn, req Request) {
	active := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "ACP_AGENT_CMD"))
	inv := acpcatalog.Discover(ctx, active, false)
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: structToMap(inv)})
}

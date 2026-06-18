// SPDX-License-Identifier: MIT

package controlplane

import (
	"context"
	"net"
	"strings"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/market"
)

// handleMarketList returns the marketplace catalogue (every pack across
// registered marketplaces) joined with install state. Optional args.query
// filters by name/description/category/tags. Read-only.
func (s *Server) handleMarketList(conn net.Conn, req Request) {
	m := s.market()
	if m == nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "marketplace not available on this daemon"})
		return
	}
	listings, err := m.List(strArg(req.Args["query"]))
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	rows := make([]map[string]any, 0, len(listings))
	for _, l := range listings {
		rows = append(rows, structToMap(l))
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"packs": rows,
		"count": len(rows),
	}})
}

// handleMarketShow resolves one pack's full contents + install state.
func (s *Server) handleMarketShow(conn net.Conn, req Request) {
	m := s.market()
	if m == nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "marketplace not available on this daemon"})
		return
	}
	name := strings.TrimSpace(strArg(req.Args["name"]))
	if name == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.name required"})
		return
	}
	pack, installed, isInstalled, err := m.Show(strArg(req.Args["marketplace"]), name)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	skills, mcps, tools := pack.Counts()
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"pack":         structToMap(pack),
		"skill_count":  skills,
		"mcp_count":    mcps,
		"tool_count":   tools,
		"installed":    isInstalled,
		"installed_at": installed.InstalledMS,
	}})
}

// handleMarketInstall materializes a pack (skills→Forge, MCP→registry, tool reqs
// reported), streaming a progress event per item then a final record. Operator-
// gated (authed write path), consistent with the default-allow posture.
func (s *Server) handleMarketInstall(ctx context.Context, conn net.Conn, req Request) {
	m := s.market()
	if m == nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "marketplace not available on this daemon"})
		return
	}
	name := strings.TrimSpace(strArg(req.Args["name"]))
	if name == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.name required"})
		return
	}
	// Stream a progress frame per materialized item (skill/mcp/tool), then the
	// final record. The webui market install proxy forwards these as SSE.
	emit := func(e market.Event) {
		s.writeResp(conn, Response{ID: req.ID, Type: RespEvent, Event: &event.Event{
			Kind:    event.Kind("market.install.progress"),
			Subject: "market.install",
			Actor:   "market",
			Payload: mustJSONRaw(e),
		}})
	}
	rec, err := m.Install(strArg(req.Args["correlation_id"]), strArg(req.Args["marketplace"]), name, strArg(req.Args["version"]), emit)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	s.publishMarket("market.pack.installed", map[string]any{
		"pack": rec.Name, "version": rec.Version, "marketplace": rec.Marketplace,
		"skills": rec.SkillIDs, "mcp": rec.MCPServers, "tools": rec.ToolReqs, "unsigned": rec.Unsigned,
	})
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: structToMap(rec)})
}

// handleMarketUninstall reverses a pack's footprint (quarantine its skills,
// remove its MCP servers) via the recorded provenance, streaming progress.
func (s *Server) handleMarketUninstall(ctx context.Context, conn net.Conn, req Request) {
	m := s.market()
	if m == nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "marketplace not available on this daemon"})
		return
	}
	name := strings.TrimSpace(strArg(req.Args["name"]))
	if name == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.name required"})
		return
	}
	emit := func(e market.Event) {
		s.writeResp(conn, Response{ID: req.ID, Type: RespEvent, Event: &event.Event{
			Kind:    event.Kind("market.uninstall.progress"),
			Subject: "market.uninstall",
			Actor:   "market",
			Payload: mustJSONRaw(e),
		}})
	}
	if err := m.Uninstall(strArg(req.Args["correlation_id"]), name, emit); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	s.publishMarket("market.pack.uninstalled", map[string]any{"pack": name})
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"uninstalled": name}})
}

// handleMarketSources lists the configured remote marketplace sources. Read-only.
func (s *Server) handleMarketSources(conn net.Conn, req Request) {
	m := s.market()
	if m == nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "marketplace not available on this daemon"})
		return
	}
	srcs, err := m.Sources()
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	rows := make([]map[string]any, 0, len(srcs))
	for _, src := range srcs {
		rows = append(rows, structToMap(src))
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"sources": rows, "count": len(rows)}})
}

// handleMarketAddSource registers a remote marketplace source (does not fetch).
func (s *Server) handleMarketAddSource(conn net.Conn, req Request) {
	m := s.market()
	if m == nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "marketplace not available on this daemon"})
		return
	}
	rawURL := strArg(req.Args["url"])
	if rawURL == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.url required"})
		return
	}
	src, err := m.AddSource(strArg(req.Args["name"]), rawURL, strArg(req.Args["pubkey"]))
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	s.publishMarket("market.source.added", map[string]any{"source": src.Name, "url": src.URL})
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: structToMap(src)})
}

// handleMarketRemoveSource drops a source + its cached catalogue.
func (s *Server) handleMarketRemoveSource(conn net.Conn, req Request) {
	m := s.market()
	if m == nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "marketplace not available on this daemon"})
		return
	}
	name := strArg(req.Args["name"])
	if name == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.name required"})
		return
	}
	found, err := m.RemoveSource(name)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	s.publishMarket("market.source.removed", map[string]any{"source": name})
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"removed": found, "name": name}})
}

// handleMarketSync fetches a source's catalogue (or all sources when name is
// empty) into the local cache, keep-last-good.
func (s *Server) handleMarketSync(ctx context.Context, conn net.Conn, req Request) {
	m := s.market()
	if m == nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "marketplace not available on this daemon"})
		return
	}
	results, err := m.Sync(ctx, strArg(req.Args["name"]))
	if err != nil && len(results) == 0 {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	rows := make([]map[string]any, 0, len(results))
	total := 0
	for _, r := range results {
		rows = append(rows, structToMap(r))
		total += r.Packs
	}
	s.publishMarket("market.synced", map[string]any{"sources": len(rows), "packs": total})
	out := map[string]any{"results": rows, "synced": len(rows), "packs": total}
	if err != nil {
		out["partial_error"] = err.Error() // some sources synced, some failed
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: out})
}

// strArg reads a trimmed string from a decoded JSON args value.
func strArg(v any) string {
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

// parseCapList parses a decoded "auto_approve_caps" arg — a comma/space-separated
// string OR a JSON array of strings — into a capability set. Used by handleRun to
// thread the chat's session-scoped auto-approve grant into the run context.
func parseCapList(v any) map[string]bool {
	add := func(out map[string]bool, raw string) {
		for _, f := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' || r == '\n' }) {
			if f = strings.TrimSpace(f); f != "" {
				out[f] = true
			}
		}
	}
	out := map[string]bool{}
	switch t := v.(type) {
	case string:
		add(out, t)
	case []any:
		for _, e := range t {
			if s, ok := e.(string); ok {
				add(out, s)
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (s *Server) market() *market.Manager {
	if s.k == nil {
		return nil
	}
	return s.k.Market()
}

func (s *Server) publishMarket(kind string, payload map[string]any) {
	if s.k == nil || s.k.Bus() == nil {
		return
	}
	_, _ = s.k.Bus().Publish(event.Spec{Subject: "market", Kind: event.Kind(kind), Actor: "market", Payload: payload})
}

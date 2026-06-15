// SPDX-License-Identifier: MIT

package controlplane

import (
	"context"
	"encoding/json"
	"net"

	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/toolbox"
)

// handleToolboxDetect returns the host CLI-tool inventory (M956): which catalog
// tools are installed (+version+path), which are missing, and which package
// managers are available. Read-only.
func (s *Server) handleToolboxDetect(ctx context.Context, conn net.Conn, req Request) {
	inv := toolbox.Detect(ctx)
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: structToMap(inv)})
}

// handleToolboxOutdated returns the set of catalog tools the host package
// managers report as upgradable (best-effort, owner's "ne güncel" view).
func (s *Server) handleToolboxOutdated(ctx context.Context, conn net.Conn, req Request) {
	out := toolbox.Outdated(ctx)
	names := make([]string, 0, len(out))
	for n := range out {
		names = append(names, n)
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"outdated": names,
		"count":    len(names),
	}})
}

// handleToolboxInstall installs one or more named catalog tools via the host
// package manager, streaming a progress event per tool then a final summary
// result. Each install runs the real manager at host level (winget/brew/apt…);
// every attempt is journaled (toolbox.*) so `agt why` can trace it. The names
// come from the authed operator (web UI), consistent with the default-allow
// host-capability posture.
func (s *Server) handleToolboxInstall(ctx context.Context, conn net.Conn, req Request) {
	names := stringSliceArg(req.Args["names"])
	if len(names) == 0 {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.names (non-empty list) required"})
		return
	}

	s.publishToolbox("toolbox.install.requested", map[string]any{"tools": names})

	installed := make([]string, 0, len(names))
	failed := make([]string, 0)
	skipped := make([]string, 0)

	for _, name := range names {
		if err := ctx.Err(); err != nil {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
			return
		}
		res := toolbox.Install(ctx, name)
		switch {
		case res.OK:
			installed = append(installed, name)
		case res.Skipped:
			skipped = append(skipped, name)
		default:
			failed = append(failed, name)
		}
		// Journal the outcome (audit trail).
		s.publishToolbox("toolbox.installed", map[string]any{
			"tool": res.Tool, "ok": res.OK, "skipped": res.Skipped,
			"manager": res.Manager, "command": res.Command, "version": res.Version, "error": res.Error,
		})
		// Stream the per-tool progress to the browser.
		s.writeResp(conn, Response{ID: req.ID, Type: RespEvent, Event: &event.Event{
			Kind:    event.Kind("toolbox.progress"),
			Subject: "toolbox.install",
			Actor:   "toolbox",
			Payload: mustJSONRaw(res),
		}})
	}

	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"installed": installed,
		"failed":    failed,
		"skipped":   skipped,
	}})
}

// publishToolbox records a toolbox lifecycle event on the bus (best-effort).
func (s *Server) publishToolbox(kind string, payload map[string]any) {
	if s.k == nil || s.k.Bus() == nil {
		return
	}
	_, _ = s.k.Bus().Publish(event.Spec{
		Subject: "toolbox",
		Kind:    event.Kind(kind),
		Actor:   "toolbox",
		Payload: payload,
	})
}

// structToMap round-trips a JSON-tagged struct into a map so a handler can put
// it in Response.Result (which the web UI's read proxy forwards verbatim).
func structToMap(v any) map[string]any {
	b, err := json.Marshal(v)
	if err != nil {
		return map[string]any{}
	}
	var m map[string]any
	if json.Unmarshal(b, &m) != nil {
		return map[string]any{}
	}
	return m
}

func mustJSONRaw(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return b
}

// stringSliceArg coerces a JSON args value (decoded as []any of strings) into a
// []string, dropping blanks.
func stringSliceArg(raw any) []string {
	list, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(list))
	for _, x := range list {
		if s, ok := x.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}

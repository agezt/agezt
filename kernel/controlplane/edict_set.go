// SPDX-License-Identifier: MIT
//
// Edict runtime setters: handleEdictSetLevel + handleEdictSetMode.
// Extracted from edict.go during the Day-204 god-file split.
// Public API unchanged.
package controlplane

import (
	"net"
	"slices"

	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/event"
)

// handleEdictSetLevel changes a capability's trust level at runtime and
// journals a policy.changed event. The capability must be known (a typo
// would otherwise create a default-deny phantom entry); the level string
// is parsed leniently (L0..L4 or aliases). The previous level is captured
// for the event + response so the change is fully reconstructable.
func (s *Server) handleEdictSetLevel(conn net.Conn, req Request) {
	capStr, err := requiredArgString(req.Args, "capability")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	cap := edict.Capability(capStr)
	if !slices.Contains(edict.AllCapabilities(), cap) {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError,
			Error: "unknown capability " + capStr + " (see `edict show` for the governed set)"})
		return
	}
	levelStr, _, err := argString(req.Args, "level")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	lvl, err := edict.ParseTrustLevel(levelStr)
	if err != nil {
		s.fail(conn, req, err)
		return
	}

	eng, k, ok := s.edictFor(conn, req)
	if !ok {
		return
	}
	from := "unset"
	if prev, ok := eng.Level(cap); ok {
		from = prev.String()
	}
	eng.SetLevel(cap, lvl)
	to := lvl.String()

	_, _ = k.Bus().Publish(event.Spec{
		Subject: "kernel.policy",
		Kind:    event.KindPolicyChanged,
		Actor:   "operator",
		Payload: map[string]any{
			"action":     "level.set",
			"capability": capStr,
			"from":       from,
			"to":         to,
		},
	})
	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			"capability": capStr,
			"from":       from,
			"to":         to,
		},
	})
}

// handleEdictSetMode changes the engine-wide approval mode at runtime and
// journals a policy.changed event. The mode string is parsed leniently
// (allow/deny/prompt); the previous mode is captured for the event +
// response so the change is fully reconstructable (and replayable, M20).
func (s *Server) handleEdictSetMode(conn net.Conn, req Request) {
	modeStr, _, err := argString(req.Args, "mode")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	mode, err := edict.ParseAskPolicy(modeStr)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	eng, k, ok := s.edictFor(conn, req)
	if !ok {
		return
	}
	from := eng.AskPolicy().String()
	eng.SetAskPolicy(mode)
	to := mode.String()

	_, _ = k.Bus().Publish(event.Spec{
		Subject: "kernel.policy",
		Kind:    event.KindPolicyChanged,
		Actor:   "operator",
		Payload: map[string]any{
			"action": "mode.set",
			"from":   from,
			"to":     to,
		},
	})
	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"from": from, "to": to},
	})
}

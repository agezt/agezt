// SPDX-License-Identifier: MIT

package controlplane

// Durable policy overlay view (M94) — surfaces the NET effect of every runtime
// policy change (`agt edict level`/`mode`/`deny`, journaled as policy.changed)
// folded via the same edict.ProjectPolicyChanges the daemon replays at boot
// (M20). `agt edict show` shows the BASE rules (loaded config); `agt edict log`
// shows the raw change events; this shows what runtime policy is ACTUALLY IN
// EFFECT — the collapsed result an operator (and a future compaction) cares
// about. Read-only; reuses the battle-tested boot fold, no new policy logic.

import (
	"encoding/json"
	"net"

	"github.com/agezt/agezt/kernel/edict"
	"github.com/agezt/agezt/kernel/event"
)

func (s *Server) handleEdictOverlay(conn net.Conn, req Request) {
	k, err := s.kernelFor(tenantOf(req))
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}

	var changes []edict.PolicyChange
	if err := k.Journal().Range(func(e *event.Event) error {
		if e.Kind != event.KindPolicyChanged {
			return nil
		}
		var ch edict.PolicyChange
		if json.Unmarshal(e.Payload, &ch) != nil {
			return nil // skip malformed, exactly as the boot replay does
		}
		changes = append(changes, ch)
		return nil
	}); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	overlay := edict.ProjectPolicyChanges(changes)

	levels := map[string]any{}
	for cap, lvl := range overlay.Levels {
		levels[string(cap)] = lvl.String()
	}
	denies := make([]map[string]any, 0, len(overlay.DenyRules))
	for _, r := range overlay.DenyRules {
		applies := make([]string, 0, len(r.AppliesTo))
		for _, c := range r.AppliesTo {
			applies = append(applies, string(c))
		}
		denies = append(denies, map[string]any{
			"name":       r.Name,
			"substring":  r.Substring,
			"applies_to": applies,
		})
	}
	var mode string
	if overlay.Mode != nil {
		mode = overlay.Mode.String()
	}
	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			"levels":         levels,
			"deny_rules":     denies,
			"mode":           mode,
			"empty":          overlay.IsEmpty(),
			"changes_folded": len(changes),
		},
	})
}

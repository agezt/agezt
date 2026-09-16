// SPDX-License-Identifier: MIT
//
// kernel/controlplane standing-order agent validator (validateStandingAgent).
// Extracted from standing_handlers.go during Day 211 god-file refactor (#83).
// Public API unchanged.
package controlplane

import (
	"fmt"
	"strings"
)

func (s *Server) validateStandingAgent(ref string) error {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil
	}
	p, ok := s.k.Roster().Get(ref)
	if !ok {
		return fmt.Errorf("unknown standing agent: %s", ref)
	}
	if p.Retired {
		return fmt.Errorf("standing agent %s is retired", p.Slug)
	}
	if !p.Enabled {
		return fmt.Errorf("standing agent %s is paused", p.Slug)
	}
	if !p.AllowsDirectCall() {
		return fmt.Errorf("standing %s", managedSubagentDirectCallError(p, "called"))
	}
	return nil
}

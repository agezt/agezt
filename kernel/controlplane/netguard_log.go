// SPDX-License-Identifier: MIT

package controlplane

// Egress-block audit (M109). The netguard egress guard refuses the http/browser
// tools' connections to internal/metadata addresses and now journals each refusal
// as a netguard.blocked event. This folds those events into an audit timeline so
// an operator can see what was stopped — a tool reaching for 169.254.169.254 is a
// strong SSRF / prompt-injection / exfiltration signal. Sister to `agt netguard
// test` (M105), which previews the policy; this records what it actually blocked.

import (
	"encoding/json"
	"net"

	"github.com/agezt/agezt/kernel/event"
)

func (s *Server) handleNetguardLog(conn net.Conn, req Request) {
	s.projectJournal(conn, req, "blocks", func(e *event.Event) (map[string]any, bool) {
		if e.Kind != event.KindNetguardBlocked {
			return nil, false
		}
		var p struct {
			IP     string `json:"ip"`
			Reason string `json:"reason"`
			Tool   string `json:"tool"`
		}
		_ = json.Unmarshal(e.Payload, &p)
		return map[string]any{"ip": p.IP, "reason": p.Reason, "tool": p.Tool}, true
	})
}

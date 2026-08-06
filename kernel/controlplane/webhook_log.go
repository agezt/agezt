// SPDX-License-Identifier: MIT

package controlplane

// Webhook delivery observability (M112). The outbound webhook dispatcher journals
// webhook.delivered (a 2xx) and webhook.failed (exhausted retries) for every
// event it POSTs to an operator-configured sink. Those were only reachable via
// `agt journal grep webhook`; this folds them into a first-class surface so an
// operator can see whether their notifications are getting through — a webhook
// silently failing is the classic "I never got paged" outage. Mirrors the
// edict/warden/provider log+stats pattern.

import (
	"encoding/json"
	"net"

	"github.com/agezt/agezt/kernel/event"
)

func (s *Server) handleWebhookLog(conn net.Conn, req Request) {
	failedOnly, _, err := argBool(req.Args, "failed")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	s.projectJournal(conn, req, "deliveries", func(e *event.Event) (map[string]any, bool) {
		isDelivered := e.Kind == event.KindWebhookDelivered
		isFailed := e.Kind == event.KindWebhookFailed
		if !isDelivered && !isFailed {
			return nil, false
		}
		if failedOnly && !isFailed {
			return nil, false
		}
		var p struct {
			URL       string `json:"url"`
			EventKind string `json:"event_kind"`
			Status    int    `json:"status"`
			Attempts  int    `json:"attempts"`
			Error     string `json:"error"`
		}
		_ = json.Unmarshal(e.Payload, &p)
		m := map[string]any{
			"ok":         isDelivered,
			"url":        p.URL,
			"event_kind": p.EventKind,
			"attempts":   p.Attempts,
		}
		if isDelivered {
			m["status"] = p.Status
		} else {
			m["error"] = p.Error
		}
		return m, true
	})
}

func (s *Server) handleWebhookStats(conn net.Conn, req Request) {
	cutoff := sinceCutoff(req.Args["since_ms"])
	sinceMS := int64Arg(req.Args["since_ms"])

	k, err := s.kernelFor(tenantOf(req))
	if err != nil {
		s.fail(conn, req, err)
		return
	}

	var delivered, failed int
	byURL := map[string][2]int{} // url → {delivered, failed}
	if err := k.Journal().Range(func(e *event.Event) error {
		isDelivered := e.Kind == event.KindWebhookDelivered
		isFailed := e.Kind == event.KindWebhookFailed
		if !isDelivered && !isFailed {
			return nil
		}
		if cutoff > 0 && e.TSUnixMS < cutoff {
			return nil
		}
		var p struct {
			URL string `json:"url"`
		}
		_ = json.Unmarshal(e.Payload, &p)
		c := byURL[p.URL]
		if isDelivered {
			delivered++
			c[0]++
		} else {
			failed++
			c[1]++
		}
		byURL[p.URL] = c
		return nil
	}); err != nil {
		s.fail(conn, req, err)
		return
	}

	total := delivered + failed
	failureRate := 0.0
	if total > 0 {
		failureRate = float64(failed) / float64(total)
	}
	byURLOut := make(map[string]any, len(byURL))
	for u, c := range byURL {
		byURLOut[u] = map[string]any{"delivered": c[0], "failed": c[1]}
	}
	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			"total":        total,
			"delivered":    delivered,
			"failed":       failed,
			"failure_rate": failureRate,
			"by_url":       byURLOut,
			"window_ms":    sinceMS,
		},
	})
}

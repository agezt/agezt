// SPDX-License-Identifier: MIT

package controlplane

// Council of Elders control plane (M839): the Web UI consults the multi-model
// panel (kernel/runtime, M837). `council_ask` convenes the panel on a question and
// streams the deliberation and consensus; the membership view and edit are typed
// operations in app/council. The agent reaches the same engine through the
// `council` tool.

import (
	"context"
	"net"
	"strings"
)

// sanitizeCorr accepts a client-supplied correlation id only if it's a short,
// plain token ([A-Za-z0-9_-], <=80 chars) — the id becomes a bus subject suffix
// and event field, so we don't let arbitrary text through. Anything else returns
// "" and the caller mints a server-side id instead.
func sanitizeCorr(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || len(s) > 80 {
		return ""
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
			return ""
		}
	}
	return s
}

func (s *Server) handleCouncilAsk(ctx context.Context, conn net.Conn, req Request) {
	question := stringArg(req.Args, "question")
	if question == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.question required"})
		return
	}
	rounds := dlInt(req.Args, "rounds")
	// The Web UI may pass its own correlation id so it can subscribe to the live
	// council.* event stream for THIS run before the (blocking) call returns —
	// letting it follow the deliberation and survive a navigation away (M987). A
	// missing/odd id falls back to a fresh server-generated one.
	corr := sanitizeCorr(stringArg(req.Args, "corr"))
	if corr == "" {
		corr = s.k.NewCorrelation()
	}
	// ctx is already tied to the connection by dispatch (StreamLive): a
	// disconnected client can't receive the deliberation, so the panel is
	// cancelled instead of spending every seat's model call into a closed
	// connection.
	res, err := s.k.Council(ctx, corr, question, nil, rounds)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	opinions := make([]map[string]any, 0, len(res.Opinions))
	for _, op := range res.Opinions {
		row := map[string]any{"seat": op.Seat, "model": op.Model, "round": op.Round, "text": op.Text}
		if op.Error != "" {
			row["error"] = op.Error
		}
		opinions = append(opinions, row)
	}
	members := make([]map[string]any, 0, len(res.Members))
	for _, m := range res.Members {
		members = append(members, map[string]any{"seat": m.Seat, "model": m.Model})
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"correlation_id": corr,
		"question":       res.Question,
		"consensus":      res.Consensus,
		"dissent":        res.Dissent,
		"rounds":         res.Rounds,
		"members":        members,
		"opinions":       opinions,
		"as_of":          res.AsOf,
		"brief":          res.Brief,
	}})
}

// handleCouncilSet replaces the default Council membership. args.members is an
// array of {seat, model}. Applies live and persists to the config store so it
// survives restart (stored as AGEZT_COUNCIL_MEMBERS, same format main.go reads).

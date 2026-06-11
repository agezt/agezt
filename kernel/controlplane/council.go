// SPDX-License-Identifier: MIT

package controlplane

// Council of Elders control plane (M839): the Web UI consults the multi-model
// panel (kernel/runtime, M837). `council_members` shows which models will speak;
// `council_ask` convenes the panel on a question and returns the deliberation +
// consensus. The agent reaches the same engine through the `council` tool.

import (
	"context"
	"net"
)

func (s *Server) handleCouncilMembers(conn net.Conn, req Request) {
	members := s.k.CouncilDefaultMembers()
	out := make([]map[string]any, 0, len(members))
	for _, m := range members {
		out = append(out, map[string]any{"seat": m.Seat, "model": m.Model})
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"members": out,
		"count":   len(out),
	}})
}

func (s *Server) handleCouncilAsk(ctx context.Context, conn net.Conn, req Request) {
	question := stringArg(req.Args, "question")
	if question == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.question required"})
		return
	}
	rounds := dlInt(req.Args, "rounds")
	corr := s.k.NewCorrelation()
	res, err := s.k.Council(ctx, corr, question, nil, rounds)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
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
	}})
}

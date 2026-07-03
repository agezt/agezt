// SPDX-License-Identifier: MIT

package controlplane

// Conductor control plane (M997): the operator/Web UI surface for the asymmetric,
// verify-driven panel (kernel/runtime, M997). `conductor_roles` previews which
// model fills each role; `conductor_ask` runs the full Thinker→Worker→Verifier
// loop and returns the answer + transcript. The agent reaches the same engine
// through the `conductor` tool.

import (
	"context"
	"net"

	"github.com/agezt/agezt/kernel/runtime"
)

// handleConductorRoles returns the default role→model assignment the Conductor
// uses when roles aren't supplied (one distinct keyed-provider model per role,
// cycling if fewer than three). Mirrors handleCouncilMembers — a read-only
// preview so the UI/CLI can show who will fill each role before a run.
func (s *Server) handleConductorRoles(conn net.Conn, req Request) {
	members := s.k.CouncilDefaultMembers()
	models := make([]string, 0, len(members))
	for _, m := range members {
		models = append(models, m.Model)
	}
	pick := func(i int) string {
		if len(models) == 0 {
			return ""
		}
		return models[i%len(models)]
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"thinker":          pick(0),
		"worker":           pick(1),
		"verifier":         pick(2),
		"available_models": models,
		"auto_filled":      true,
	}})
}

// handleConductorAsk runs the full Conductor loop and returns the result.
// Mirrors handleCouncilAsk: an optional client-supplied correlation id lets the
// Web UI subscribe to the live conductor.* event stream for this run before the
// (blocking) call returns.
func (s *Server) handleConductorAsk(ctx context.Context, conn net.Conn, req Request) {
	task := stringArg(req.Args, "task")
	if task == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.task required"})
		return
	}
	corr := sanitizeCorr(stringArg(req.Args, "corr"))
	if corr == "" {
		corr = s.k.NewCorrelation()
	}
	// A disconnected client can't receive the answer — cancel the Thinker→
	// Worker→Verifier loop instead of spending it into a closed connection.
	ctx, cancel := cancelOnConnClose(ctx, conn)
	defer cancel()

	res, err := s.k.Conduct(ctx, corr, runtime.ConductorConfig{
		Task:      task,
		Thinker:   stringArg(req.Args, "thinker"),
		Worker:    stringArg(req.Args, "worker"),
		Verifier:  stringArg(req.Args, "verifier"),
		MaxRounds: dlInt(req.Args, "max_rounds"),
		Plan:      dlBool(req.Args, "plan"),
	})
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	steps := make([]map[string]any, 0, len(res.Steps))
	for _, st := range res.Steps {
		row := map[string]any{"round": st.Round, "role": st.Role, "model": st.Model}
		if st.Text != "" {
			row["text"] = st.Text
		}
		if st.Verdict != "" {
			row["verdict"] = st.Verdict
		}
		if st.Reason != "" {
			row["reason"] = st.Reason
		}
		if st.Error != "" {
			row["error"] = st.Error
		}
		if st.Exec != nil {
			row["exec"] = map[string]any{
				"ran": st.Exec.Ran, "ok": st.Exec.OK,
				"language": st.Exec.Language, "output": st.Exec.Output,
			}
		}
		steps = append(steps, row)
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"correlation_id": corr,
		"task":           res.Task,
		"answer":         res.Answer,
		"passed":         res.Passed,
		"roles":          res.Roles,
		"rounds":         res.Rounds,
		"plan":           res.Plan,
		"steps":          steps,
	}})
}

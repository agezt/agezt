// SPDX-License-Identifier: MIT

package controlplane

// Conductor control plane (M997): the operator/Web UI surface for the asymmetric,
// verify-driven panel (kernel/runtime, M997). `conductor_ask` runs the full
// Thinker→Worker→Verifier loop and returns the answer + transcript; the role
// preview is the typed `conductor_roles` in app/council. The agent reaches the
// same engine through the `conductor` tool.

import (
	"context"
	"net"

	"github.com/agezt/agezt/kernel/runtime"
)

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
	// ctx is already tied to the connection by dispatch (StreamLive): a
	// disconnected client can't receive the answer, so the Thinker→Worker→
	// Verifier loop is cancelled instead of being spent into a closed connection.
	res, err := s.k.Conduct(ctx, corr, runtime.ConductorConfig{
		Task:      task,
		Thinker:   stringArg(req.Args, "thinker"),
		Worker:    stringArg(req.Args, "worker"),
		Verifier:  stringArg(req.Args, "verifier"),
		MaxRounds: dlInt(req.Args, "max_rounds"),
		Plan:      dlBool(req.Args, "plan"),
	})
	if err != nil {
		s.fail(conn, req, err)
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

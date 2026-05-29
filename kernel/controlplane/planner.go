// SPDX-License-Identifier: MIT

package controlplane

import (
	"context"
	"encoding/json"
	"net"

	"github.com/ersinkoc/agezt/kernel/planner"
)

// handlePlanGenerate calls planner.Generate with the daemon's
// configured Provider and returns the resulting JSON string +
// node count. The CLI typically forwards the JSON to CmdPlan to
// execute, or prints it for the operator to review.
//
// **Why not generate-then-execute in one command.** Two reasons:
//
//  1. Operators frequently want to *audit* a generated plan before
//     running it — especially the first few times they try the
//     planner. A single combined command would either fire blindly
//     or paper a confirmation prompt over the wire (which doesn't
//     compose with `agt pulse`, `--json` tooling, etc.).
//  2. The CLI side composes these cleanly: `agt plan run <intent>`
//     calls Generate then forwards the JSON to CmdPlan in the same
//     terminal session. That keeps the server endpoints small and
//     single-purpose; the orchestration lives on the client where
//     a Ctrl+C in the middle is unambiguous.
//
// Args:
//
//	intent (string, required) — the natural-language task
//	model  (string, optional) — override the provider's default
//
// Returns (RespResult):
//
//	plan_json  (string) — the validated plan JSON
//	node_count (int)    — number of nodes in the generated plan
func (s *Server) handlePlanGenerate(ctx context.Context, conn net.Conn, req Request) {
	intent, _ := req.Args["intent"].(string)
	if intent == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.intent required"})
		return
	}
	model, _ := req.Args["model"].(string)

	cfg := planner.Config{Provider: s.k.Provider(), Model: model}
	if cfg.Provider == nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "daemon has no provider configured"})
		return
	}

	rawJSON, plan, err := planner.Generate(ctx, cfg, intent)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			"plan_json":  rawJSON,
			"node_count": len(plan.Nodes),
		},
	})
}

// handlePlanRefine calls planner.Refine with the operator-supplied
// original plan JSON + feedback, and returns the validated
// replacement plan (M1.uu).
//
// The operator workflow is two-step on purpose: they review the
// generated plan, see something they want to change, write a
// sentence describing the change, and ask for a revision. This
// keeps a human in the loop on every plan revision — no automated
// LLM-to-LLM cascades.
//
// Args:
//
//	plan_json (string, required) — the existing plan to refine
//	feedback  (string, required) — natural-language change request
//	model     (string, optional) — override the provider's default
//
// Returns (RespResult):
//
//	plan_json  (string) — the validated replacement plan JSON
//	node_count (int)    — number of nodes in the replacement
func (s *Server) handlePlanRefine(ctx context.Context, conn net.Conn, req Request) {
	planJSON, _ := req.Args["plan_json"].(string)
	if planJSON == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.plan_json required"})
		return
	}
	feedback, _ := req.Args["feedback"].(string)
	if feedback == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.feedback required"})
		return
	}
	model, _ := req.Args["model"].(string)

	var original planner.Plan
	if err := json.Unmarshal([]byte(planJSON), &original); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "plan_json: " + err.Error()})
		return
	}

	cfg := planner.Config{Provider: s.k.Provider(), Model: model}
	if cfg.Provider == nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "daemon has no provider configured"})
		return
	}

	rawJSON, revised, err := planner.Refine(ctx, cfg, original, feedback)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: err.Error()})
		return
	}
	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			"plan_json":  rawJSON,
			"node_count": len(revised.Nodes),
		},
	})
}

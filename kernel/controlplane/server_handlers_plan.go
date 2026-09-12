// SPDX-License-Identifier: MIT

// Control-plane plan submission + decide handler.
// Code extracted from server_handlers.go during the Day-80 god-file split.
// Public API unchanged.
package controlplane


import (
	"context"
	"fmt"
	"net"
	"strings"

	"encoding/json"
	"github.com/agezt/agezt/kernel/approval"
	intentmodel "github.com/agezt/agezt/kernel/intent"
	"github.com/agezt/agezt/kernel/scheduler"
)

// planSpec is the JSON shape a `agt plan` client submits. It's a thin
// wire shape that the server reifies into scheduler.Plan with the
// kernel's wired LoopRunner + Approvals registry.
type planSpec struct {
	Name        string             `json:"name"`
	MaxParallel int                `json:"max_parallel"`
	Intent      *intentmodel.Frame `json:"intent,omitempty"`
	Nodes       []planNodeSpec     `json:"nodes"`
}

type planNodeSpec struct {
	ID   string   `json:"id"`
	Kind string   `json:"kind"`
	Deps []string `json:"deps,omitempty"`
	// Loop fields.
	Intent string `json:"intent,omitempty"`
	// Gate fields.
	Capability  string `json:"capability,omitempty"`
	Description string `json:"description,omitempty"`
}

func (s *Server) handlePlan(ctx context.Context, conn net.Conn, req Request) {
	rawAny, ok := req.Args["plan_json"]
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.plan_json required (JSON string)"})
		return
	}
	rawStr, ok := rawAny.(string)
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.plan_json must be a JSON string"})
		return
	}
	var spec planSpec
	if err := json.Unmarshal([]byte(rawStr), &spec); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "plan_json parse: " + err.Error()})
		return
	}
	if len(spec.Nodes) == 0 {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "plan has no nodes"})
		return
	}

	runner := s.k.LoopRunner()
	apr := s.k.Approvals()
	nodes := make([]scheduler.Node, 0, len(spec.Nodes))
	for _, ns := range spec.Nodes {
		switch ns.Kind {
		case "loop":
			nodes = append(nodes, &scheduler.LoopNode{
				NodeID: ns.ID, Deps: ns.Deps, Intent: ns.Intent, Runner: runner, IntentFrame: spec.Intent,
			})
		case "gate":
			nodes = append(nodes, &scheduler.GateNode{
				NodeID: ns.ID, Deps: ns.Deps, Approvals: apr,
				Capability: ns.Capability, Description: ns.Description, IntentFrame: spec.Intent,
			})
		default:
			s.writeResp(conn, Response{ID: req.ID, Type: RespError,
				Error: fmt.Sprintf("node %q: unknown kind %q (want loop|gate)", ns.ID, ns.Kind)})
			return
		}
	}
	plan := scheduler.Plan{
		Name:        spec.Name,
		MaxParallel: spec.MaxParallel,
		Nodes:       nodes,
	}

	planID := "plan-" + strings.TrimPrefix(req.ID, "q")
	if planID == "plan-" {
		planID = ""
	}

	// Subscribe to per-plan events before launching so the client
	// sees plan.started + every node.* event in order.
	subjectPrefix := "plan."
	sub, err := s.k.Bus().Subscribe(subjectPrefix+">", 1024)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	defer sub.Cancel()

	type planResult struct {
		res *scheduler.PlanResult
		err error
	}
	resultCh := make(chan planResult, 1)
	go func() {
		r, err := s.k.RunPlan(ctx, plan, planID)
		resultCh <- planResult{r, err}
	}()

	for {
		select {
		case ev, ok := <-sub.C:
			if !ok {
				s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "event subscription closed"})
				return
			}
			s.writeResp(conn, Response{ID: req.ID, Type: RespEvent, Event: ev})
		case r := <-resultCh:
			// Drain in-flight events.
			drain := true
			for drain {
				select {
				case ev := <-sub.C:
					if ev == nil {
						drain = false
					} else {
						s.writeResp(conn, Response{ID: req.ID, Type: RespEvent, Event: ev})
					}
				default:
					drain = false
				}
			}
			if r.err != nil {
				s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: r.err.Error()})
				return
			}
			outputs := map[string]any{}
			for id, nr := range r.res.NodeResults {
				outputs[id] = nr.Output
			}
			s.writeResp(conn, Response{
				ID: req.ID, Type: RespResult,
				Result: map[string]any{
					"plan_id":      r.res.PlanID,
					"node_outputs": outputs,
				},
			})
			return
		case <-ctx.Done():
			return
		}
	}
}

func (s *Server) handleDecide(conn net.Conn, req Request) {
	idAny := req.Args["id"]
	id, _ := idAny.(string)
	decAny := req.Args["decision"]
	dec, _ := decAny.(string)
	reasonAny := req.Args["reason"]
	reason, _ := reasonAny.(string)

	if id == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.id required"})
		return
	}
	var decision approval.Decision
	switch dec {
	case "grant":
		decision = approval.DecisionGrant
	case "deny":
		decision = approval.DecisionDeny
	default:
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: `args.decision must be "grant" or "deny"`})
		return
	}
	if err := s.k.Approvals().Resolve(id, decision, reason, "operator"); err != nil {
		s.fail(conn, req, err)
		return
	}
	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"ok": true, "id": id, "decision": dec},
	})
}

// SetCancelOnDisconnect enables/disables cancelling a streaming run when its
// client connection drops (M35). Called once at startup by the daemon when
// AGEZT_CANCEL_ON_DISCONNECT=on. Off by default.

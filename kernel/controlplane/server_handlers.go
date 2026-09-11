// SPDX-License-Identifier: MIT

package controlplane

// Server small handlers + connection plumbing: handleConn (per-conn
// request loop), writeResp/recoverConn (response write + panic recovery),
// handleVersion/Halt/CancelRun/Resume/Why/Whoami/Verify/Approvals/Decide
// (one-liner command handlers). Carved out of server.go during the Day
// 27 god file split #2 so the main file can focus on lifecycle + handleRun.

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"
	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/approval"
	intentmodel "github.com/agezt/agezt/kernel/intent"
	"github.com/agezt/agezt/kernel/scheduler"
)

func (s *Server) handleConn(ctx context.Context, conn net.Conn) {
	defer conn.Close()
	// Contain a panic ANYWHERE in this connection's handling — including the
	// pre-auth read/parse phase below — to THIS connection: an unrecovered panic
	// in this goroutine crashes the whole process, taking down every in-flight run
	// and channel. A single malformed/edge-case request must not be a daemon-wide
	// DoS — mirror net/http's per-request recover for the control plane's custom
	// TCP protocol. Deferred before parsing; recoverConn reads req.ID at panic time
	// (empty before the request is parsed). Must be deferred directly so its
	// recover() takes effect.
	var req Request
	defer s.recoverConn(conn, &req)

	// Generous read deadline per request — runs can take minutes.
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Minute))

	reader := bufio.NewReader(conn)
	// Bounded read (M188): the request line is read BEFORE authentication
	// (the token is inside it), so any local client that can reach the
	// loopback port can stream bytes here. A plain ReadBytes('\n') grows
	// without limit until a newline, so a client that never sends one
	// drives the daemon to OOM — a pre-auth DoS. Cap the request and
	// reject anything larger.
	line, err := readBoundedLine(reader, maxRequestBytes)
	if err != nil {
		if errors.Is(err, errRequestTooLarge) {
			s.writeResp(conn, Response{Type: RespError, Error: "request too large"})
		}
		return
	}
	if err := json.Unmarshal(line, &req); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "bad request: " + err.Error()})
		return
	}
	// Authentication + authorization (M38). The primary (admin) token
	// authorizes everything, on any tenant. Otherwise the request must name a
	// tenant AND present that tenant's own token: the principal is then that
	// tenant, restricted to the TenantAllowed subset of the command registry
	// and pinned to its own tenant. This completes M14 tenant isolation on the
	// control side — a tenant manages its own runs/policy without the primary
	// token, and cannot touch another tenant or daemon-global state.
	primary := s.tokenIsPrimary(req.Token)
	var tenantID string
	if !primary {
		reqTenant, _ := req.Args["tenant"].(string)
		reqTenant = strings.TrimSpace(reqTenant)
		if s.tenants == nil || reqTenant == "" || !s.tenants.Authorize(reqTenant, req.Token) {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unauthorized"})
			return
		}
		tenantID = reqTenant
	}

	// Command lookup happens AFTER the auth gate, and the tenant allowlist is
	// applied BEFORE unknown commands are distinguished: a tenant token probing
	// an unregistered command gets the same deny-by-default "forbidden" the
	// legacy tenantTokenAllows gate produced, not an "unknown command" oracle.
	spec, known := commandRegistry[req.Cmd]
	if !primary {
		if !known || !spec.TenantAllowed {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError,
				Error: fmt.Sprintf("forbidden: a tenant token cannot run %q (primary token required)", req.Cmd)})
			return
		}
		// Pin the tenant arg to the authorized tenant (defense in depth — it
		// already matched, but no handler should ever see a different value).
		if req.Args == nil {
			req.Args = map[string]any{}
		}
		req.Args["tenant"] = tenantID
	}
	if !known {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown command: " + req.Cmd})
		return
	}

	dc := &DispatchCtx{Ctx: ctx, Conn: conn, Req: req, S: s, K: s.k, Tenant: tenantID, Primary: primary}
	if spec.TenantRouted {
		// Resolve the request's kernel once at the dispatch boundary. For a
		// primary caller without a tenant arg kernelFor("") == s.k, so
		// tenant-routed commands cost nothing extra in the common case.
		k, err := s.kernelFor(tenantOf(req))
		if err != nil {
			s.fail(conn, req, err)
			return
		}
		dc.K = k
	}
	if spec.Streaming == StreamLive {
		// Long-lived streaming work: tie the handler's context to the client
		// connection so a disconnect cancels the (model-heavy) work instead of
		// spending it into a closed connection. Handlers receive the tied
		// context as their ctx parameter and must NOT call cancelOnConnClose
		// themselves — two goroutines reading one conn would race.
		cctx, cancel := cancelOnConnClose(ctx, conn)
		defer cancel()
		dc.Ctx = cctx
	}
	spec.Handler(dc)
}

func (s *Server) writeResp(conn net.Conn, resp Response) {
	_ = writeResp(conn, resp)
}

// recoverConn is deferred per connection: it turns a panic ANYWHERE in the
// connection's handling — the pre-auth parse phase as well as command handling —
// into an error response to the caller instead of an unrecovered goroutine panic
// that would crash the daemon. It takes a *Request (not an id) and reads the id
// at panic time so it can be deferred at the very top of handleConn, before the
// request has been parsed; an early panic simply carries an empty id. Best-effort
// — if the connection is already broken the error write is dropped; the
// load-bearing guarantee is that the process survives so other connections, runs,
// and channels keep working. recoverConn must be deferred DIRECTLY (not wrapped in
// a closure) so its recover() actually stops the panic.
func (s *Server) recoverConn(conn net.Conn, req *Request) {
	if r := recover(); r != nil {
		var id string
		if req != nil {
			id = req.ID
		}
		s.writeResp(conn, Response{ID: id, Type: RespError, Error: "internal error"})
	}
}

// writeResp is the underlying writer that *returns* its error.
// Used by long-lived handlers (pulse) where a broken pipe is the
// client-disconnect signal that should stop the goroutine; the
// method form above keeps the fire-and-forget shape every other
// handler relies on.
func writeResp(conn net.Conn, resp Response) error {
	enc, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	enc = append(enc, '\n')
	_ = conn.SetWriteDeadline(time.Now().Add(30 * time.Second))
	_, err = conn.Write(enc)
	return err
}

// ----- command handlers -----

func (s *Server) handleVersion(conn net.Conn, req Request) {
	rev, committed, modified := brand.BuildInfo()
	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			brand.Binary:       brand.Version,
			"protocol_version": brand.ProtocolVersion,
			// Build provenance (M971) — lets operators confirm which build a
			// daemon is actually running, since the semver only moves per release.
			"revision":       rev,
			"built":          committed,
			"build_modified": modified,
		},
	})
}

func (s *Server) handleHalt(conn net.Conn, req Request) {
	reason, _, err := argString(req.Args, "reason")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	s.k.HaltWith(reason)
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"ok":     true,
		"halted": true,
		"reason": reason,
	}})
}

// handleCancelRun cancels a single in-flight run by correlation id (M32),
// leaving the kernel un-halted and other runs untouched — the targeted
// alternative to the global halt. Routes to the tenant kernel when a
// tenant is named (empty → primary), mirroring handleRun.
func (s *Server) handleCancelRun(conn net.Conn, req Request) {
	corr, err := requiredArgString(req.Args, "correlation")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	tenantID, _, err := argString(req.Args, "tenant")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	k, err := s.kernelFor(tenantID)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	cancelled := k.CancelRun(corr)
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"correlation": corr,
		"cancelled":   cancelled,
	}})
}

func (s *Server) handleResume(conn net.Conn, req Request) {
	reason, _, err := argString(req.Args, "reason")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	s.k.ResumeWith(reason)
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"ok":     true,
		"halted": false,
		"reason": reason,
	}})
}

func (s *Server) handleWhy(conn net.Conn, req Request) {
	idAny := req.Args["event_id"]
	id, _ := idAny.(string)
	if id == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.event_id required"})
		return
	}
	// Tenant-scoped (M53): an empty tenant traces the primary journal; a named
	// tenant traces its own isolated journal, so a tenant walks only its own
	// events — completing tenant isolation on the observability surface (M39
	// did runs list/stats; this does why).
	k, err := s.kernelFor(tenantOf(req))
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	events, err := k.Why(id)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	out := make([]any, 0, len(events))
	for _, e := range events {
		out = append(out, e)
	}
	// Parent backlink (M42): if this chain belongs to a sub-agent run,
	// surface its lead's correlation so an operator can walk child→parent
	// (only parent→child was visible before). correlation is the chain's
	// shared id; parent_correlation is "" for top-level runs.
	corr := ""
	if len(events) > 0 {
		corr = events[0].CorrelationID
	}
	parent := ""
	if corr != "" {
		parent = k.ParentOf(corr)
	}
	// Causation provenance (SPEC-01 §7.1): the chain of events linked by
	// causation_id from the root cause down to this one, ordered oldest-first.
	// Unlike the correlation grouping above, this crosses correlation
	// boundaries — e.g. a Pulse initiative back to its originating tick, which
	// carries a different correlation and is therefore absent from `events`.
	// Best-effort: a failure here must not sink the whole why response.
	causation := make([]any, 0, 4)
	if chain, cErr := k.Causes(id); cErr == nil && len(chain) > 1 {
		for _, e := range chain {
			causation = append(causation, e)
		}
	}
	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			"events":             out,
			"correlation":        corr,
			"parent_correlation": parent,
			"causation_chain":    causation,
		},
	})
}

// handleWhoami reports the authenticated principal (M62). By the time a request
// reaches a handler, handleConn has already verified the token: the primary
// token equals s.Token(); any other token that got here authenticated as the
// tenant named in (and pinned to) req.Args["tenant"]. So identity is a pure
// read of req.Token vs the primary token — no new auth state.
func (s *Server) handleWhoami(conn net.Conn, req Request) {
	if s.tokenIsPrimary(req.Token) {
		s.writeResp(conn, Response{
			ID:   req.ID,
			Type: RespResult,
			Result: map[string]any{
				"identity": "primary",
				"primary":  true,
				"tenant":   "",
			},
		})
		return
	}
	tenant, _ := req.Args["tenant"].(string)
	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			"identity": "tenant",
			"primary":  false,
			"tenant":   tenant,
		},
	})
}

func (s *Server) handleVerify(conn net.Conn, req Request) {
	if err := s.k.Verify(); err != nil {
		s.fail(conn, req, err)
		return
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"ok": true}})
}

func (s *Server) handleApprovals(conn net.Conn, req Request) {
	pending := s.k.Approvals().Pending()
	out := make([]map[string]any, 0, len(pending))
	for _, p := range pending {
		out = append(out, map[string]any{
			"id":                     p.ID,
			"capability":             p.Capability,
			"tool_name":              p.ToolName,
			"input":                  p.Input,
			"reason":                 p.Reason,
			"actor":                  p.Actor,
			"correlation_id":         p.CorrelationID,
			"created_unix":           p.CreatedAt.Unix(),
			"timeout_unix":           p.Timeout.Unix(),
			"effect_class":           p.EffectClass,
			"predicted_effects":      p.PredictedEffects,
			"affected_resources":     p.AffectedResources,
			"rollback_notes":         p.RollbackNotes,
			"confidence":             p.Confidence,
			"canonical_intent":       p.CanonicalIntent,
			"harmful_interpretation": p.HarmfulInterpretation,
			"ambiguity_score":        p.AmbiguityScore,
			"regret_axes":            p.RegretAxes,
			"confirmation_prompt":    p.ConfirmationPrompt,
		})
	}
	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"pending": out, "count": len(out)},
	})
}

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

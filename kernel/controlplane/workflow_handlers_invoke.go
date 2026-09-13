// SPDX-License-Identifier: MIT

// Package controlplane: Workflow test/invoke handlers + their timeouts
// (workflowTestNodeTimeout + workflowWebhookReplyTimeout consts +
// handleWorkflowTestNode + handleWorkflowWebhook + runWorkflowDetached).
// Extracted from workflow_handlers.go during the Day-211 god-file split.
// Public API unchanged.
package controlplane


import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/event"
	kernelruntime "github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/kernel/workflow"
)
// workflowTestNodeTimeout bounds one single-node probe — a node's own
// timeout_sec applies inside it; this is the outer hard stop.
const workflowTestNodeTimeout = 3 * time.Minute

// workflowWebhookReplyTimeout bounds one SYNC webhook run (M812) — reply
// workflows are request/response, not pipelines; long work stays async.
const workflowWebhookReplyTimeout = 2 * time.Minute

// handleWorkflowTestNode (M811) runs ONE node of the POSTED graph with
// caller-supplied upstream data — the canvas's "Test node" button. The graph
// rides in the request (the canvas's truth, unsaved edits included).
func (s *Server) handleWorkflowTestNode(conn net.Conn, req Request) {
	raw, ok := req.Args["workflow"]
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.workflow required"})
		return
	}
	b, err := json.Marshal(raw)
	if err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.workflow: " + err.Error()})
		return
	}
	var w workflow.Workflow
	if err := json.Unmarshal(b, &w); err != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.workflow: " + err.Error()})
		return
	}
	nodeID, err := requiredArgString(req.Args, "node")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	var data map[string]any
	if raw, ok := req.Args["data"]; ok {
		m, isMap := raw.(map[string]any)
		if !isMap {
			s.failMsg(conn, req, "args.data must be an object")
			return
		}
		data = m
	}
	payload := req.Args["payload"]
	corr := s.k.NewCorrelation()
	ctx, cancel := context.WithTimeout(context.Background(), workflowTestNodeTimeout)
	defer cancel()
	res, err := s.k.TestWorkflowNode(ctx, corr, w, nodeID, data, payload)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	s.writeResp(conn, Response{
		ID:   req.ID,
		Type: RespResult,
		Result: map[string]any{
			"output": res.Output, "port": res.Port, "attempts": res.Attempts,
			"correlation_id": corr,
		},
	})
}

// handleWorkflowWebhook (M809) authenticates an external webhook POST and
// fires the workflow ASYNC. The gate is strict and entirely here, the single
// source of truth: the workflow must exist, be ENABLED, declare a webhook
// trigger, and the presented secret must match in constant time. Refusals
// are deliberately uniform ("webhook refused") so a probing caller cannot
// distinguish unknown-name from bad-secret from disabled.
func (s *Server) handleWorkflowWebhook(conn net.Conn, req Request) {
	refuse := func() {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "webhook refused"})
	}
	// Wrong-typed args refuse uniformly too — the gate must not leak which
	// input was malformed.
	ref, _, refErr := argString(req.Args, "ref")
	secret, _, secErr := argString(req.Args, "secret")
	if refErr != nil || secErr != nil || strings.TrimSpace(ref) == "" || secret == "" {
		refuse()
		return
	}
	w, found := s.k.Workflows().Get(strings.TrimSpace(ref))
	if !found || !w.Enabled {
		refuse()
		return
	}
	spec := w.TriggerSpec()
	if spec.Kind != "webhook" ||
		subtle.ConstantTimeCompare([]byte(spec.Secret), []byte(secret)) != 1 {
		refuse()
		return
	}
	corr := s.k.NewCorrelation()
	payload := req.Args["payload"]

	// Reply mode (M812): the AUTHENTICATED caller holds the line and gets
	// the run's outputs back — n8n's "respond to webhook". Post-auth run
	// failures may be honest (the caller proved knowledge of the secret);
	// only the auth gate stays uniform.
	if w.TriggerSpec().Reply {
		ctx, cancel := context.WithTimeout(context.Background(), workflowWebhookReplyTimeout)
		defer cancel()
		ctx = kernelruntime.WithWakeContext(ctx, kernelruntime.WakeContext{Source: "webhook", TriggerSubject: "webhook:" + w.Name})
		runRes, err := s.k.RunWorkflow(ctx, corr, w.Name, payload)
		if err != nil {
			s.writeResp(conn, Response{
				ID: req.ID, Type: RespError,
				Error: "webhook run failed: " + err.Error() + " (correlation " + corr + ")",
			})
			return
		}
		s.writeResp(conn, Response{
			ID:   req.ID,
			Type: RespResult,
			Result: map[string]any{
				"correlation_id": corr, "workflow": w.Name,
				"executed": runRes.Executed, "outputs": runRes.Outputs,
			},
		})
		return
	}

	// Fire-and-return: a webhook caller gets an immediate accept; the run
	// proceeds under its own deadline and the journal carries the arc.
	go s.runWorkflowDetached(kernelruntime.WakeContext{Source: "webhook", TriggerSubject: "webhook:" + w.Name}, corr, w.Name, payload)
	s.writeResp(conn, Response{
		ID:     req.ID,
		Type:   RespResult,
		Result: map[string]any{"accepted": true, "correlation_id": corr, "workflow": w.Name},
	})
}

// runWorkflowDetached runs a workflow on its own deadline, disconnected from the
// caller's connection: the webhook and async-run handlers both answer "accepted"
// immediately and let the journal carry the arc.
//
// Panic firewall (WF-001). These are the two paths that reach the engine on a
// bare `go` WITHOUT passing through the trigger runner's safeFire, so they had no
// recover of their own — and a workflow executes third-party code (plugin
// subprocesses, MCP servers, scripts), so one bad node took the daemon down. The
// caller has already been answered by the time this runs, which is exactly why
// the panic must be journaled: there is no request left to return an error on.
func (s *Server) runWorkflowDetached(wake kernelruntime.WakeContext, corr, name string, payload any) {
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		fmt.Fprintf(os.Stderr, "workflow %q (%s) panicked: %v\n", name, wake.Source, r)
		if s.k == nil || s.k.Bus() == nil {
			return
		}
		_, _ = s.k.Bus().Publish(event.Spec{
			Subject:       "workflow." + name,
			Kind:          event.KindWorkflowPanic,
			Actor:         "controlplane",
			CorrelationID: corr,
			Payload: map[string]any{
				"workflow": name,
				"source":   wake.Source,
				"panic":    fmt.Sprintf("%v", r),
			},
		})
	}()
	ctx, cancel := context.WithTimeout(context.Background(), workflowRunTimeout)
	defer cancel()
	ctx = kernelruntime.WithWakeContext(ctx, wake)
	_, _ = s.k.RunWorkflow(ctx, corr, name, payload) // failures land in workflow.failed
}

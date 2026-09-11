// SPDX-License-Identifier: MIT

package controlplane

// Agent wake + resolve handlers + the operator-wake lineage bookkeeping
// (M833/M846). Carved out of roster.go during the Day 24 god file split
// #7 so the main file can focus on escalation/lifecycle.

import (
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/roster"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/tools/overseertool"
)

func (s *Server) handleAgentWake(conn net.Conn, req Request) {
	ref, err := requiredArgString(req.Args, "ref")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	p, ok := s.k.Roster().Get(ref)
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown agent: " + ref})
		return
	}
	if p.Retired {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "agent " + p.Slug + " is retired — revive it first"})
		return
	}
	if !p.Enabled {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "agent " + p.Slug + " is paused"})
		return
	}
	if !p.AllowsDirectCall() {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: managedSubagentDirectCallError(p, "called")})
		return
	}
	intent, _, ierr := argString(req.Args, "intent")
	if ierr != nil {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: ierr.Error()})
		return
	}
	reason := strings.TrimSpace(stringArg(req.Args, "reason"))
	intent = buildOperatorWakeIntent(strings.TrimSpace(intent), p.Slug, reason, req.Args)
	if strings.TrimSpace(intent) == "" {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "agent wake requires args.intent or args.reason"})
		return
	}
	corr := s.k.NewCorrelation()
	lineage := operatorIncidentLineage(req.Args)
	runbook := agentAutonomyRunbookPayload(p)
	publishOperatorAction(s.k, "agent.wake", corr, map[string]any{
		"phase":              "requested",
		"agent":              p.Slug,
		"reason":             reason,
		"intent":             truncate(intent, 240),
		"autonomy_runbook":   runbook,
		"incident_id":        lineage.incidentID,
		"root_incident_id":   lineage.rootIncidentID,
		"parent_incident_id": lineage.parentIncidentID,
	})
	go s.runAgentWake(corr, p, intent, reason, lineage)
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"accepted":       true,
		"agent":          p.Slug,
		"correlation_id": corr,
	}})
}

func (s *Server) handleAgentResolve(conn net.Conn, req Request) {
	ref, err := requiredArgString(req.Args, "ref")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	p, ok := s.k.Roster().Get(ref)
	if !ok {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown agent: " + ref})
		return
	}
	resolution := strings.TrimSpace(stringArg(req.Args, "resolution"))
	switch resolution {
	case "paused", "retired", "delegated", "force_chain":
	default:
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "args.resolution must be paused, retired, delegated, or force_chain"})
		return
	}
	summary := strings.TrimSpace(stringArg(req.Args, "summary"))
	lineage := operatorIncidentLineage(req.Args)
	corr := s.k.NewCorrelation()
	requested := map[string]any{
		"phase":              "requested",
		"agent":              p.Slug,
		"resolution":         resolution,
		"resolution_summary": summary,
		"incident_id":        lineage.incidentID,
		"root_incident_id":   lineage.rootIncidentID,
		"parent_incident_id": lineage.parentIncidentID,
	}
	if delegateTo := strings.TrimSpace(stringArg(req.Args, "delegate_to")); delegateTo != "" {
		requested["delegate_to"] = delegateTo
	}
	if taskType := strings.TrimSpace(stringArg(req.Args, "task_type")); taskType != "" {
		requested["routing_task_type"] = taskType
	}
	if chain, ok := req.Args["task_model_chain"].([]any); ok && len(chain) > 0 {
		requested["routing_task_model_chain"] = normalizeTaskModelChain(chain)
	}
	publishOperatorAction(s.k, "agent.resolve", corr, requested)

	result, err := s.applyAgentResolution(p, resolution, summary, req.Args)
	if err != nil {
		fail := map[string]any{
			"phase":              "failed",
			"agent":              p.Slug,
			"resolution":         resolution,
			"resolution_summary": summary,
			"reason":             err.Error(),
			"incident_id":        lineage.incidentID,
			"root_incident_id":   lineage.rootIncidentID,
			"parent_incident_id": lineage.parentIncidentID,
		}
		if result.delegateTo != "" {
			fail["delegate_to"] = result.delegateTo
		}
		if result.taskType != "" {
			fail["routing_task_type"] = result.taskType
		}
		if len(result.taskModelChain) > 0 {
			fail["routing_task_model_chain"] = result.taskModelChain
		}
		publishOperatorAction(s.k, "agent.resolve", corr, fail)
		s.fail(conn, req, err)
		return
	}
	completed := map[string]any{
		"phase":              "completed",
		"agent":              p.Slug,
		"resolution":         resolution,
		"resolution_summary": summary,
		"incident_id":        lineage.incidentID,
		"root_incident_id":   lineage.rootIncidentID,
		"parent_incident_id": lineage.parentIncidentID,
	}
	if result.delegateTo != "" {
		completed["delegate_to"] = result.delegateTo
	}
	if result.messageID != "" {
		completed["message_id"] = result.messageID
	}
	if result.taskType != "" {
		completed["routing_task_type"] = result.taskType
	}
	if len(result.taskModelChain) > 0 {
		completed["routing_task_model_chain"] = result.taskModelChain
	}
	if len(result.previousTaskModelChain) > 0 {
		completed["previous_routing_task_model_chain"] = result.previousTaskModelChain
	}
	if result.routingForceGeneration > 0 {
		completed["routing_force_generation"] = result.routingForceGeneration
	}
	if result.previousRoutingForceGeneration > 0 {
		completed["previous_routing_force_generation"] = result.previousRoutingForceGeneration
	}
	publishOperatorAction(s.k, "agent.resolve", corr, completed)
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"applied":        true,
		"agent":          p.Slug,
		"resolution":     resolution,
		"correlation_id": corr,
	}})
}

type appliedAgentResolution struct {
	delegateTo                     string
	messageID                      string
	taskType                       string
	taskModelChain                 []string
	previousTaskModelChain         []string
	routingForceGeneration         int
	previousRoutingForceGeneration int
}

type routingChainApplier interface {
	ApplyRoutingChain(ref, taskType string, targetChain []string, reason string) (overseertool.RepairResult, error)
}

func (s *Server) applyAgentResolution(p roster.Profile, resolution, summary string, args map[string]any) (appliedAgentResolution, error) {
	switch resolution {
	case "paused":
		if p.Retired {
			return appliedAgentResolution{}, fmt.Errorf("agent %s is retired — revive it first", p.Slug)
		}
		_, err := s.k.SetProfileEnabled(p.Slug, false)
		return appliedAgentResolution{}, err
	case "retired":
		reason := summary
		if reason == "" {
			reason = "retired by operator incident resolution"
		}
		_, err := s.k.SetProfileRetired(p.Slug, true, reason)
		return appliedAgentResolution{}, err
	case "delegated":
		target := strings.TrimSpace(stringArg(args, "delegate_to"))
		if err := s.validateOperatorDelegateTarget(p, target); err != nil {
			return appliedAgentResolution{}, err
		}
		st, ok := s.boardWriter()
		if !ok {
			return appliedAgentResolution{}, fmt.Errorf("the board is not available on this daemon")
		}
		text := strings.TrimSpace(summary)
		if text == "" {
			text = "Operator delegated this incident for ownership review."
		}
		msg, err := st.HelpRequest("operator", target, text, time.Now().UnixMilli())
		if err != nil {
			return appliedAgentResolution{}, err
		}
		if s.boardNotify != nil {
			s.boardNotify(msg, "")
		}
		return appliedAgentResolution{delegateTo: target, messageID: strings.TrimSpace(msg.ID)}, nil
	case "force_chain":
		taskType := strings.TrimSpace(stringArg(args, "task_type"))
		chain := normalizeTaskModelChain(argListAny(args["task_model_chain"]))
		if taskType == "" || len(chain) == 0 {
			return appliedAgentResolution{}, fmt.Errorf("force_chain resolution requires task_type and task_model_chain")
		}
		if exhausted := latestExhaustedRoutingChain(s.k, p.Slug, operatorIncidentLineage(args), taskType); len(exhausted) > 0 && equalStringSlices(exhausted, chain) {
			return appliedAgentResolution{}, fmt.Errorf("force_chain resolution must choose a new chain for exhausted routing policy")
		}
		src, ok := overseertool.NewKernelSource(s.k, s.baseDir).(routingChainApplier)
		if !ok {
			return appliedAgentResolution{}, fmt.Errorf("force_chain resolution is not supported by the active repair source")
		}
		prevGen := latestOperatorForceGeneration(s.k, p.Slug, taskType)
		res, err := src.ApplyRoutingChain(p.Slug, taskType, chain, summary)
		if err != nil {
			return appliedAgentResolution{}, err
		}
		return appliedAgentResolution{
			taskType:                       firstNonEmpty(res.RoutingTaskType, taskType),
			taskModelChain:                 append([]string(nil), firstNonEmptyStrings(res.RoutingTaskModelChain, chain)...),
			previousTaskModelChain:         append([]string(nil), res.PreviousRoutingTaskModelChain...),
			routingForceGeneration:         prevGen + 1,
			previousRoutingForceGeneration: prevGen,
		}, nil
	default:
		return appliedAgentResolution{}, nil
	}
}

func latestOperatorForceGeneration(k *runtime.Kernel, slug, taskType string) int {
	if k == nil || strings.TrimSpace(slug) == "" || strings.TrimSpace(taskType) == "" {
		return 0
	}
	best := 0
	_ = k.Journal().Range(func(e *event.Event) error {
		if e.Kind != event.KindInfo || (e.Subject != "doctor.auto_repair" && e.Subject != "agent.resolve") {
			return nil
		}
		var pl map[string]any
		if json.Unmarshal(e.Payload, &pl) != nil {
			return nil
		}
		if strings.TrimSpace(plString(pl, "agent")) != slug || strings.TrimSpace(plString(pl, "resolution")) != "force_chain" {
			return nil
		}
		phase := strings.TrimSpace(plString(pl, "phase"))
		if phase != "resolution_applied" && phase != "completed" {
			return nil
		}
		if strings.TrimSpace(plString(pl, "routing_task_type")) != taskType {
			return nil
		}
		if gen := intNumber(pl["routing_force_generation"]); gen > best {
			best = gen
		}
		return nil
	})
	return best
}

func (s *Server) validateOperatorDelegateTarget(p roster.Profile, target string) error {
	target = strings.TrimSpace(target)
	if target == "" {
		return fmt.Errorf("delegated resolution requires delegate_to")
	}
	if strings.EqualFold(target, strings.TrimSpace(p.Slug)) {
		return fmt.Errorf("delegated resolution points back to the root agent %s", p.Slug)
	}
	if owner := firstNonEmpty(p.ParentAgent, p.OwnerAgent); owner != "" && strings.EqualFold(target, owner) {
		return fmt.Errorf("delegated resolution points back to the current owner %s", owner)
	}
	dst, ok := s.k.Roster().Get(target)
	if !ok {
		return fmt.Errorf("delegated resolution target %s does not exist", target)
	}
	if dst.Retired {
		return fmt.Errorf("delegated resolution target %s is retired", dst.Slug)
	}
	if !dst.AllowsDirectCall() {
		return fmt.Errorf("delegated resolution target %s is a managed sub-agent", dst.Slug)
	}
	return nil
}

func latestExhaustedRoutingChain(k *runtime.Kernel, slug string, lineage operatorWakeLineage, taskType string) []string {
	if k == nil || strings.TrimSpace(slug) == "" || strings.TrimSpace(taskType) == "" || !lineage.hasAny() {
		return nil
	}
	var bestChain []string
	var bestSeq int64
	_ = k.Journal().Range(func(e *event.Event) error {
		if e.Kind != event.KindInfo || e.Subject != "doctor.auto_repair" {
			return nil
		}
		var pl map[string]any
		if json.Unmarshal(e.Payload, &pl) != nil {
			return nil
		}
		if !strings.EqualFold(strings.TrimSpace(plString(pl, "agent")), slug) {
			return nil
		}
		if strings.TrimSpace(plString(pl, "phase")) != "routing_force_exhausted_detected" {
			return nil
		}
		if !strings.EqualFold(strings.TrimSpace(plString(pl, "routing_task_type")), taskType) {
			return nil
		}
		if !incidentLineageMatchesPayload(lineage, pl) || e.Seq <= bestSeq {
			return nil
		}
		bestSeq = e.Seq
		bestChain = plStrings(pl, "routing_task_model_chain")
		return nil
	})
	return append([]string(nil), bestChain...)
}

func incidentLineageMatchesPayload(lineage operatorWakeLineage, pl map[string]any) bool {
	if !lineage.hasAny() {
		return false
	}
	payloadIDs := []string{
		strings.TrimSpace(plString(pl, "incident_id")),
		strings.TrimSpace(plString(pl, "root_incident_id")),
		strings.TrimSpace(plString(pl, "parent_incident_id")),
	}
	return incidentIDInSlice(lineage.incidentID, payloadIDs) ||
		incidentIDInSlice(lineage.rootIncidentID, payloadIDs) ||
		incidentIDInSlice(lineage.parentIncidentID, payloadIDs)
}

func incidentIDInSlice(id string, items []string) bool {
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	for _, item := range items {
		if strings.EqualFold(id, strings.TrimSpace(item)) {
			return true
		}
	}
	return false
}

func (l operatorWakeLineage) hasAny() bool {
	return strings.TrimSpace(l.incidentID) != "" ||
		strings.TrimSpace(l.rootIncidentID) != "" ||
		strings.TrimSpace(l.parentIncidentID) != ""
}

func argListAny(v any) []any {
	if raw, ok := v.([]any); ok {
		return raw
	}
	return nil
}

func normalizeTaskModelChain(raw []any) []string {
	if len(raw) == 0 {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		switch v := item.(type) {
		case string:
			if v = strings.TrimSpace(v); v != "" {
				out = append(out, v)
			}
		}
	}
	return out
}

func equalStringSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !strings.EqualFold(strings.TrimSpace(a[i]), strings.TrimSpace(b[i])) {
			return false
		}
	}
	return true
}


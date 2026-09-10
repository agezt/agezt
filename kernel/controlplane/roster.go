// SPDX-License-Identifier: MIT

package controlplane

// Agent roster CRUD handlers (M783) — the management path behind `agt agent`.
// Lifecycle changes go through the kernel so every create/edit/pause/resume/
// remove is journaled (roster.*) and auditable via `agt why`. Profiles are
// addressed by ref = id OR slug everywhere, so operators can say
// `agt agent show researcher` without copying ULIDs.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/agezt/agezt/internal/brand"
	"github.com/agezt/agezt/kernel/board"
	"github.com/agezt/agezt/kernel/event"
	"github.com/agezt/agezt/kernel/roster"
	"github.com/agezt/agezt/kernel/runtime"
	"github.com/agezt/agezt/plugins/tools/overseertool"
)

type agentRepairRow struct {
	Seq                            int64
	TSUnixMS                       int64
	Agent                          string
	CorrelationID                  string
	Mode                           string
	Phase                          string
	Reason                         string
	Fingerprint                    string
	SelfRepairAttempt              int
	SelfRepairMaxAttempts          int
	Issues                         []string
	Applied                        []string
	Answer                         string
	Error                          string
	TargetAgent                    string
	TargetCorr                     string
	MailboxMessage                 string
	Resolution                     string
	ResolutionSummary              string
	DelegateTo                     string
	DelegatedBy                    string
	RootAgent                      string
	ChainDepth                     int
	IncidentID                     string
	RootIncidentID                 string
	ParentIncidentID               string
	NextEligibleMS                 int64
	RoutingTaskType                string
	RoutingTaskModelChain          []string
	PreviousRoutingTaskModelChain  []string
	RoutingForceGeneration         int
	PreviousRoutingForceGeneration int
}

type agentEscalationRow struct {
	MessageID         string
	From              string
	To                string
	Text              string
	TSUnixMS          int64
	Status            string
	ReplyCount        int
	Acked             bool
	SourceAgent       string
	Mode              string
	WakePhase         string
	WakeReason        string
	WakeError         string
	WakeCorrelationID string
	Fingerprint       string
	Resolution        string
	ResolutionSummary string
	DelegateTo        string
	OriginKind        string
	OriginAgent       string
	RootAgent         string
	ChainDepth        int
	IncidentID        string
	RootIncidentID    string
	ParentIncidentID  string
}

type agentRepairSummary struct {
	Latest        agentRepairRow
	HasLatest     bool
	InflightCount int
}

type agentRoutingPressure struct {
	Count      int
	LastReason string
	LastFailed string
	LastNext   string
	LastTSMS   int64
}

type agentRetryPressure struct {
	Count       int
	LastReason  string
	LastTSMS    int64
	NextAttempt int
	MaxAttempts int
}

type agentEscalationLoad struct {
	Open  int
	Acked int
}

type agentWakeStatus struct {
	ScheduleCount       int
	StandingCount       int
	EventSubjects       []string
	NextScheduledWakeMS int64
	NextScheduledLabel  string
}

type agentLiveStatus struct {
	ActiveRuns              int
	ActiveCorrelationID     string
	ActiveIntent            string
	ActiveStartedMS         int64
	ActiveModel             string
	ActiveSpentMc           int64
	ActivePhase             string
	ActiveLastEventMS       int64
	ActiveLastEventKind     string
	ActiveDetail            string
	ActiveTool              string
	ActiveIter              int
	ActiveWakeSource        string
	ActiveWakeReason        string
	ActiveScheduleID        string
	ActiveStandingID        string
	ActiveStandingName      string
	ActiveTriggerSubject    string
	ActiveParentCorrelation string
}

type agentLastActivity struct {
	TSUnixMS      int64
	Kind          string
	CorrelationID string
	Summary       string
}


// agentStatusAccums holds the per-agent state that the journal-derivable
// helpers accumulate. Roster-agentList page is the single consumer; collecting
// every accumulator in a SINGLE journal.Range pass turns what used to be
// O(journalSize) per helper (and 11× that across all helpers) into one
// O(journalSize) walk. Large, busy journals were reliably tripping the
// control-plane connection's 10-minute read deadline under the previous
// 11-Range design (each Range does a callback-driven O(n) walk over every
// durable event, with JSON-unmarshal + map lookups per event). The
// single-pass dispatch below is behavior-preserving: each accumulator's
// key-set and "latest wins" semantics match the original per-helper
// implementations (the retired per-helper methods have been deleted; this
// dispatch is now the only journal-derived roster-status path).
func (s *Server) handleAgentImpact(conn net.Conn, req Request) {
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
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: s.agentImpactResult(p)})
}

// handleAgentTombstone returns a read-only death certificate for an agent: its
// identity, lifecycle/retirement record, and durable resource footprint. Portable
// archival/audit artifact — it removes and mutates nothing (NEXT.md #7).
func (s *Server) handleAgentRepair(conn net.Conn, req Request) {
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
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: managedSubagentDirectCallError(p, "repaired")})
		return
	}
	corr := s.k.NewCorrelation()
	reason := strings.TrimSpace(stringArg(req.Args, "reason"))
	lineage := operatorIncidentLineage(req.Args)
	publishOperatorAction(s.k, "agent.repair", corr, map[string]any{
		"phase":              "requested",
		"agent":              p.Slug,
		"reason":             reason,
		"incident_id":        lineage.incidentID,
		"root_incident_id":   lineage.rootIncidentID,
		"parent_incident_id": lineage.parentIncidentID,
	})
	go func() {
		// Panic firewall (WF-001). This is the operator's "Repair" button: it
		// answers "accepted" immediately and drives a full governed run — provider
		// calls, tools, plugin subprocesses — on a bare `go`, with nothing above it
		// able to recover. Missed in the original sweep because the goroutine is a
		// closure in a roster handler rather than in one of the runner packages.
		defer func() {
			r := recover()
			if r == nil {
				return
			}
			fmt.Fprintf(os.Stderr, "operator repair of %q panicked: %v\n", p.Slug, r)
			publishOperatorAction(s.k, "agent.repair", corr, map[string]any{
				"phase":              "failed",
				"agent":              p.Slug,
				"reason":             reason,
				"error":              fmt.Sprintf("repair panicked: %v", r),
				"incident_id":        lineage.incidentID,
				"root_incident_id":   lineage.rootIncidentID,
				"parent_incident_id": lineage.parentIncidentID,
			})
		}()
		src := overseertool.NewKernelSource(s.k, s.baseDir)
		res, err := src.RepairAgent(p.Slug, reason)
		if err != nil {
			publishOperatorAction(s.k, "agent.repair", corr, map[string]any{
				"phase":              "failed",
				"agent":              p.Slug,
				"reason":             reason,
				"error":              err.Error(),
				"incident_id":        lineage.incidentID,
				"root_incident_id":   lineage.rootIncidentID,
				"parent_incident_id": lineage.parentIncidentID,
			})
			return
		}
		publishOperatorAction(s.k, "agent.repair", corr, map[string]any{
			"phase":                             "completed",
			"agent":                             p.Slug,
			"reason":                            reason,
			"applied":                           res.Applied,
			"routing_task_type":                 res.RoutingTaskType,
			"routing_task_model_chain":          res.RoutingTaskModelChain,
			"previous_routing_task_model_chain": res.PreviousRoutingTaskModelChain,
			"answer":                            truncate(res.Answer, 300),
			"incident_id":                       lineage.incidentID,
			"root_incident_id":                  lineage.rootIncidentID,
			"parent_incident_id":                lineage.parentIncidentID,
		})
	}()
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"accepted":       true,
		"agent":          p.Slug,
		"correlation_id": corr,
	}})
}

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

func (s *Server) handleAgentEscalations(conn net.Conn, req Request) {
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
	limit, err := argLimit(req.Args, 20, 100)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	st, err := s.boardReader()
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	// Cursor pagination (M-pending follow-up): `cursor` is the opaque
	// "<ts_unix_ms>:<message_id>" boundary of the previous page; server skips
	// entries strictly newer-or-equal. ts can collide across messages, so the
	// message_id is the tie-break.
	var cursorTS int64
	var cursorID string
	cursorOK := false
	if raw, _, cerr := argString(req.Args, "cursor"); cerr != nil {
		s.fail(conn, req, cerr)
		return
	} else if raw != "" {
		tsStr, id, _ := strings.Cut(raw, ":")
		if ts, perr := strconv.ParseInt(tsStr, 10, 64); perr == nil {
			cursorTS, cursorID, cursorOK = ts, id, true
		}
	}
	if !cursorOK {
		cursorTS, cursorID = 0, ""
	}
	rows, nextCursor := s.agentEscalationRows(st, p.Slug, limit, cursorTS, cursorID)
	openCount := 0
	for _, row := range rows {
		if row.Status == "open" {
			openCount++
		}
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, map[string]any{
			"message_id":          row.MessageID,
			"from":                row.From,
			"to":                  row.To,
			"text":                row.Text,
			"ts_unix_ms":          row.TSUnixMS,
			"status":              row.Status,
			"reply_count":         row.ReplyCount,
			"acked":               row.Acked,
			"source_agent":        row.SourceAgent,
			"mode":                row.Mode,
			"wake_phase":          row.WakePhase,
			"wake_reason":         row.WakeReason,
			"wake_error":          row.WakeError,
			"wake_correlation_id": row.WakeCorrelationID,
			"fingerprint":         row.Fingerprint,
			"resolution":          row.Resolution,
			"resolution_summary":  row.ResolutionSummary,
			"delegate_to":         row.DelegateTo,
			"origin_kind":         row.OriginKind,
			"origin_agent":        row.OriginAgent,
			"root_agent":          row.RootAgent,
			"chain_depth":         row.ChainDepth,
			"incident_id":         row.IncidentID,
			"root_incident_id":    row.RootIncidentID,
			"parent_incident_id":  row.ParentIncidentID,
		})
	}
	result := map[string]any{
		"slug":        p.Slug,
		"escalations": out,
		"count":       len(out),
		"open_count":  openCount,
	}
	if nextCursor != "" {
		result["next_cursor"] = nextCursor
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: result})
}

type operatorWakeLineage struct {
	incidentID       string
	rootIncidentID   string
	parentIncidentID string
}

func operatorIncidentLineage(args map[string]any) operatorWakeLineage {
	return operatorWakeLineage{
		incidentID:       strings.TrimSpace(stringArg(args, "incident_id")),
		rootIncidentID:   strings.TrimSpace(stringArg(args, "root_incident_id")),
		parentIncidentID: strings.TrimSpace(stringArg(args, "parent_incident_id")),
	}
}

// agentAutonomyRunbookPayload delegates to the canonical roster builder so manual
// operator wakes share the exact runbook shape as schedule/standing/delegated wakes.
func agentAutonomyRunbookPayload(p roster.Profile) map[string]any {
	return roster.AutonomyRunbook(p)
}

func publishOperatorAction(k *runtime.Kernel, subject, corr string, payload map[string]any) {
	if k == nil || k.Bus() == nil {
		return
	}
	_, _ = k.Bus().Publish(event.Spec{
		Subject:       subject,
		Kind:          event.KindInfo,
		Actor:         "controlplane",
		CorrelationID: corr,
		Payload:       payload,
	})
}

func buildOperatorWakeIntent(explicit, slug, reason string, args map[string]any) string {
	if text := strings.TrimSpace(explicit); text != "" {
		return text
	}
	var b strings.Builder
	b.WriteString("Manual wake-up.\n")
	b.WriteString("You are agent ")
	b.WriteString(slug)
	b.WriteString(". You were explicitly woken by the operator/control plane.\n")
	if reason = strings.TrimSpace(reason); reason != "" {
		b.WriteString("Reason: ")
		b.WriteString(reason)
		b.WriteString("\n")
	}
	if root := strings.TrimSpace(stringArg(args, "root_incident_id")); root != "" {
		b.WriteString("Incident root: ")
		b.WriteString(root)
		b.WriteString("\n")
	}
	if incident := strings.TrimSpace(stringArg(args, "incident_id")); incident != "" {
		b.WriteString("Incident hop: ")
		b.WriteString(incident)
		b.WriteString("\n")
	}
	b.WriteString("Inspect your durable instructions, memory, mailbox, tasklist, and current health context. Do the next concrete recovery step and then stop.")
	return b.String()
}

func (s *Server) runAgentWake(corr string, p roster.Profile, intent, reason string, lineage operatorWakeLineage) {
	runbook := agentAutonomyRunbookPayload(p)
	ctx := runtime.WithAgentProfile(context.Background(), p)
	ctx = runtime.WithWakeContext(ctx, runtime.WakeContext{
		Source: "operator",
		Reason: reason,
	})
	if p.MaxCostMc > 0 {
		ctx = runtime.WithMaxCost(ctx, p.MaxCostMc)
	}
	var (
		answer string
		err    error
	)
	if p.RetryPolicy != nil && p.RetryPolicy.MaxAttempts > 1 {
		answer, err = s.k.RunWithRetry(ctx, corr, intent, *p.RetryPolicy)
	} else {
		answer, err = s.k.RunWith(ctx, corr, intent)
	}
	if err != nil {
		publishOperatorAction(s.k, "agent.wake", corr, map[string]any{
			"phase":              "failed",
			"agent":              p.Slug,
			"reason":             reason,
			"error":              err.Error(),
			"autonomy_runbook":   runbook,
			"incident_id":        lineage.incidentID,
			"root_incident_id":   lineage.rootIncidentID,
			"parent_incident_id": lineage.parentIncidentID,
		})
		return
	}
	publishOperatorAction(s.k, "agent.wake", corr, map[string]any{
		"phase":              "completed",
		"agent":              p.Slug,
		"reason":             reason,
		"answer":             truncate(answer, 300),
		"autonomy_runbook":   runbook,
		"incident_id":        lineage.incidentID,
		"root_incident_id":   lineage.rootIncidentID,
		"parent_incident_id": lineage.parentIncidentID,
	})
}

// agentActivitySummary decides whether one event belongs in an agent's timeline
// and renders a one-line summary. Attribution is by the slug fields the events
// already carry, plus the agent's own run correlations for run-scoped events.
func agentAutoRepairCooldown() time.Duration {
	raw := strings.TrimSpace(os.Getenv(brand.EnvPrefix + "AUTO_REPAIR_COOLDOWN"))
	if raw == "" {
		return 30 * time.Minute
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return 30 * time.Minute
	}
	return d
}

func (s *Server) agentRepairSummaries() map[string]agentRepairSummary {
	cooldown := agentAutoRepairCooldown()
	latestBySlug := map[string]agentRepairRow{}
	latestBySlugFingerprint := map[string]map[string]agentRepairRow{}
	_ = s.k.Journal().Range(func(e *event.Event) error {
		if e.Subject != "doctor.auto_repair" || e.Kind != event.KindInfo {
			return nil
		}
		var pl map[string]any
		if json.Unmarshal(e.Payload, &pl) != nil {
			return nil
		}
		slug := plString(pl, "agent")
		if strings.TrimSpace(slug) == "" {
			return nil
		}
		row := agentRepairRow{
			Seq:                            e.Seq,
			TSUnixMS:                       e.TSUnixMS,
			CorrelationID:                  e.CorrelationID,
			Mode:                           plString(pl, "mode"),
			Phase:                          plString(pl, "phase"),
			Reason:                         plString(pl, "reason"),
			Fingerprint:                    plString(pl, "fingerprint"),
			SelfRepairAttempt:              plInt(pl, "self_repair_attempt"),
			SelfRepairMaxAttempts:          plInt(pl, "self_repair_max_attempts"),
			Issues:                         plStrings(pl, "issues"),
			Applied:                        plStrings(pl, "applied"),
			Answer:                         plString(pl, "answer"),
			Error:                          plString(pl, "error"),
			TargetAgent:                    plString(pl, "target_agent"),
			TargetCorr:                     plString(pl, "target_correlation"),
			MailboxMessage:                 plString(pl, "mailbox_message_id"),
			Resolution:                     plString(pl, "resolution"),
			ResolutionSummary:              plString(pl, "resolution_summary"),
			DelegateTo:                     plString(pl, "delegate_to"),
			DelegatedBy:                    plString(pl, "delegated_by"),
			RootAgent:                      plString(pl, "root_agent"),
			ChainDepth:                     intNumber(pl["chain_depth"]),
			IncidentID:                     plString(pl, "incident_id"),
			RootIncidentID:                 plString(pl, "root_incident_id"),
			ParentIncidentID:               plString(pl, "parent_incident_id"),
			NextEligibleMS:                 e.TSUnixMS + cooldown.Milliseconds(),
			RoutingTaskType:                plString(pl, "routing_task_type"),
			RoutingTaskModelChain:          plStrings(pl, "routing_task_model_chain"),
			PreviousRoutingTaskModelChain:  plStrings(pl, "previous_routing_task_model_chain"),
			RoutingForceGeneration:         intNumber(pl["routing_force_generation"]),
			PreviousRoutingForceGeneration: intNumber(pl["previous_routing_force_generation"]),
		}
		if cur, ok := latestBySlug[slug]; !ok || row.Seq > cur.Seq {
			latestBySlug[slug] = row
		}
		if row.Fingerprint != "" {
			if latestBySlugFingerprint[slug] == nil {
				latestBySlugFingerprint[slug] = map[string]agentRepairRow{}
			}
			if cur, ok := latestBySlugFingerprint[slug][row.Fingerprint]; !ok || row.Seq > cur.Seq {
				latestBySlugFingerprint[slug][row.Fingerprint] = row
			}
		}
		return nil
	})
	out := map[string]agentRepairSummary{}
	for slug, latest := range latestBySlug {
		sum := agentRepairSummary{Latest: latest, HasLatest: true}
		for _, row := range latestBySlugFingerprint[slug] {
			if row.Phase == "queued" || row.Phase == "routing_rollback_queued" {
				sum.InflightCount++
			}
		}
		out[slug] = sum
	}
	return out
}

func repairPhaseLabel(mode, phase string) string {
	mode = strings.TrimSpace(mode)
	switch strings.TrimSpace(phase) {
	case "routing_forced_failed_detected":
		return "forced chain failed"
	case "routing_force_exhausted_detected":
		return "forced chain exhausted"
	case "routing_unstable_detected":
		return "unstable routing"
	case "attempts_exhausted":
		return "repair exhausted"
	case "queued":
		if mode == "routing_unstable" {
			return "unstable routing"
		}
		if mode == "degraded" {
			return "doctor queued"
		}
		if mode == "routing" {
			return "routing queued"
		}
		return "repair queued"
	case "routing_rollback_queued":
		return "rollback queued"
	case "completed":
		if mode == "degraded" {
			return "doctor repaired"
		}
		if mode == "routing" {
			return "routing stabilized"
		}
		return "repaired"
	case "routing_rollback_completed":
		return "rolled back"
	case "failed":
		if mode == "degraded" {
			return "doctor failed"
		}
		if mode == "routing" {
			return "routing failed"
		}
		return "repair failed"
	case "routing_rollback_failed":
		return "rollback failed"
	case "escalation_answered":
		return "manager answered"
	case "resolution_applied":
		return "manager applied"
	case "escalation_woke":
		return "manager woke"
	case "escalation_skipped":
		return "wake skipped"
	case "escalation_failed":
		return "wake failed"
	case "resolution_failed":
		return "resolution failed"
	case "delegation_queued":
		return "delegation queued"
	case "delegation_woke":
		return "delegation woke"
	case "delegation_failed":
		return "delegation failed"
	default:
		if strings.TrimSpace(phase) == "" {
			return "idle"
		}
		return phase
	}
}

func repairRowsView(rows []agentRepairRow) []map[string]any {
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, repairRowView(row))
	}
	return out
}

func repairRowView(row agentRepairRow) map[string]any {
	return map[string]any{
		"seq":                               row.Seq,
		"ts_unix_ms":                        row.TSUnixMS,
		"correlation_id":                    row.CorrelationID,
		"mode":                              row.Mode,
		"phase":                             row.Phase,
		"reason":                            row.Reason,
		"fingerprint":                       row.Fingerprint,
		"self_repair_attempt":               row.SelfRepairAttempt,
		"self_repair_max_attempts":          row.SelfRepairMaxAttempts,
		"issues":                            row.Issues,
		"applied":                           row.Applied,
		"answer":                            row.Answer,
		"error":                             row.Error,
		"target_agent":                      row.TargetAgent,
		"target_correlation":                row.TargetCorr,
		"mailbox_message_id":                row.MailboxMessage,
		"resolution":                        row.Resolution,
		"resolution_summary":                row.ResolutionSummary,
		"delegate_to":                       row.DelegateTo,
		"delegated_by":                      row.DelegatedBy,
		"root_agent":                        row.RootAgent,
		"chain_depth":                       row.ChainDepth,
		"incident_id":                       row.IncidentID,
		"root_incident_id":                  row.RootIncidentID,
		"parent_incident_id":                row.ParentIncidentID,
		"next_eligible_ms":                  row.NextEligibleMS,
		"routing_task_type":                 row.RoutingTaskType,
		"routing_task_model_chain":          row.RoutingTaskModelChain,
		"previous_routing_task_model_chain": row.PreviousRoutingTaskModelChain,
		"routing_force_generation":          row.RoutingForceGeneration,
		"previous_routing_force_generation": row.PreviousRoutingForceGeneration,
	}
}

func (s *Server) agentEscalationRows(st *board.Store, slug string, limit int, cursorTS int64, cursorID string) ([]agentEscalationRow, string) {
	msgs := st.Read("help", boardReadMaxLimit)
	metaByMessage := map[string]agentRepairRow{}
	_ = s.k.Journal().Range(func(e *event.Event) error {
		if e.Subject != "doctor.auto_repair" || e.Kind != event.KindInfo {
			return nil
		}
		var pl map[string]any
		if json.Unmarshal(e.Payload, &pl) != nil {
			return nil
		}
		if plString(pl, "target_agent") != slug {
			return nil
		}
		msgID := plString(pl, "mailbox_message_id")
		if strings.TrimSpace(msgID) == "" {
			return nil
		}
		row := agentRepairRow{
			Seq:                            e.Seq,
			TSUnixMS:                       e.TSUnixMS,
			Agent:                          plString(pl, "agent"),
			CorrelationID:                  e.CorrelationID,
			Mode:                           plString(pl, "mode"),
			Phase:                          plString(pl, "phase"),
			Reason:                         plString(pl, "reason"),
			Error:                          plString(pl, "error"),
			Fingerprint:                    plString(pl, "fingerprint"),
			SelfRepairAttempt:              plInt(pl, "self_repair_attempt"),
			SelfRepairMaxAttempts:          plInt(pl, "self_repair_max_attempts"),
			TargetAgent:                    plString(pl, "target_agent"),
			TargetCorr:                     plString(pl, "target_correlation"),
			Resolution:                     plString(pl, "resolution"),
			ResolutionSummary:              plString(pl, "resolution_summary"),
			DelegateTo:                     plString(pl, "delegate_to"),
			DelegatedBy:                    plString(pl, "delegated_by"),
			RootAgent:                      plString(pl, "root_agent"),
			ChainDepth:                     intNumber(pl["chain_depth"]),
			IncidentID:                     plString(pl, "incident_id"),
			RootIncidentID:                 plString(pl, "root_incident_id"),
			ParentIncidentID:               plString(pl, "parent_incident_id"),
			RoutingForceGeneration:         intNumber(pl["routing_force_generation"]),
			PreviousRoutingForceGeneration: intNumber(pl["previous_routing_force_generation"]),
		}
		if cur, ok := metaByMessage[msgID]; !ok || row.Seq > cur.Seq {
			metaByMessage[msgID] = row
		}
		return nil
	})
	out := make([]agentEscalationRow, 0, len(msgs))
	for _, msg := range msgs {
		if !msg.Help {
			continue
		}
		if msg.To != slug && msg.To != board.Everyone {
			continue
		}
		replies := st.Replies(msg.ID, boardReadMaxLimit)
		acked := boardMessageAckedBy(msg, slug)
		status := "open"
		if len(replies) > 0 {
			status = "answered"
		} else if acked {
			status = "acked"
		}
		row := agentEscalationRow{
			MessageID:  msg.ID,
			From:       msg.From,
			To:         msg.To,
			Text:       msg.Text,
			TSUnixMS:   msg.TSMS,
			Status:     status,
			ReplyCount: len(replies),
			Acked:      acked,
		}
		if meta, ok := metaByMessage[msg.ID]; ok {
			row.SourceAgent = meta.Agent
			row.Mode = meta.Mode
			row.WakePhase = meta.Phase
			row.WakeReason = meta.Reason
			row.WakeError = meta.Error
			row.WakeCorrelationID = meta.TargetCorr
			row.Fingerprint = meta.Fingerprint
			row.Resolution = meta.Resolution
			row.ResolutionSummary = meta.ResolutionSummary
			row.DelegateTo = meta.DelegateTo
			row.RootAgent = meta.RootAgent
			row.ChainDepth = meta.ChainDepth
			row.IncidentID = meta.IncidentID
			row.RootIncidentID = meta.RootIncidentID
			row.ParentIncidentID = meta.ParentIncidentID
			if strings.HasPrefix(meta.Phase, "delegation_") {
				row.OriginKind = "delegated"
				row.OriginAgent = firstNonEmpty(meta.DelegatedBy, msg.From)
			} else {
				row.OriginKind = "doctor"
				row.OriginAgent = firstNonEmpty(msg.From, meta.DelegatedBy)
			}
		}
		// Prefer parsing the broken/source agent out of the message text only when
		// the event envelope didn't carry one (older history).
		if row.SourceAgent == "" {
			row.SourceAgent = escalationSourceFromText(msg.Text)
		}
		if row.RootAgent == "" {
			row.RootAgent = row.SourceAgent
		}
		if row.OriginKind == "" {
			row.OriginKind = "doctor"
			row.OriginAgent = msg.From
		}
		out = append(out, row)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].TSUnixMS > out[j].TSUnixMS })
	// Cursor pagination: cursor encodes (TSUnixMS, MessageID) of the LAST
	// entry on the previous page; server skips entries strictly newer-or-equal.
	if cursorTS > 0 || cursorID != "" {
		filtered := out[:0]
		for _, r := range out {
			if r.TSUnixMS > cursorTS {
				continue
			}
			if r.TSUnixMS == cursorTS && r.MessageID >= cursorID {
				continue
			}
			filtered = append(filtered, r)
		}
		out = filtered
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
		last := out[limit-1]
		return out, strconv.FormatInt(last.TSUnixMS, 10) + ":" + last.MessageID
	}
	return out, ""
}

func boardMessageAckedBy(m board.Message, slug string) bool {
	slug = strings.ToLower(strings.TrimSpace(slug))
	for _, by := range m.AckedBy {
		if strings.ToLower(strings.TrimSpace(by)) == slug {
			return true
		}
	}
	return false
}

func escalationSourceFromText(text string) string {
	text = strings.TrimSpace(text)
	const prefix = "Doctor "
	if !strings.HasPrefix(text, prefix) {
		return ""
	}
	if i := strings.Index(text, " for agent "); i > 0 {
		// The agent named after "for agent" is the broken agent, not the owner.
		start := i + len(" for agent ")
		if end := strings.Index(text[start:], "."); end > 0 {
			return strings.TrimSpace(text[start : start+end])
		}
	}
	return ""
}

func joinActivityParts(parts ...string) string {
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return strings.Join(out, " · ")
}

func wakeRunbookActivitySuffix(pl map[string]any) string {
	raw, _ := pl["autonomy_runbook"].(map[string]any)
	if len(raw) == 0 {
		return ""
	}
	parts := []string{
		plString(raw, "trigger_contract"),
		plString(raw, "route_contract"),
		plString(raw, "recovery_contract"),
		plString(raw, "sleep_contract"),
	}
	clean := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			clean = append(clean, part)
		}
	}
	if len(clean) == 0 {
		return ""
	}
	return "contract " + strings.Join(clean, "/")
}

func (s *Server) handleAgentRetire(conn net.Conn, req Request) {
	s.handleAgentSetRetired(conn, req, true)
}

func (s *Server) handleAgentRevive(conn net.Conn, req Request) {
	s.handleAgentSetRetired(conn, req, false)
}

func (s *Server) handleAgentSetRetired(conn net.Conn, req Request, retired bool) {
	ref, err := requiredArgString(req.Args, "ref")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	reason := stringArg(req.Args, "reason")
	// Compute impact BEFORE the state change so a retire reports what it affected.
	var impact []string
	var impactSummary map[string]any
	if retired {
		if p, ok := s.k.Roster().Get(ref); ok {
			impact = s.k.AgentImpact(p.Slug)
			impactSummary = s.agentImpactResult(p)
		}
	} else if p, ok := s.k.Roster().Get(ref); ok {
		if err := s.validateAgentHierarchyRefs(p); err != nil {
			s.fail(conn, req, err)
			return
		}
	}
	p, err := s.k.SetProfileRetired(ref, retired, reason)
	if err != nil {
		if errors.Is(err, roster.ErrNotFound) {
			s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "unknown agent: " + ref})
			return
		}
		s.fail(conn, req, err)
		return
	}
	res := map[string]any{"profile": profileView(p)}
	if retired {
		pausedStanding, err := s.pauseAgentStanding(p.Slug)
		if err != nil {
			s.fail(conn, req, err)
			return
		}
		pausedSchedules, err := s.pauseAgentSchedules(p.Slug)
		if err != nil {
			s.fail(conn, req, err)
			return
		}
		res["impact"] = impact
		res["impact_summary"] = impactSummary
		res["standing_paused"] = pausedStanding
		res["schedules_paused"] = pausedSchedules
		if impactSummary != nil {
			impactSummary["standing_paused"] = pausedStanding
			impactSummary["schedules_paused"] = pausedSchedules
		}
		publishOperatorAction(s.k, "agent.retire", s.k.NewCorrelation(), map[string]any{
			"agent":            p.Slug,
			"reason":           p.RetiredReason,
			"retired_ms":       p.RetiredMS,
			"standing_paused":  pausedStanding,
			"schedules_paused": pausedSchedules,
			"impact_summary":   impactSummary,
		})
	} else {
		pausedStanding := s.countAgentPausedStanding(p.Slug)
		pausedSchedules := s.countAgentPausedSchedules(p.Slug)
		res["standing_paused"] = pausedStanding
		res["schedules_paused"] = pausedSchedules
		publishOperatorAction(s.k, "agent.revive", s.k.NewCorrelation(), map[string]any{
			"agent":            p.Slug,
			"standing_paused":  pausedStanding,
			"schedules_paused": pausedSchedules,
		})
	}
	s.invalidateAgentListCache()
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: res})
}

func (s *Server) handleAgentRemove(conn net.Conn, req Request) {
	ref, err := requiredArgString(req.Args, "ref")
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	p, found := s.k.Roster().Get(ref)
	if !found {
		s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{"removed": false}})
		return
	}
	if p.System {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: "system agent " + p.Slug + " cannot be removed; retire or pause it instead"})
		return
	}
	cascade := parseAgentRemoveCascade(req.Args["cascade"])
	subagents := s.agentSubagents(p.Slug)
	if len(subagents) > 0 && !cascade.Subagents {
		s.writeResp(conn, Response{ID: req.ID, Type: RespError, Error: fmt.Sprintf("agent %s has %d dependent sub-agent(s); set cascade.subagents=true to retire them before removal", p.Slug, len(subagents))})
		return
	}
	retainedMailboxMessageLabels := s.agentRemovalMailboxImpact(p.Slug, subagents, cascade.Subagents)
	retainedWorkflowRefLabels := s.agentWorkflowImpact(p)
	retainedSubagentWorkflowRefLabels := []string(nil)
	if cascade.Subagents {
		retainedSubagentWorkflowRefLabels = s.subagentImpact(subagents, (*Server).agentWorkflowImpact)
	}
	retainedMailboxMessages := len(retainedMailboxMessageLabels)
	retainedWorkflowRefs := len(retainedWorkflowRefLabels)
	retainedSubagentWorkflowRefs := len(retainedSubagentWorkflowRefLabels)
	retiredSubagents, retiredSubagentSlugs, err := s.retireAgentSubagents(p.Slug, subagents, cascade.Subagents)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	removedStanding, err := s.removeAgentStanding(p.Slug, cascade.Standing)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	removedSchedules, err := s.removeAgentSchedules(p.Slug, cascade.Schedules)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	if cascade.Subagents {
		for _, child := range subagents {
			n, err := s.removeAgentStanding(child.Slug, cascade.Standing)
			if err != nil {
				s.fail(conn, req, err)
				return
			}
			removedStanding += n
			n, err = s.removeAgentSchedules(child.Slug, cascade.Schedules)
			if err != nil {
				s.fail(conn, req, err)
				return
			}
			removedSchedules += n
		}
	}
	forgotMemory, err := s.forgetAgentMemory(p, cascade.Memory)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	forgotAuthoredMemory, err := s.forgetAgentAuthoredSharedMemory(p.Slug, cascade.AuthoredMemory)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	archivedSkills, err := s.archiveAgentSkills(p.Slug, cascade.Skills)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	deletedConfig, prunedConfigAccess, err := s.deleteAgentConfigEntries(p.Slug, cascade.Config)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	deletedWorkspaces, err := s.deleteAgentWorkspace(p, cascade.Workspace)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	if cascade.Subagents {
		for _, child := range subagents {
			n, err := s.forgetAgentMemory(child, cascade.Memory)
			if err != nil {
				s.fail(conn, req, err)
				return
			}
			forgotMemory += n
			n, err = s.forgetAgentAuthoredSharedMemory(child.Slug, cascade.AuthoredMemory)
			if err != nil {
				s.fail(conn, req, err)
				return
			}
			forgotAuthoredMemory += n
			n, err = s.archiveAgentSkills(child.Slug, cascade.Skills)
			if err != nil {
				s.fail(conn, req, err)
				return
			}
			archivedSkills += n
			var pruned int
			n, pruned, err = s.deleteAgentConfigEntries(child.Slug, cascade.Config)
			if err != nil {
				s.fail(conn, req, err)
				return
			}
			deletedConfig += n
			prunedConfigAccess += pruned
			n, err = s.deleteAgentWorkspace(child, cascade.Workspace)
			if err != nil {
				s.fail(conn, req, err)
				return
			}
			deletedWorkspaces += n
		}
	}
	ok, err := s.k.RemoveProfile(ref)
	if err != nil {
		s.fail(conn, req, err)
		return
	}
	if ok {
		publishOperatorAction(s.k, "agent.remove", s.k.NewCorrelation(), map[string]any{
			"agent":                                  p.Slug,
			"removed":                                true,
			"cascade":                                agentRemoveCascadeView(cascade),
			"standing_removed":                       removedStanding,
			"schedules_removed":                      removedSchedules,
			"memories_forgotten":                     forgotMemory,
			"authored_memories_forgotten":            forgotAuthoredMemory,
			"skills_archived":                        archivedSkills,
			"configs_deleted":                        deletedConfig,
			"configs_access_pruned":                  prunedConfigAccess,
			"workspaces_deleted":                     deletedWorkspaces,
			"subagents_retired":                      retiredSubagents,
			"subagents_retired_slugs":                retiredSubagentSlugs,
			"mailbox_messages_retained":              retainedMailboxMessages,
			"mailbox_messages_retained_refs":         retainedMailboxMessageLabels,
			"workflow_refs_retained":                 retainedWorkflowRefs,
			"workflow_refs_retained_labels":          retainedWorkflowRefLabels,
			"subagent_workflow_refs_retained":        retainedSubagentWorkflowRefs,
			"subagent_workflow_refs_retained_labels": retainedSubagentWorkflowRefLabels,
		})
	}
	s.writeResp(conn, Response{ID: req.ID, Type: RespResult, Result: map[string]any{
		"removed":                                ok,
		"standing_removed":                       removedStanding,
		"schedules_removed":                      removedSchedules,
		"memories_forgotten":                     forgotMemory,
		"authored_memories_forgotten":            forgotAuthoredMemory,
		"skills_archived":                        archivedSkills,
		"configs_deleted":                        deletedConfig,
		"configs_access_pruned":                  prunedConfigAccess,
		"workspaces_deleted":                     deletedWorkspaces,
		"subagents_retired":                      retiredSubagents,
		"subagents_retired_slugs":                retiredSubagentSlugs,
		"mailbox_messages_retained":              retainedMailboxMessages,
		"mailbox_messages_retained_refs":         retainedMailboxMessageLabels,
		"workflow_refs_retained":                 retainedWorkflowRefs,
		"workflow_refs_retained_labels":          retainedWorkflowRefLabels,
		"subagent_workflow_refs_retained":        retainedSubagentWorkflowRefs,
		"subagent_workflow_refs_retained_labels": retainedSubagentWorkflowRefLabels,
	}})
	s.invalidateAgentListCache()
}

type agentRemoveCascade struct {
	Standing       bool
	Schedules      bool
	Memory         bool
	AuthoredMemory bool
	Skills         bool
	Config         bool
	Workspace      bool
	Subagents      bool
}

func agentRemoveCascadeView(c agentRemoveCascade) map[string]any {
	return map[string]any{
		"standing":        c.Standing,
		"schedules":       c.Schedules,
		"memory":          c.Memory,
		"authored_memory": c.AuthoredMemory,
		"skills":          c.Skills,
		"config":          c.Config,
		"workspace":       c.Workspace,
		"subagents":       c.Subagents,
	}
}

func parseAgentRemoveCascade(raw any) agentRemoveCascade {
	var c agentRemoveCascade
	m, ok := raw.(map[string]any)
	if !ok {
		return c
	}
	c.Standing = boolish(m["standing"])
	c.Schedules = boolish(m["schedules"])
	c.Memory = boolish(m["memory"])
	c.AuthoredMemory = boolish(m["authored_memory"]) || boolish(m["authored_shared_memory"])
	c.Skills = boolish(m["skills"])
	c.Config = boolish(m["config"])
	c.Workspace = boolish(m["workspace"]) || boolish(m["workdir"])
	c.Subagents = boolish(m["subagents"])
	return c
}

func boolish(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		x = strings.TrimSpace(strings.ToLower(x))
		return x == "1" || x == "true" || x == "yes" || x == "on"
	default:
		return false
	}
}

func agentMailboxImpactLabel(msg board.Message, slug string) (string, bool) {
	from := strings.ToLower(strings.TrimSpace(msg.From))
	to := strings.ToLower(strings.TrimSpace(msg.To))
	acked := boardMessageAckedBy(msg, slug)
	var direction string
	switch {
	case from == slug:
		direction = "sent"
	case to == slug:
		direction = "received"
	case msg.To == board.Everyone && from != slug:
		direction = "broadcast"
	case acked:
		direction = "acked"
	default:
		return "", false
	}
	topic := strings.TrimSpace(msg.Topic)
	if topic == "" {
		topic = "board"
	}
	id := strings.TrimSpace(msg.ID)
	if id == "" {
		id = strconv.FormatInt(msg.TSMS, 10)
	}
	return topic + " " + direction + " (" + id + ")", true
}

func agentSubagentImpact(slug string, children []roster.Profile) []string {
	out := make([]string, 0, len(children))
	for _, child := range children {
		roles := make([]string, 0, 2)
		if strings.EqualFold(strings.TrimSpace(child.OwnerAgent), slug) {
			roles = append(roles, "owner")
		}
		if strings.EqualFold(strings.TrimSpace(child.ParentAgent), slug) {
			roles = append(roles, "parent")
		}
		if len(roles) == 0 {
			roles = append(roles, "descendant")
		}
		label := child.Slug
		if strings.TrimSpace(child.Name) != "" && strings.TrimSpace(child.Name) != child.Slug {
			label = strings.TrimSpace(child.Name) + " (" + child.Slug + ")"
		}
		if len(roles) > 0 {
			label += " [" + strings.Join(roles, ", ") + "]"
		}
		if child.Retired {
			label += " [retired]"
		}
		out = append(out, label)
	}
	sort.Strings(out)
	return out
}

func workflowNodeConfigReferencesAgent(raw json.RawMessage, slug string) bool {
	if len(raw) == 0 {
		return false
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return false
	}
	return jsonValueReferencesAgent(v, strings.ToLower(strings.TrimSpace(slug)), "")
}

func jsonValueReferencesAgent(v any, slug, key string) bool {
	switch x := v.(type) {
	case map[string]any:
		for k, value := range x {
			if jsonValueReferencesAgent(value, slug, strings.ToLower(strings.TrimSpace(k))) {
				return true
			}
		}
	case []any:
		for _, value := range x {
			if jsonValueReferencesAgent(value, slug, key) {
				return true
			}
		}
	case string:
		if !agentReferenceConfigKey(key) {
			return false
		}
		return strings.EqualFold(strings.TrimSpace(x), slug)
	}
	return false
}

func agentReferenceConfigKey(key string) bool {
	switch key {
	case "agent", "agent_slug", "target_agent", "owner_agent", "parent_agent", "delegate_to", "source_agent", "root_agent":
		return true
	default:
		return false
	}
}

func (s *Server) agentSubagents(slug string) []roster.Profile {
	root := strings.TrimSpace(slug)
	if root == "" {
		return nil
	}
	byManager := map[string][]roster.Profile{}
	for _, p := range s.k.Roster().List() {
		childSlug := strings.TrimSpace(p.Slug)
		if childSlug == "" || strings.EqualFold(childSlug, root) {
			continue
		}
		seenManager := map[string]bool{}
		for _, manager := range []string{strings.TrimSpace(p.OwnerAgent), strings.TrimSpace(p.ParentAgent)} {
			if manager == "" || strings.EqualFold(manager, childSlug) || seenManager[strings.ToLower(manager)] {
				continue
			}
			seenManager[strings.ToLower(manager)] = true
			byManager[strings.ToLower(manager)] = append(byManager[strings.ToLower(manager)], p)
		}
	}
	var out []roster.Profile
	seen := map[string]bool{strings.ToLower(root): true}
	var walk func(string)
	walk = func(parent string) {
		for _, child := range byManager[strings.ToLower(strings.TrimSpace(parent))] {
			key := strings.ToLower(strings.TrimSpace(child.Slug))
			if key == "" || seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, child)
			walk(child.Slug)
		}
	}
	walk(root)
	sort.Slice(out, func(i, j int) bool {
		return strings.Compare(out[i].Slug, out[j].Slug) < 0
	})
	return out
}

func (s *Server) agentWorkspaceInfo(p roster.Profile) (string, bool) {
	workdir := strings.TrimSpace(p.Workdir)
	if workdir == "" {
		return "", false
	}
	root := s.agentWorkspaceRoot()
	dir, ok := confineUnder(root, workdir)
	if !ok || filepath.Clean(dir) == filepath.Clean(root) {
		return "", false
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return "", false
	}
	files, bytes := countTreeFiles(dir)
	return filepath.ToSlash(workdir) + fmt.Sprintf(" (%d file(s), %d bytes)", files, bytes), true
}

func (s *Server) agentWorkspaceRoot() string {
	if ws := os.Getenv(brand.EnvPrefix + "WORKSPACE"); strings.TrimSpace(ws) != "" {
		return ws
	}
	return filepath.Join(s.k.BaseDir(), "workspace")
}

func countTreeFiles(root string) (int, int64) {
	var files int
	var bytes int64
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		info, ierr := d.Info()
		if ierr != nil {
			return nil
		}
		files++
		bytes += info.Size()
		return nil
	})
	return files, bytes
}

func registerRosterCommands() {
	register(
		commandSpec{Cmd: CmdAgentList, Handler: func(dc *DispatchCtx) { dc.S.handleAgentList(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdAgentAdd, Handler: func(dc *DispatchCtx) { dc.S.handleAgentAdd(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdAgentEdit, Handler: func(dc *DispatchCtx) { dc.S.handleAgentEdit(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdAgentSetEnabled, Handler: func(dc *DispatchCtx) { dc.S.handleAgentSetEnabled(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdAgentRemove, Handler: func(dc *DispatchCtx) { dc.S.handleAgentRemove(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdAgentTaskUpdate, Handler: func(dc *DispatchCtx) { dc.S.handleAgentTaskUpdate(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdAgentImpact, Handler: func(dc *DispatchCtx) { dc.S.handleAgentImpact(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdAgentTombstone, Handler: func(dc *DispatchCtx) { dc.S.handleAgentTombstone(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdAgentGraveyard, Handler: func(dc *DispatchCtx) { dc.S.handleAgentGraveyard(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdAgentActivity, Handler: func(dc *DispatchCtx) { dc.S.handleAgentActivity(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdAgentRepairStatus, Handler: func(dc *DispatchCtx) { dc.S.handleAgentRepairStatus(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdAgentRepair, Handler: func(dc *DispatchCtx) { dc.S.handleAgentRepair(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdAgentEscalations, Handler: func(dc *DispatchCtx) { dc.S.handleAgentEscalations(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdAgentWake, Handler: func(dc *DispatchCtx) { dc.S.handleAgentWake(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdAgentResolve, Handler: func(dc *DispatchCtx) { dc.S.handleAgentResolve(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdAgentRetire, Handler: func(dc *DispatchCtx) { dc.S.handleAgentRetire(dc.Conn, dc.Req) }},
		commandSpec{Cmd: CmdAgentRevive, Handler: func(dc *DispatchCtx) { dc.S.handleAgentRevive(dc.Conn, dc.Req) }},
	)
}

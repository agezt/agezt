// SPDX-License-Identifier: MIT

package roster

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/agezt/agezt/kernel/app"
	"github.com/agezt/agezt/kernel/contract/opapi"
	"github.com/agezt/agezt/kernel/event"
	core "github.com/agezt/agezt/kernel/roster"
)

type RepairRowOutput struct {
	Seq                            int64    `json:"seq"`
	TSUnixMS                       int64    `json:"ts_unix_ms"`
	CorrelationID                  string   `json:"correlation_id"`
	Mode                           string   `json:"mode"`
	Phase                          string   `json:"phase"`
	Reason                         string   `json:"reason"`
	Fingerprint                    string   `json:"fingerprint"`
	SelfRepairAttempt              int      `json:"self_repair_attempt"`
	SelfRepairMaxAttempts          int      `json:"self_repair_max_attempts"`
	Issues                         []string `json:"issues"`
	Applied                        []string `json:"applied"`
	Answer                         string   `json:"answer"`
	Error                          string   `json:"error"`
	TargetAgent                    string   `json:"target_agent"`
	TargetCorrelation              string   `json:"target_correlation"`
	MailboxMessageID               string   `json:"mailbox_message_id"`
	Resolution                     string   `json:"resolution"`
	ResolutionSummary              string   `json:"resolution_summary"`
	DelegateTo                     string   `json:"delegate_to"`
	DelegatedBy                    string   `json:"delegated_by"`
	RootAgent                      string   `json:"root_agent"`
	ChainDepth                     int      `json:"chain_depth"`
	IncidentID                     string   `json:"incident_id"`
	RootIncidentID                 string   `json:"root_incident_id"`
	ParentIncidentID               string   `json:"parent_incident_id"`
	NextEligibleMS                 int64    `json:"next_eligible_ms"`
	RoutingTaskType                string   `json:"routing_task_type"`
	RoutingTaskModelChain          []string `json:"routing_task_model_chain"`
	PreviousRoutingTaskModelChain  []string `json:"previous_routing_task_model_chain"`
	RoutingForceGeneration         int      `json:"routing_force_generation"`
	PreviousRoutingForceGeneration int      `json:"previous_routing_force_generation"`
}

func repairRowOutput(row RepairRow) RepairRowOutput {
	return RepairRowOutput{
		Seq: row.Seq, TSUnixMS: row.TSUnixMS, CorrelationID: row.CorrelationID, Mode: row.Mode, Phase: row.Phase,
		Reason: row.Reason, Fingerprint: row.Fingerprint, SelfRepairAttempt: row.SelfRepairAttempt,
		SelfRepairMaxAttempts: row.SelfRepairMaxAttempts, Issues: row.Issues, Applied: row.Applied, Answer: row.Answer,
		Error: row.Error, TargetAgent: row.TargetAgent, TargetCorrelation: row.TargetCorr, MailboxMessageID: row.MailboxMessage,
		Resolution: row.Resolution, ResolutionSummary: row.ResolutionSummary, DelegateTo: row.DelegateTo,
		DelegatedBy: row.DelegatedBy, RootAgent: row.RootAgent, ChainDepth: row.ChainDepth, IncidentID: row.IncidentID,
		RootIncidentID: row.RootIncidentID, ParentIncidentID: row.ParentIncidentID, NextEligibleMS: row.NextEligibleMS,
		RoutingTaskType: row.RoutingTaskType, RoutingTaskModelChain: row.RoutingTaskModelChain,
		PreviousRoutingTaskModelChain: row.PreviousRoutingTaskModelChain, RoutingForceGeneration: row.RoutingForceGeneration,
		PreviousRoutingForceGeneration: row.PreviousRoutingForceGeneration,
	}
}

func repairRowOutputs(rows []RepairRow) []RepairRowOutput {
	out := make([]RepairRowOutput, 0, len(rows))
	for _, row := range rows {
		out = append(out, repairRowOutput(row))
	}
	return out
}

type RepairContract struct {
	RetryAttempts      int      `json:"retry_attempts"`
	RetryBackoff       string   `json:"retry_backoff"`
	RetryOn            []string `json:"retry_on"`
	DoctorAgent        string   `json:"doctor_agent"`
	FailureThreshold   int      `json:"failure_threshold"`
	SelfRepairEnabled  bool     `json:"self_repair_enabled"`
	SelfRepairAttempts int      `json:"self_repair_attempts"`
	EscalateTo         string   `json:"escalate_to"`
	CooldownSec        int      `json:"cooldown_sec"`
	AuthorityBoundary  string   `json:"authority_boundary"`
}

func repairContract(p core.Profile, cooldown time.Duration) RepairContract {
	retryAttempts := 1
	retryBackoff := "none"
	retryOn := []string{"error", "timeout"}
	if p.RetryPolicy != nil {
		if p.RetryPolicy.MaxAttempts > 0 {
			retryAttempts = p.RetryPolicy.MaxAttempts
		}
		if strings.TrimSpace(p.RetryPolicy.Backoff) != "" {
			retryBackoff = strings.TrimSpace(p.RetryPolicy.Backoff)
		}
		if len(p.RetryPolicy.RetryOn) > 0 {
			retryOn = append([]string(nil), p.RetryPolicy.RetryOn...)
		}
	}
	selfRepairEnabled := p.SelfRepairPolicy != nil && p.SelfRepairPolicy.Enabled
	selfRepairMax := 0
	escalateTo := ""
	if p.SelfRepairPolicy != nil {
		selfRepairMax = p.SelfRepairPolicy.MaxAttempts
		escalateTo = strings.TrimSpace(p.SelfRepairPolicy.EscalateTo)
	}
	doctor := ""
	failureThreshold := 0
	if p.HealthPolicy != nil {
		doctor = strings.TrimSpace(p.HealthPolicy.DoctorAgent)
		failureThreshold = p.HealthPolicy.FailureThreshold
	}
	return RepairContract{
		RetryAttempts: retryAttempts, RetryBackoff: retryBackoff, RetryOn: retryOn, DoctorAgent: doctor,
		FailureThreshold: failureThreshold, SelfRepairEnabled: selfRepairEnabled, SelfRepairAttempts: selfRepairMax,
		EscalateTo: escalateTo, CooldownSec: int(cooldown / time.Second),
		AuthorityBoundary: "agent identity owns retry, doctor, self-repair and escalation; schedules/workflows only wake this contract",
	}
}

// RepairNextAction always carries action/label/detail/tone; the optional fields
// are present (possibly empty) only for the decisions that report them.
type RepairNextAction struct {
	Action         string  `json:"action"`
	Label          string  `json:"label"`
	Detail         string  `json:"detail"`
	Tone           string  `json:"tone"`
	CorrelationID  *string `json:"correlation_id,omitempty"`
	Fingerprint    *string `json:"fingerprint,omitempty"`
	Phase          *string `json:"phase,omitempty"`
	NextEligibleMS *int64  `json:"next_eligible_ms,omitempty"`
	DelegateTo     *string `json:"delegate_to,omitempty"`
}

func repairNextAction(p core.Profile, rows, inflight []RepairRow, nowMS int64) RepairNextAction {
	action := "manual_repair"
	label := "manual repair"
	detail := "no autonomous repair is currently queued"
	tone := "muted"
	if p.Retired {
		return RepairNextAction{Action: "revive_required", Label: "revive required", Detail: "graveyard agent cannot repair until revived", Tone: "muted"}
	}
	if !p.Enabled {
		return RepairNextAction{Action: "resume_required", Label: "resume required", Detail: "paused agent cannot repair until resumed", Tone: "warn"}
	}
	if len(inflight) > 0 {
		row := inflight[0]
		return RepairNextAction{
			Action: "wait_inflight", Label: "repair in flight", Detail: repairDecisionDetail(row, "doctor/self-repair run is already queued"),
			Tone: "accent", CorrelationID: &row.CorrelationID, Fingerprint: &row.Fingerprint, Phase: &row.Phase,
		}
	}
	var latest RepairRow
	if len(rows) > 0 {
		latest = rows[0]
		if latest.NextEligibleMS > nowMS {
			return RepairNextAction{
				Action: "cooldown", Label: "cooldown active", Detail: repairDecisionDetail(latest, "wait before another autonomous repair attempt"),
				Tone: "warn", NextEligibleMS: &latest.NextEligibleMS, Phase: &latest.Phase, Fingerprint: &latest.Fingerprint,
			}
		}
		switch strings.TrimSpace(latest.Phase) {
		case "attempts_exhausted", "resolution_failed", "routing_rollback_failed", "failed":
			target := firstNonEmpty(strings.TrimSpace(latest.DelegateTo), repairEscalationOwner(p))
			if target != "" {
				return RepairNextAction{
					Action: "escalate_owner", Label: "escalate owner", Detail: repairDecisionDetail(latest, "self-repair failed; owner should take over"),
					Tone: "bad", DelegateTo: &target, Phase: &latest.Phase,
				}
			}
			return RepairNextAction{
				Action: "operator_resolution", Label: "operator resolution",
				Detail: repairDecisionDetail(latest, "repair failed and no owner escalation target is configured"), Tone: "bad", Phase: &latest.Phase,
			}
		}
	}
	if p.SelfRepairPolicy != nil && p.SelfRepairPolicy.Enabled {
		action = "run_self_repair"
		label = "self-repair eligible"
		detail = "next failure can trigger autonomous self-repair"
		tone = "good"
	} else if p.HealthPolicy != nil && strings.TrimSpace(p.HealthPolicy.DoctorAgent) != "" {
		action = "doctor_monitor"
		label = "doctor monitoring"
		detail = "doctor can queue repair after health threshold"
		tone = "good"
	}
	if latest.Phase != "" {
		detail = repairDecisionDetail(latest, detail)
	}
	return RepairNextAction{Action: action, Label: label, Detail: detail, Tone: tone}
}

func repairEscalationOwner(p core.Profile) string {
	if p.SelfRepairPolicy != nil && strings.TrimSpace(p.SelfRepairPolicy.EscalateTo) != "" {
		return strings.TrimSpace(p.SelfRepairPolicy.EscalateTo)
	}
	return firstNonEmpty(strings.TrimSpace(p.ParentAgent), strings.TrimSpace(p.OwnerAgent))
}

func repairDecisionDetail(row RepairRow, fallback string) string {
	parts := []string{fallback}
	if row.Mode != "" {
		parts = append(parts, "mode "+row.Mode)
	}
	if row.Phase != "" {
		parts = append(parts, "phase "+row.Phase)
	}
	if row.Fingerprint != "" {
		parts = append(parts, "fingerprint "+row.Fingerprint)
	}
	if row.Reason != "" {
		parts = append(parts, row.Reason)
	} else if row.Error != "" {
		parts = append(parts, row.Error)
	}
	if row.SelfRepairAttempt > 0 && row.SelfRepairMaxAttempts > 0 {
		parts = append(parts, fmt.Sprintf("attempt %d/%d", row.SelfRepairAttempt, row.SelfRepairMaxAttempts))
	}
	return strings.Join(parts, " · ")
}

type RepairStatusOutput struct {
	Slug           string            `json:"slug"`
	CooldownSec    int               `json:"cooldown_sec"`
	Contract       RepairContract    `json:"contract"`
	History        []RepairRowOutput `json:"history"`
	Count          int               `json:"count"`
	Total          int               `json:"total"`
	Inflight       []RepairRowOutput `json:"inflight"`
	InflightCount  int               `json:"inflight_count"`
	NextCursor     string            `json:"next_cursor,omitempty"`
	Latest         *RepairRowOutput  `json:"latest,omitempty"`
	NextEligibleMS *int64            `json:"next_eligible_ms,omitempty"`
	NextAction     RepairNextAction  `json:"next_action"`
}

// RepairStatusService folds the selected journal into one agent's autonomous
// self-repair history: newest-first rows, inflight fingerprints, the effective
// cooldown and the next decision.
type RepairStatusService struct {
	get      func(string) (core.Profile, bool)
	journal  func(func(*event.Event) error) error
	cooldown func() time.Duration
	now      func() time.Time
}

func NewRepairStatus(get func(string) (core.Profile, bool), journal func(func(*event.Event) error) error, cooldown func() time.Duration, now func() time.Time) *RepairStatusService {
	if now == nil {
		now = time.Now
	}
	return &RepairStatusService{get: get, journal: journal, cooldown: cooldown, now: now}
}

func (s *RepairStatusService) RepairStatus(_ context.Context, in RepairStatusRequest) (RepairStatusOutput, error) {
	ref, err := in.ref()
	if err != nil {
		return RepairStatusOutput{}, err
	}
	p, ok := s.get(ref)
	if !ok {
		return RepairStatusOutput{}, errors.New("unknown agent: " + ref)
	}
	limit, err := in.limit(20, 100)
	if err != nil {
		return RepairStatusOutput{}, err
	}
	cooldown := s.cooldown()
	var rows []RepairRow
	latestByFingerprint := map[string]RepairRow{}
	_ = s.journal(func(e *event.Event) error {
		if e.Subject != "doctor.auto_repair" || e.Kind != event.KindInfo {
			return nil
		}
		var pl map[string]any
		if json.Unmarshal(e.Payload, &pl) != nil || plString(pl, "agent") != p.Slug {
			return nil
		}
		row := RepairRow{
			Seq:                            e.Seq,
			TSUnixMS:                       e.TSUnixMS,
			Agent:                          plString(pl, "agent"),
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
			NextEligibleMS:                 e.TSUnixMS + cooldown.Milliseconds(),
			Resolution:                     plString(pl, "resolution"),
			ResolutionSummary:              plString(pl, "resolution_summary"),
			DelegateTo:                     plString(pl, "delegate_to"),
			DelegatedBy:                    plString(pl, "delegated_by"),
			RootAgent:                      plString(pl, "root_agent"),
			ChainDepth:                     intNumber(pl["chain_depth"]),
			IncidentID:                     plString(pl, "incident_id"),
			RootIncidentID:                 plString(pl, "root_incident_id"),
			ParentIncidentID:               plString(pl, "parent_incident_id"),
			RoutingTaskType:                plString(pl, "routing_task_type"),
			RoutingTaskModelChain:          plStrings(pl, "routing_task_model_chain"),
			PreviousRoutingTaskModelChain:  plStrings(pl, "previous_routing_task_model_chain"),
			RoutingForceGeneration:         intNumber(pl["routing_force_generation"]),
			PreviousRoutingForceGeneration: intNumber(pl["previous_routing_force_generation"]),
		}
		rows = append(rows, row)
		if row.Fingerprint != "" {
			latestByFingerprint[row.Fingerprint] = row
		}
		return nil
	})
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Seq > rows[j].Seq })
	total := len(rows)

	// The cursor pages the history only. latest, next_eligible_ms and the next
	// action describe the agent's current state from the full row list, so the
	// page is filtered into its own slice rather than over the full list.
	history := rows
	if cursorSeq, cursorOK := in.cursor(); cursorOK {
		history = make([]RepairRow, 0, len(rows))
		for _, r := range rows {
			if r.Seq >= cursorSeq {
				continue
			}
			history = append(history, r)
		}
	}
	var next string
	if limit > 0 && len(history) > limit {
		history = history[:limit]
		next = strconv.FormatInt(history[limit-1].Seq, 10)
	}
	inflight := make([]RepairRow, 0, len(latestByFingerprint))
	for _, row := range latestByFingerprint {
		if row.Phase == "queued" || row.Phase == "routing_rollback_queued" {
			inflight = append(inflight, row)
		}
	}
	sort.SliceStable(inflight, func(i, j int) bool { return inflight[i].Seq > inflight[j].Seq })

	out := RepairStatusOutput{
		Slug: p.Slug, CooldownSec: int(cooldown / time.Second), Contract: repairContract(p, cooldown),
		History: repairRowOutputs(history), Count: len(history), Total: total,
		Inflight: repairRowOutputs(inflight), InflightCount: len(inflight), NextCursor: next,
	}
	if len(rows) > 0 {
		latest := repairRowOutput(rows[0])
		out.Latest = &latest
		out.NextEligibleMS = &rows[0].NextEligibleMS
	}
	out.NextAction = repairNextAction(p, rows, inflight, s.now().UnixMilli())
	return out, nil
}

func RepairStatusOperations(provider func(context.Context) *RepairStatusService) ([]app.Operation, error) {
	if provider == nil {
		return nil, errors.New("roster repair status provider required")
	}
	op, err := app.NewOperation(opapi.Spec{Name: "agent_repair_status", ReadOnly: true, Authz: opapi.PrimaryOnly, Tenancy: opapi.Primary, AllowUnknownInput: true, InputSchema: json.RawMessage(`{"type":"object","additionalProperties":true,"properties":{"ref":{},"limit":{},"cursor":{}}}`), HTTP: opapi.HTTP{Method: "GET", Path: "/api/agents/repair_status"}}, func(ctx context.Context, in RepairStatusRequest) (RepairStatusOutput, error) {
		return provider(ctx).RepairStatus(ctx, in)
	})
	if err != nil {
		return nil, err
	}
	return []app.Operation{op}, nil
}

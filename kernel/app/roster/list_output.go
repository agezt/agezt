// SPDX-License-Identifier: MIT

package roster

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/agezt/agezt/kernel/platform/schema"
	core "github.com/agezt/agezt/kernel/roster"
	"reflect"
	"strings"
)

// StatusOutput describes every emitted roster status field; only observation
// payloads for runbook/mailbox retain their original dynamic object values.
type StatusOutput struct {
	ActiveCorrelationID        *string         `json:"active_correlation_id,omitempty"`
	ActiveDetail               *string         `json:"active_detail,omitempty"`
	ActiveIntent               *string         `json:"active_intent,omitempty"`
	ActiveIter                 *int            `json:"active_iter,omitempty"`
	ActiveLastEventKind        *string         `json:"active_last_event_kind,omitempty"`
	ActiveLastEventMS          *int64          `json:"active_last_event_ms,omitempty"`
	ActiveModel                *string         `json:"active_model,omitempty"`
	ActiveParentCorrelation    *string         `json:"active_parent_correlation,omitempty"`
	ActivePhase                *string         `json:"active_phase,omitempty"`
	ActiveRunCount             int             `json:"active_run_count"`
	ActiveScheduleID           *string         `json:"active_schedule_id,omitempty"`
	ActiveSpentMC              *int64          `json:"active_spent_mc,omitempty"`
	ActiveStandingID           *string         `json:"active_standing_id,omitempty"`
	ActiveStandingName         *string         `json:"active_standing_name,omitempty"`
	ActiveStartedMS            *int64          `json:"active_started_ms,omitempty"`
	ActiveTool                 *string         `json:"active_tool,omitempty"`
	ActiveTriggerSubject       *string         `json:"active_trigger_subject,omitempty"`
	ActiveWakeReason           *string         `json:"active_wake_reason,omitempty"`
	ActiveWakeSource           *string         `json:"active_wake_source,omitempty"`
	ConfigIssues               *[]string       `json:"config_issues,omitempty"`
	EscalationAckedCount       int             `json:"escalation_acked_count"`
	EscalationOpenCount        int             `json:"escalation_open_count"`
	HealthFailures             *int            `json:"health_failures,omitempty"`
	HealthLabel                string          `json:"health_label"`
	HealthState                string          `json:"health_state"`
	HealthThreshold            *int            `json:"health_threshold,omitempty"`
	HealthWindow               *int            `json:"health_window,omitempty"`
	InvalidRuntimeOverrides    int             `json:"invalid_runtime_overrides"`
	LastActiveMS               *int64          `json:"last_active_ms,omitempty"`
	LastActivityCorrelationID  *string         `json:"last_activity_correlation_id,omitempty"`
	LastActivityKind           *string         `json:"last_activity_kind,omitempty"`
	LastActivityMS             *int64          `json:"last_activity_ms,omitempty"`
	LastActivitySummary        *string         `json:"last_activity_summary,omitempty"`
	LastAutonomyRunbook        *map[string]any `json:"last_autonomy_runbook,omitempty"`
	LastFailureMS              *int64          `json:"last_failure_ms,omitempty"`
	MailboxWakes               *map[string]any `json:"mailbox_wakes,omitempty"`
	MisconfigurationCount      int             `json:"misconfiguration_count"`
	NextWakeLabel              *string         `json:"next_wake_label,omitempty"`
	NextWakeMS                 *int64          `json:"next_wake_ms,omitempty"`
	OperationalLabel           string          `json:"operational_label"`
	OperationalState           string          `json:"operational_state"`
	PolicyDeniedCount          *int            `json:"policy_denied_count,omitempty"`
	PolicyDeniedLastCapability *string         `json:"policy_denied_last_capability,omitempty"`
	PolicyDeniedLastHard       *bool           `json:"policy_denied_last_hard,omitempty"`
	PolicyDeniedLastMS         *int64          `json:"policy_denied_last_ms,omitempty"`
	PolicyDeniedLastReason     *string         `json:"policy_denied_last_reason,omitempty"`
	PolicyDeniedLastTool       *string         `json:"policy_denied_last_tool,omitempty"`
	RepairChainDepth           *int            `json:"repair_chain_depth,omitempty"`
	RepairIncidentID           *string         `json:"repair_incident_id,omitempty"`
	RepairInflight             int             `json:"repair_inflight"`
	RepairLabel                string          `json:"repair_label"`
	RepairLastCorrelationID    string          `json:"repair_last_correlation_id"`
	RepairLastError            *string         `json:"repair_last_error,omitempty"`
	RepairLastTSMS             int64           `json:"repair_last_ts_ms"`
	RepairMode                 *string         `json:"repair_mode,omitempty"`
	RepairNextEligibleMS       int64           `json:"repair_next_eligible_ms"`
	RepairParentIncidentID     *string         `json:"repair_parent_incident_id,omitempty"`
	RepairRootAgent            *string         `json:"repair_root_agent,omitempty"`
	RepairRootIncidentID       *string         `json:"repair_root_incident_id,omitempty"`
	RepairSelfAttempt          *int            `json:"repair_self_attempt,omitempty"`
	RepairSelfMaxAttempts      *int            `json:"repair_self_max_attempts,omitempty"`
	RepairState                string          `json:"repair_state"`
	RetryCount                 int             `json:"retry_count"`
	RetryLastReason            *string         `json:"retry_last_reason,omitempty"`
	RetryLastTSMS              *int64          `json:"retry_last_ts_ms,omitempty"`
	RetryMaxAttempts           *int            `json:"retry_max_attempts,omitempty"`
	RetryNextAttempt           *int            `json:"retry_next_attempt,omitempty"`
	RoutingCurrentChain        *[]string       `json:"routing_current_chain,omitempty"`
	RoutingFallbackCount       int             `json:"routing_fallback_count"`
	RoutingForceGeneration     *int            `json:"routing_force_generation,omitempty"`
	RoutingForcedChain         *[]string       `json:"routing_forced_chain,omitempty"`
	RoutingLastFailed          *string         `json:"routing_last_failed,omitempty"`
	RoutingLastNext            *string         `json:"routing_last_next,omitempty"`
	RoutingLastReason          *string         `json:"routing_last_reason,omitempty"`
	RoutingLastTSMS            *int64          `json:"routing_last_ts_ms,omitempty"`
	RoutingPreviousChain       *[]string       `json:"routing_previous_chain,omitempty"`
	RoutingTaskType            *string         `json:"routing_task_type,omitempty"`
	SelfRepairEnabled          bool            `json:"self_repair_enabled"`
	WakeEventSubjects          *[]string       `json:"wake_event_subjects,omitempty"`
	WakeScheduleCount          *int            `json:"wake_schedule_count,omitempty"`
	WakeStandingCount          *int            `json:"wake_standing_count,omitempty"`
}
type ProfileOutput struct {
	core.Profile
	Kind          string        `json:"kind"`
	Managed       bool          `json:"managed"`
	Status        *StatusOutput `json:"status,omitempty"`
	StatusPresent bool          `json:"-"`
}

// MarshalJSON preserves legacy profile float64 projection while status integers
// retain their original exact values. Its schema is derived from the full fields.
func (p ProfileOutput) MarshalJSON() ([]byte, error) {
	out := ProfileView(p.Profile)
	out["kind"] = p.Kind
	out["managed"] = p.Managed
	if p.StatusPresent || p.Status != nil {
		out["status"] = p.Status
	}
	return json.Marshal(out)
}
func statusOutput(in map[string]any) (*StatusOutput, error) {
	if in == nil {
		return nil, nil
	}
	raw, err := json.Marshal(in)
	if err != nil {
		return nil, err
	}
	var out StatusOutput
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&out); err != nil {
		return nil, err
	}
	// Decoding drops explicit nulls; nil slices/maps (a schedule-only wake row's
	// subjects) must still emit null, so restore them as pointers to nil values.
	v := reflect.ValueOf(&out).Elem()
	for key, value := range in {
		if !nullJSON(value) {
			continue
		}
		i, ok := statusFieldIndex[key]
		if !ok {
			return nil, fmt.Errorf("roster status %s is not typed", key)
		}
		f := v.Field(i)
		if f.Kind() != reflect.Pointer || (f.Type().Elem().Kind() != reflect.Slice && f.Type().Elem().Kind() != reflect.Map) {
			return nil, fmt.Errorf("roster status %s cannot be null", key)
		}
		f.Set(reflect.New(f.Type().Elem()))
	}
	return &out, nil
}

var statusFieldIndex = func() map[string]int {
	t := reflect.TypeFor[StatusOutput]()
	out := make(map[string]int, t.NumField())
	for i := range t.NumField() {
		name, _, _ := strings.Cut(t.Field(i).Tag.Get("json"), ",")
		out[name] = i
	}
	return out
}()

func nullJSON(v any) bool {
	if v == nil {
		return true
	}
	r := reflect.ValueOf(v)
	switch r.Kind() {
	case reflect.Pointer, reflect.Slice, reflect.Map, reflect.Interface:
		return r.IsNil()
	}
	return false
}

type ListOutput struct {
	Profiles     []ProfileOutput `json:"profiles"`
	Count        int             `json:"count"`
	Total        int             `json:"total"`
	EnabledCount int             `json:"enabled_count"`
	NextCursor   string          `json:"next_cursor,omitempty"`
}

// The schema view has the same complete public fields but no custom marshaler.
type profileSchema struct {
	core.Profile
	Kind    string        `json:"kind"`
	Managed bool          `json:"managed"`
	Status  *StatusOutput `json:"status,omitempty"`
}
type listSchema struct {
	Profiles     []profileSchema `json:"profiles"`
	Count        int             `json:"count"`
	Total        int             `json:"total"`
	EnabledCount int             `json:"enabled_count"`
	NextCursor   string          `json:"next_cursor,omitempty"`
}

func listOutputSchema() (json.RawMessage, error) {
	return schema.FromType(reflect.TypeFor[listSchema](), false)
}

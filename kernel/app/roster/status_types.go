// SPDX-License-Identifier: MIT
package roster

type RepairRow struct {
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

type RepairSummary struct {
	Latest        RepairRow
	HasLatest     bool
	InflightCount int
}

type RoutingPressure struct {
	Count      int
	LastReason string
	LastFailed string
	LastNext   string
	LastTSMS   int64
}

type RetryPressure struct {
	Count       int
	LastReason  string
	LastTSMS    int64
	NextAttempt int
	MaxAttempts int
}

type EscalationLoad struct {
	Open  int
	Acked int
}

type WakeStatus struct {
	ScheduleCount       int
	StandingCount       int
	EventSubjects       []string
	NextScheduledWakeMS int64
	NextScheduledLabel  string
}

type LiveStatus struct {
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

type LastActivity struct {
	TSUnixMS      int64
	Kind          string
	CorrelationID string
	Summary       string
}

type PolicyDenials struct {
	Count          int
	LastTool       string
	LastReason     string
	LastCapability string
	LastHard       bool
	LastTSMS       int64
}
type DegradedStatus struct {
	Failures, Threshold, Window int
	LastFailureMS               int64
}
type MisconfigurationStatus struct{ Issues []string }
type RoutingStatus struct{ Count int }
type ForcedStatus struct {
	Count           int
	TaskType        string
	ForcedChain     []string
	ForceGeneration int
}
type UnstableStatus struct {
	Count                       int
	TaskType                    string
	CurrentChain, PreviousChain []string
}
type DeadStatus struct{ LastActiveMS int64 }
type StatusSnapshot struct {
	Degraded                              map[string]DegradedStatus
	Misconfigured                         map[string]MisconfigurationStatus
	Routing                               map[string]RoutingStatus
	Forced, ForcedFailed, ForcedExhausted map[string]ForcedStatus
	Unstable                              map[string]UnstableStatus
	Dead                                  map[string]DeadStatus
	Repairs                               map[string]RepairSummary
	Escalations                           map[string]EscalationLoad
	Wakes                                 map[string]WakeStatus
	Live                                  map[string]LiveStatus
	LastActivities                        map[string]LastActivity
	Runbooks, MailboxWakes                map[string]map[string]any
	PolicyDenials                         map[string]PolicyDenials
	RoutingCounts                         map[string]RoutingPressure
	RetryCounts                           map[string]RetryPressure
}

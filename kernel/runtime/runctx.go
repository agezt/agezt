// SPDX-License-Identifier: MIT

// Run context types: WakeContext + other context types.
// Code extracted from runctx.go during the Day-69 god-file split. Public API unchanged.
package runtime


import (
	"time"
)



// per-run context keys used by RunWith → policyHook to carry the
// actor/correlation IDs into approval.Submit so audit events stay
// linked to the originating task.
type ctxKey int

const (
	ctxKeyActor ctxKey = iota
	ctxKeyCorrelation
	ctxKeyModel
	ctxKeyImages
	ctxKeySystem
	ctxKeyRunTimeout
	ctxKeyTools
	ctxKeyMaxCost
	ctxKeyJSONMode
	ctxKeyTrustCeiling
	ctxKeyRoot
	ctxKeyModelChain
	ctxKeyAgentIdent
	ctxKeySystemAgent
	ctxKeyAgentLifecycle
	ctxKeyAgentRetryPolicy
	ctxKeyAgentToolPolicy
	ctxKeyAgentConfigOverrides
	ctxKeyAgentNoisePolicy
	ctxKeyWakeContext
	ctxKeyAutoApproveCaps
	ctxKeyTrustedObservations
	ctxKeyResumeOwned // M1002: a resume ticket for this corr is already owned by an outer frame
	ctxKeyResumeSeed  // M1002: prior conversation + iter to seed a resumed run
)

// agentIdent carries a named agent's identity + daily ceiling for the
// Governor's per-agent ledger (M793).
type agentIdent struct {
	slug    string
	dailyMc int64
}

type agentToolPolicy struct {
	allow []string
	deny  []string
}

const agentNoiseStateNS = "agent_noise"

type agentNoisePolicy struct {
	silentOnSuccess      bool
	disableMemoryWrites  bool
	minNotifySeverity    string
	minNotifyIntervalSec int
}

type agentNoiseState struct {
	LastNotifyMS    int64 `json:"last_notify_ms"`
	PendingNotifyMS int64 `json:"pending_notify_ms,omitempty"`
}

const agentNoisePendingNotifyTTL = 5 * time.Minute

// WakeContext is durable provenance for why a run exists. It is stamped on
// task.received by agent.Run and intentionally kept separate from the prompt.
type WakeContext struct {
	Source            string
	Reason            string
	ScheduleID        string
	StandingID        string
	StandingName      string
	TriggerSubject    string
	ParentCorrelation string
}

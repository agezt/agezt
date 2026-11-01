// SPDX-License-Identifier: MIT

// Package runctx is the runs module's vocabulary: the context a run carries.
//
// # WHY IT LIVES HERE
//
// A run's dispatch — who acts, under which correlation, with which model,
// system prompt, cost cap, trust ceiling, tool allowlist and agent profile —
// is the RUN's own state. It moved here in W3.76c from the daemon's
// `kernel/runtime/runctx*.go`, which held it in five files beside the runtime
// that merely reads it.
//
// The family has no store, no rule and no Kernel reference: it is a vocabulary.
// Three parts of it were already elsewhere and stay there:
//
//   - the wake provenance and the tool allowlist are CONTRACT rows
//     (agentapi, toolapi): a host states them without naming this module, and
//     the setters here are thin delegates;
//   - the prompt-injection MODE is the platform's rule (W3.74), and the
//     daemon keeps the name;
//   - the profile's system prompt (AgentProfileSystem) and the notify severity
//     ranking are the run's dispatch rules, and they moved with the vocabulary
//     that reads them.
package runctx

import (
	"time"

	"github.com/agezt/agezt/kernel/contract/agentapi"
)

// The per-run context keys used by RunWith → policyHook to carry the
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
	ctxKeyAutoApproveCaps
	ctxKeyTrustedObservations
)

// agentIdent carries a named agent's identity + daily ceiling for the
// Governor's per-agent ledger (M793).
type agentIdent struct {
	slug    string
	dailyMc int64
}

// AgentToolPolicy is a named agent's tool allow/deny list.
type AgentToolPolicy struct {
	Allow []string
	Deny  []string
}

// AgentNoisePolicy is a named agent's noise policy: what the daemon may stay
// quiet about.
type AgentNoisePolicy struct {
	SilentOnSuccess      bool
	DisableMemoryWrites  bool
	MinNotifySeverity    string
	MinNotifyIntervalSec int
}

// AgentNoiseState is the daemon's per-agent noise bookkeeping.
type AgentNoiseState struct {
	LastNotifyMS    int64 `json:"last_notify_ms"`
	PendingNotifyMS int64 `json:"pending_notify_ms,omitempty"`
}

// AgentNoisePendingNotifyTTL is how long a notify send stays "in progress"
// for the per-agent noise policy: a second notify inside the window is
// refused rather than duplicated.
const AgentNoisePendingNotifyTTL = 5 * time.Minute

// WakeContext is durable provenance for why a run exists. It is stamped on
// task.received by agent.Run and intentionally kept separate from the prompt.
//
// The ROW lives in the contract layer (agentapi): it is provenance about a
// run, not a mechanism of the runtime, and the host that starts a run has to
// be able to state it without naming this module. This package re-exports the
// row as an alias — one row, one spelling.
type WakeContext = agentapi.WakeContext

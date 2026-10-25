// SPDX-License-Identifier: MIT

package workflowapi

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/agezt/agezt/kernel/contract/approvalapi"
	"github.com/agezt/agezt/kernel/contract/eventapi"
	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/contract/toolapi"
)

// ErrHalted is the engine's answer when the host has stopped the kernel: the
// run refuses, and the refusal says why.
//
// The sentinel lives in the contract layer because the HOST reports it (the
// kernel maps it to its own ErrHalted) and the host may not name a module's
// error.
var ErrHalted = errors.New("workflow: kernel is halted")

// Ports are the host capabilities the engine calls out to. A nil port means
// the capability is unavailable: the engine then refuses the node that needs
// it (a code node without a sandbox, an llm node without a provider) instead
// of failing the whole daemon.
//
// The struct lives here because it is the HOST's own surface handed to the
// engine — and the engine is a module, so a module's ports are the contract's.
// The rows it carries travel with it: the event spec (`eventapi.Spec`), the
// approval request and verdict (`approvalapi`), the completion
// (`llm`) and the tool result (`toolapi`).
type Ports struct {
	// Publish journals one event (workflow.started, workflow.node, …).
	Publish func(eventapi.Spec)
	// Complete runs one attributed completion through the host's Governor.
	Complete func(ctx context.Context, corr, taskType string, req llm.CompletionRequest) (*llm.CompletionResponse, error)
	// InvokeTool runs one governed tool call.
	InvokeTool func(ctx context.Context, corr, callID, toolName string, args json.RawMessage) (toolapi.Result, error)
	// RunCode runs one sandboxed script.
	RunCode func(ctx context.Context, corr, callPrefix, language, code, input string) (toolapi.Result, error)
	// ScriptRunnerReady reports whether the code sandbox exists.
	ScriptRunnerReady func() bool
	// Approve submits one operator gate and returns the decision.
	Approve func(ctx context.Context, spec approvalapi.SubmitSpec) approvalapi.Outcome
	// Model resolves the model a run should ask for (the live default plus
	// the run's own chain).
	Model func(ctx context.Context) string
	// DefaultModel is the daemon's live default model — what a draft follows
	// after the default is hot-swapped.
	DefaultModel func(ctx context.Context) string
	// HasProvider reports whether a provider is configured at all.
	HasProvider func() bool
	// Halted reports whether the host has stopped the kernel.
	Halted func() bool
}

// Provenance is why a run exists. The host fills it in from its own wake
// context — the module never reads another package's context keys.
//
// The row lives here because the engine's port carries it, and a module's
// port is the contract's.
type Provenance struct {
	Source            string
	Reason            string
	ScheduleID        string
	StandingID        string
	StandingName      string
	TriggerSubject    string
	ParentCorrelation string
}

// Trigger says what fired a workflow: the trigger kind ("cron" or "event"),
// the matched event subject (event triggers only) and a short human label
// for the daemon's logging.
//
// The row lives here because the host's trigger runner carries it, and the
// host is not allowed to name a module's row.
type Trigger struct {
	// Source is the trigger kind: "cron" or "event".
	Source string
	// Subject is the matched event subject (event triggers only).
	Subject string
	// Reason is a short human label ("cron interval", "event task.failed")
	// for the daemon's logging.
	Reason string
}

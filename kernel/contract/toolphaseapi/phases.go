// SPDX-License-Identifier: MIT

// Package toolphaseapi defines the batch-aware invocation service port.
package toolphaseapi

import (
	"context"
	"encoding/json"
	"time"

	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/contract/policyapi"
	"github.com/agezt/agezt/kernel/contract/toolapi"
)

// Resolution retains trusted implementation metadata and schema admission.
type Resolution struct {
	Tool       toolapi.Tool
	Definition toolapi.ToolDef
	Found      bool
	InputError error
}

// Decision retains policy metadata context and the audited verdict.
type Decision struct {
	Context context.Context
	Verdict policyapi.PolicyVerdict
}

// Execution retains the backend outcome and timeout/panic classification facts.
type Execution struct {
	Result     toolapi.Result
	Err        error
	PanicValue any
	TimedOut   bool
}

// Phases is the lower-layer port for callers that admit a whole batch before
// execution. It shares an invocation service without changing loop scheduling.
// Publisher kinds use strings so contracts need not import the journal module;
// implementations supply the registered kind, callers supply its envelope.
type Phases interface {
	Resolve(llm.ToolCall, func(string) (toolapi.Tool, bool)) Resolution
	Decide(context.Context, llm.ToolCall, toolapi.ToolDef, policyapi.Policy, func(llm.ToolCall, policyapi.PolicyVerdict) error) (Decision, error)
	Announce(llm.ToolCall, func(string, map[string]any) error) error
	Execute(context.Context, toolapi.Tool, json.RawMessage, time.Duration, func(any) error) Execution
	Settle(llm.ToolCall, toolapi.Result, map[string]any, func(string, map[string]any) error) error
}

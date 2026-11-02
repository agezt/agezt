// SPDX-License-Identifier: MIT

package agent

// The model-provider and tool contracts live in kernel/contract/llm and
// kernel/contract/toolapi (architecture/21 W1.1). These aliases keep every
// existing agent.X reference compiling with identical types; new code should
// import the contract packages directly so it does not depend on the loop.

import (
	"github.com/agezt/agezt/kernel/contract/llm"
	"github.com/agezt/agezt/kernel/contract/toolapi"
)

// Model-provider contract (kernel/contract/llm).
type (
	Role               = llm.Role
	Message            = llm.Message
	ToolCall           = llm.ToolCall
	CompletionRequest  = llm.CompletionRequest
	StopReason         = llm.StopReason
	CompletionResponse = llm.CompletionResponse
	Usage              = llm.Usage
	Params             = llm.Params
	Provider           = llm.Provider
	StreamingProvider  = llm.StreamingProvider
	Chunk              = llm.Chunk
)

const (
	RoleSystem    = llm.RoleSystem
	RoleUser      = llm.RoleUser
	RoleAssistant = llm.RoleAssistant
	RoleTool      = llm.RoleTool

	StopEndTurn   = llm.StopEndTurn
	StopToolUse   = llm.StopToolUse
	StopMaxTokens = llm.StopMaxTokens
)

// Tool contract (kernel/contract/toolapi).
type (
	Tool             = toolapi.Tool
	ToolDef          = toolapi.ToolDef
	ToolCapability   = toolapi.ToolCapability
	EffectClass      = toolapi.EffectClass
	ToolEffect       = toolapi.ToolEffect
	Result           = toolapi.Result
	ObservationTrust = toolapi.ObservationTrust
)

const (
	EffectUnknown      = toolapi.EffectUnknown
	EffectReadOnly     = toolapi.EffectReadOnly
	EffectReversible   = toolapi.EffectReversible
	EffectCompensable  = toolapi.EffectCompensable
	EffectIrreversible = toolapi.EffectIrreversible

	ObservationTrustDefault = toolapi.ObservationTrustDefault
	ObservationTrusted      = toolapi.ObservationTrusted
	ObservationUntrusted    = toolapi.ObservationUntrusted
)

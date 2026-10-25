// SPDX-License-Identifier: MIT

package workflow

// The rows and the rule now live in the contract layer
// (kernel/contract/workflowapi) — W3.29. The module keeps its names as
// ALIASES and its rule as a one-line DELEGATION, so there is one row and one
// spelling of it: the engine here, the agent-facing tool there, and the
// console in between all speak the same workflow.
//
// The store, the engine, the runner and the templates stay here: they are the
// implementation.

import "github.com/agezt/agezt/kernel/contract/workflowapi"

// The rows.
type (
	// Workflow is one durable, named graph.
	Workflow = workflowapi.Workflow
	// Node is one typed step on the canvas.
	Node = workflowapi.Node
	// Edge wires From's completion to To's execution.
	Edge = workflowapi.Edge
	// TriggerConfig is the trigger node's config: how a workflow STARTS.
	TriggerConfig = workflowapi.TriggerConfig

	// The per-node config payloads.
	ToolConfig         = workflowapi.ToolConfig
	LLMConfig          = workflowapi.LLMConfig
	ConditionConfig    = workflowapi.ConditionConfig
	TransformConfig    = workflowapi.TransformConfig
	DelayConfig        = workflowapi.DelayConfig
	HTTPConfig         = workflowapi.HTTPConfig
	CodeConfig         = workflowapi.CodeConfig
	MapConfig          = workflowapi.MapConfig
	FilterConfig       = workflowapi.FilterConfig
	SwitchCase         = workflowapi.SwitchCase
	SwitchConfig       = workflowapi.SwitchConfig
	MergeConfig        = workflowapi.MergeConfig
	ApprovalConfig     = workflowapi.ApprovalConfig
	SubflowConfig      = workflowapi.SubflowConfig
	PipelineConfig     = workflowapi.PipelineConfig
	PipelineStepConfig = workflowapi.PipelineStepConfig

	// RunResult carries one run's outcome: per-node outputs and the ordered
	// list of executed node ids.
	RunResult = workflowapi.RunResult
	// NodeTestResult is one tested node's outcome.
	NodeTestResult = workflowapi.NodeTestResult

	// Ports are the host capabilities the engine calls out to.
	Ports = workflowapi.Ports
	// Provenance is why a run exists.
	Provenance = workflowapi.Provenance
	// Trigger says what fired a workflow.
	Trigger = workflowapi.Trigger
)

// ErrHalted is returned when the host reports that the kernel is halted.
//
// The sentinel lives in the contract layer (kernel/contract/workflowapi) — the
// HOST reports it, and the host may not name a module's error.
var ErrHalted = workflowapi.ErrHalted

// The engine answers the contract's port, so the host can drive the workflow
// lifecycle without ever naming this module (W3.48).
var _ workflowapi.Engine = (*Engine)(nil)

// The node types the engine executes.
const (
	NodeTrigger   = workflowapi.NodeTrigger
	NodeTool      = workflowapi.NodeTool
	NodeLLM       = workflowapi.NodeLLM
	NodeCondition = workflowapi.NodeCondition
	NodeTransform = workflowapi.NodeTransform
	NodeDelay     = workflowapi.NodeDelay
	NodeHTTP      = workflowapi.NodeHTTP
	NodeCode      = workflowapi.NodeCode
	NodeMap       = workflowapi.NodeMap
	NodeFilter    = workflowapi.NodeFilter
	NodeSwitch    = workflowapi.NodeSwitch
	NodeMerge     = workflowapi.NodeMerge
	NodeApproval  = workflowapi.NodeApproval
	NodeSubflow   = workflowapi.NodeSubflow
	NodePipeline  = workflowapi.NodePipeline
)

// ErrNotFound is returned for an unknown workflow id/name.
var ErrNotFound = workflowapi.ErrNotFound

// Validate checks a workflow's shape: name/id rules, exactly one trigger,
// known node types, edges that resolve, legal ports, per-type config, and an
// acyclic graph. The engine refuses to run anything Validate rejects.
//
// The rule lives in the contract layer (kernel/contract/workflowapi) — it is
// pure, and it is the rule the whole daemon enforces. This is a delegation,
// so there is one rule.
func Validate(w Workflow) error { return workflowapi.Validate(w) }

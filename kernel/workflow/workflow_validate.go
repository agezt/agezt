// SPDX-License-Identifier: MIT

// Package workflow is the n8n-style workflow engine (M798): durable, named
// graphs of TYPED nodes — trigger, tool, llm, condition, transform, delay —
// wired by edges and carrying data between nodes with {{path}} templates.
// Unlike kernel/planner (intent-in, agent-loop-per-node), a workflow node is
// a precise, deterministic step: THIS tool with THESE args, THIS prompt to
// THIS model, THIS branch on THIS value. The graph is what you see on the
// console canvas; the engine (kernel/runtime) executes it under the same
// governance as everything else — tool nodes pass Edict, llm nodes ride the
// Governor, every step is journaled (workflow.*).
//
// Storage mirrors kernel/standing: one atomic JSON file, journaled CRUD.
package workflow

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Workflow validators + Workflow accessors: validateTriggerConfig, validateEdgePort, validateReliability, validateNodeConfig + orEmpty + Workflow.TriggerNode + Workflow.NodeByID.
// Code extracted from workflow.go during the Day-46 god-file split. Public API unchanged.

func validateTriggerConfig(n *Node) error {
	var c TriggerConfig
	if err := json.Unmarshal(orEmpty(n.Config), &c); err != nil {
		return fmt.Errorf("workflow: node %s: trigger config: %w", n.ID, err)
	}
	switch c.Kind {
	case "", "manual":
		return nil
	case "cron":
		hasInterval := c.IntervalSec != 0
		hasDaily := strings.TrimSpace(c.DailyAt) != ""
		if hasInterval == hasDaily { // both or neither
			return fmt.Errorf("workflow: node %s: cron trigger needs exactly one of interval_sec or daily_at", n.ID)
		}
		if hasInterval && c.IntervalSec < minIntervalSec {
			return fmt.Errorf("workflow: node %s: interval_sec must be >= %d", n.ID, minIntervalSec)
		}
		if hasDaily && !dailyAtRe.MatchString(strings.TrimSpace(c.DailyAt)) {
			return fmt.Errorf("workflow: node %s: daily_at must be HH:MM", n.ID)
		}
		return nil
	case "event":
		subj := strings.TrimSpace(c.Subject)
		if subj == "" {
			return fmt.Errorf("workflow: node %s: event trigger needs a subject glob", n.ID)
		}
		if strings.HasPrefix(subj, "workflow.") || subj == ">" || subj == "*" {
			// A workflow run publishes workflow.* events — triggering on them
			// (or on everything) is a feedback-loop foot-gun, refused outright.
			return fmt.Errorf("workflow: node %s: event subject %q is too broad or self-referential", n.ID, subj)
		}
		return nil
	case "webhook":
		// The secret is the ONLY gate between the internet-facing hook path
		// and a workflow run — a short one is a refused config, not a risk
		// the operator silently accepts.
		if len(strings.TrimSpace(c.Secret)) < minWebhookSecretLen {
			return fmt.Errorf("workflow: node %s: webhook trigger needs a secret of at least %d characters", n.ID, minWebhookSecretLen)
		}
		return nil
	default:
		return fmt.Errorf("workflow: node %s: unknown trigger kind %q (manual|cron|event|webhook)", n.ID, c.Kind)
	}
}

// Per-type config shapes (also the engine's parse targets).

const maxSwitchCases = 16
const maxPipelineSteps = 16

var conditionOps = map[string]bool{
	"equals": true, "not_equals": true, "contains": true,
	"not_empty": true, "empty": true, "gt": true, "lt": true,
}

// validateEdgePort enforces each node type's legal output ports: condition
// fires true/false, switch fires its declared case ports or "default",
// failable nodes may add an "error" branch, everything else uses the
// default port only.
func validateEdgePort(from *Node, port string) error {
	switch from.Type {
	case NodeCondition:
		if port != "true" && port != "false" {
			return fmt.Errorf("workflow: edge from condition %q needs port \"true\" or \"false\"", from.ID)
		}
		return nil
	case NodeSwitch:
		if port == "default" {
			return nil
		}
		var c SwitchConfig
		_ = json.Unmarshal(orEmpty(from.Config), &c)
		for _, cs := range c.Cases {
			if cs.Port == port {
				return nil
			}
		}
		return fmt.Errorf("workflow: edge from switch %q uses undeclared port %q", from.ID, port)
	default:
		if port == "" {
			return nil
		}
		if port == "error" && failable[from.Type] {
			return nil
		}
		return fmt.Errorf("workflow: edge from %q (%s) must use the default port", from.ID, from.Type)
	}
}

// Reliability bounds (M808): a node may not retry forever or sleep the run
// away between attempts.
const (
	maxNodeTimeoutSec = 600
	maxNodeRetries    = 5
	maxRetryDelaySec  = 60
)

// validateReliability checks the per-node retry/timeout settings. Retries
// only make sense on failable nodes (a transform never fails transiently);
// timeouts apply to anything that does work, so everything but the trigger.
func validateReliability(n *Node) error {
	if n.TimeoutSec < 0 || n.TimeoutSec > maxNodeTimeoutSec {
		return fmt.Errorf("workflow: node %s: timeout_sec must be 0..%d", n.ID, maxNodeTimeoutSec)
	}
	if n.TimeoutSec > 0 && n.Type == NodeTrigger {
		return fmt.Errorf("workflow: node %s: the trigger does not run — timeout_sec is meaningless on it", n.ID)
	}
	if n.Retries < 0 || n.Retries > maxNodeRetries {
		return fmt.Errorf("workflow: node %s: retries must be 0..%d", n.ID, maxNodeRetries)
	}
	if n.RetryDelaySec < 0 || n.RetryDelaySec > maxRetryDelaySec {
		return fmt.Errorf("workflow: node %s: retry_delay_sec must be 0..%d", n.ID, maxRetryDelaySec)
	}
	if (n.Retries > 0 || n.RetryDelaySec > 0) && !failable[n.Type] {
		return fmt.Errorf("workflow: node %s: retries only apply to failable nodes (tool/llm/http/code/approval/subworkflow/pipeline)", n.ID)
	}
	return nil
}

func validateNodeConfig(n *Node) error {
	if err := validateReliability(n); err != nil {
		return err
	}
	switch n.Type {
	case NodeTrigger:
		return validateTriggerConfig(n)
	case NodeTool:
		var c ToolConfig
		if err := json.Unmarshal(orEmpty(n.Config), &c); err != nil {
			return fmt.Errorf("workflow: node %s: tool config: %w", n.ID, err)
		}
		if strings.TrimSpace(c.Tool) == "" {
			return fmt.Errorf("workflow: node %s: tool name is required", n.ID)
		}
		return nil
	case NodeLLM:
		var c LLMConfig
		if err := json.Unmarshal(orEmpty(n.Config), &c); err != nil {
			return fmt.Errorf("workflow: node %s: llm config: %w", n.ID, err)
		}
		if strings.TrimSpace(c.Prompt) == "" {
			return fmt.Errorf("workflow: node %s: llm prompt is required", n.ID)
		}
		return nil
	case NodeCondition:
		var c ConditionConfig
		if err := json.Unmarshal(orEmpty(n.Config), &c); err != nil {
			return fmt.Errorf("workflow: node %s: condition config: %w", n.ID, err)
		}
		if !conditionOps[c.Op] {
			return fmt.Errorf("workflow: node %s: condition op %q (want equals|not_equals|contains|not_empty|empty|gt|lt)", n.ID, c.Op)
		}
		return nil
	case NodeTransform:
		var c TransformConfig
		if err := json.Unmarshal(orEmpty(n.Config), &c); err != nil {
			return fmt.Errorf("workflow: node %s: transform config: %w", n.ID, err)
		}
		if c.Template == "" {
			return fmt.Errorf("workflow: node %s: transform template is required", n.ID)
		}
		return nil
	case NodeDelay:
		var c DelayConfig
		if err := json.Unmarshal(orEmpty(n.Config), &c); err != nil {
			return fmt.Errorf("workflow: node %s: delay config: %w", n.ID, err)
		}
		if c.Seconds <= 0 || c.Seconds > maxDelaySeconds {
			return fmt.Errorf("workflow: node %s: delay seconds must be in (0, %d]", n.ID, maxDelaySeconds)
		}
		return nil
	case NodeHTTP:
		var c HTTPConfig
		if err := json.Unmarshal(orEmpty(n.Config), &c); err != nil {
			return fmt.Errorf("workflow: node %s: http config: %w", n.ID, err)
		}
		m := strings.ToUpper(strings.TrimSpace(c.Method))
		if m != "GET" && m != "POST" {
			return fmt.Errorf("workflow: node %s: http method must be GET or POST", n.ID)
		}
		if strings.TrimSpace(c.URL) == "" {
			return fmt.Errorf("workflow: node %s: http url is required", n.ID)
		}
		return nil
	case NodeCode:
		var c CodeConfig
		if err := json.Unmarshal(orEmpty(n.Config), &c); err != nil {
			return fmt.Errorf("workflow: node %s: code config: %w", n.ID, err)
		}
		if strings.TrimSpace(c.Language) == "" || strings.TrimSpace(c.Code) == "" {
			return fmt.Errorf("workflow: node %s: code needs language and code", n.ID)
		}
		return nil
	case NodeMap:
		var c MapConfig
		if err := json.Unmarshal(orEmpty(n.Config), &c); err != nil {
			return fmt.Errorf("workflow: node %s: map config: %w", n.ID, err)
		}
		if strings.TrimSpace(c.Items) == "" || c.Template == "" {
			return fmt.Errorf("workflow: node %s: map needs items and template", n.ID)
		}
		return nil
	case NodeFilter:
		var c FilterConfig
		if err := json.Unmarshal(orEmpty(n.Config), &c); err != nil {
			return fmt.Errorf("workflow: node %s: filter config: %w", n.ID, err)
		}
		if strings.TrimSpace(c.Items) == "" {
			return fmt.Errorf("workflow: node %s: filter needs items", n.ID)
		}
		if !conditionOps[c.Op] {
			return fmt.Errorf("workflow: node %s: filter op %q (want equals|not_equals|contains|not_empty|empty|gt|lt)", n.ID, c.Op)
		}
		return nil
	case NodeSwitch:
		var c SwitchConfig
		if err := json.Unmarshal(orEmpty(n.Config), &c); err != nil {
			return fmt.Errorf("workflow: node %s: switch config: %w", n.ID, err)
		}
		if strings.TrimSpace(c.Value) == "" {
			return fmt.Errorf("workflow: node %s: switch needs a value", n.ID)
		}
		if len(c.Cases) == 0 || len(c.Cases) > maxSwitchCases {
			return fmt.Errorf("workflow: node %s: switch needs 1..%d cases", n.ID, maxSwitchCases)
		}
		seen := map[string]bool{}
		for _, cs := range c.Cases {
			p := strings.TrimSpace(cs.Port)
			if p == "" || p == "default" || p == "error" {
				return fmt.Errorf("workflow: node %s: switch case ports must be named (not empty/default/error)", n.ID)
			}
			if seen[p] {
				return fmt.Errorf("workflow: node %s: duplicate switch port %q", n.ID, p)
			}
			seen[p] = true
		}
		return nil
	case NodeMerge:
		var c MergeConfig
		if err := json.Unmarshal(orEmpty(n.Config), &c); err != nil {
			return fmt.Errorf("workflow: node %s: merge config: %w", n.ID, err)
		}
		if c.Mode != "" && c.Mode != "all" && c.Mode != "any" {
			return fmt.Errorf("workflow: node %s: merge mode must be \"all\" or \"any\"", n.ID)
		}
		return nil
	case NodeApproval:
		var c ApprovalConfig
		if err := json.Unmarshal(orEmpty(n.Config), &c); err != nil {
			return fmt.Errorf("workflow: node %s: approval config: %w", n.ID, err)
		}
		if strings.TrimSpace(c.Description) == "" {
			return fmt.Errorf("workflow: node %s: approval needs a description (what the operator reads)", n.ID)
		}
		return nil
	case NodeSubflow:
		var c SubflowConfig
		if err := json.Unmarshal(orEmpty(n.Config), &c); err != nil {
			return fmt.Errorf("workflow: node %s: subworkflow config: %w", n.ID, err)
		}
		if strings.TrimSpace(c.Workflow) == "" {
			return fmt.Errorf("workflow: node %s: subworkflow needs a workflow name", n.ID)
		}
		return nil
	case NodePipeline:
		var c PipelineConfig
		if err := json.Unmarshal(orEmpty(n.Config), &c); err != nil {
			return fmt.Errorf("workflow: node %s: pipeline config: %w", n.ID, err)
		}
		if len(c.Steps) == 0 || len(c.Steps) > maxPipelineSteps {
			return fmt.Errorf("workflow: node %s: pipeline needs 1..%d steps", n.ID, maxPipelineSteps)
		}
		seen := map[string]bool{}
		for _, step := range c.Steps {
			id := strings.TrimSpace(step.ID)
			if !idRe.MatchString(id) {
				return fmt.Errorf("workflow: node %s: pipeline step id %q must match %s", n.ID, step.ID, idRe)
			}
			if seen[id] {
				return fmt.Errorf("workflow: node %s: duplicate pipeline step id %q", n.ID, id)
			}
			seen[id] = true
			if strings.TrimSpace(step.Tool) == "" {
				return fmt.Errorf("workflow: node %s: pipeline step %s needs a tool", n.ID, id)
			}
			if len(step.OutputSchema) > maxConfigBytes {
				return fmt.Errorf("workflow: node %s: pipeline step %s output_schema exceeds %d bytes", n.ID, id, maxConfigBytes)
			}
			if raw := strings.TrimSpace(string(step.OutputSchema)); raw != "" {
				if !json.Valid(step.OutputSchema) || !strings.HasPrefix(raw, "{") {
					return fmt.Errorf("workflow: node %s: pipeline step %s output_schema must be a JSON object", n.ID, id)
				}
			}
		}
		return nil
	default:
		return fmt.Errorf("workflow: node %s: unknown type %q (want %s)", n.ID, n.Type, strings.Join(knownTypes, "|"))
	}
}

func orEmpty(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage(`{}`)
	}
	return raw
}

// OpenStore opens (or creates) the registry under dir.
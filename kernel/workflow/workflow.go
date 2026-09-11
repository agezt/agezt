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
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Workflow types + Validate + canonicalDailyAt + Workflow.TriggerSpec.
// Code extracted from workflow.go during the Day-46 god-file split. Public API unchanged.


// ErrNotFound is returned for an unknown workflow id/name.
var ErrNotFound = errors.New("workflow: not found")

// Node types the engine executes.
const (
	NodeTrigger   = "trigger"   // the single entry point: manual | cron | event (M799)
	NodeTool      = "tool"      // one governed tool call: config {tool, args}
	NodeLLM       = "llm"       // one completion: config {prompt, system?, model?}
	NodeCondition = "condition" // branch: config {left, op, right} → "true"/"false" ports
	NodeTransform = "transform" // pure template: config {template} → output
	NodeDelay     = "delay"     // wait: config {seconds}

	// The M800 library.
	NodeHTTP     = "http"        // request via the governed http tool: {method, url, headers?, body?}
	NodeCode     = "code"        // sandboxed script: {language, code, input?} (input → stdin.txt)
	NodeMap      = "map"         // per-item template over an array: {items, template} ({{item}}, {{index}})
	NodeFilter   = "filter"      // keep matching items: {items, left, op, right} ({{item}} in left/right)
	NodeSwitch   = "switch"      // multi-way branch: {value, cases:[{equals, port}]} → case ports | "default"
	NodeMerge    = "merge"       // join branches: {mode: "all"|"any"} (all = wait for every incoming edge)
	NodeApproval = "approval"    // HITL gate: {description, capability?} — blocks on the operator
	NodeSubflow  = "subworkflow" // run another stored workflow: {workflow, payload?} (depth-capped)
	NodePipeline = "pipeline"    // deterministic typed tool chain: {steps:[{id,tool,args,output_schema?}]}
)

// knownTypes gates validation; ordered for error messages.
var knownTypes = []string{
	NodeTrigger, NodeTool, NodeLLM, NodeCondition, NodeTransform, NodeDelay,
	NodeHTTP, NodeCode, NodeMap, NodeFilter, NodeSwitch, NodeMerge, NodeApproval, NodeSubflow,
	NodePipeline,
}

// Failable nodes may wire an "error" port: when the node fails AND such an
// edge exists, the run survives — {{node.output.error}} carries the message
// and the error branch runs instead of the default one.
var failable = map[string]bool{
	NodeTool: true, NodeLLM: true, NodeHTTP: true, NodeCode: true,
	NodeApproval: true, NodeSubflow: true, NodePipeline: true,
}

// Node is one typed step on the canvas.
type Node struct {
	ID    string `json:"id"`   // unique within the workflow, [a-z0-9_-]
	Type  string `json:"type"` // one of the Node* constants
	Label string `json:"label,omitempty"`
	// Config is the type-specific payload (see the constants above). Its
	// string fields may carry {{path}} templates resolved at run time
	// against upstream outputs: {{trigger.payload}}, {{<node_id>.output}}.
	Config json.RawMessage `json:"config,omitempty"`
	// X/Y are canvas coordinates — pure presentation, never semantics.
	X float64 `json:"x,omitempty"`
	Y float64 `json:"y,omitempty"`

	// Reliability settings (M808) — production runs need per-node control,
	// not per-workflow hope. TimeoutSec bounds ONE attempt (0 = no extra
	// bound beyond the run's own deadline). Retries re-runs a FAILABLE node
	// after a failure (the error port only fires once retries are
	// exhausted); RetryDelaySec pauses between attempts.
	TimeoutSec    int `json:"timeout_sec,omitempty"`
	Retries       int `json:"retries,omitempty"`
	RetryDelaySec int `json:"retry_delay_sec,omitempty"`
}

// Edge wires From's completion to To's execution. Port selects a labelled
// output: a condition node fires exactly one of "true"/"false"; every other
// node fires its "" (default) port.
type Edge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Port string `json:"port,omitempty"`
}

// Workflow is one durable, named graph.
type Workflow struct {
	ID   string `json:"id"`
	Name string `json:"name"` // unique handle, immutable shape rules like roster slugs
	// Enabled gates triggers (M799) — a disabled workflow never auto-fires;
	// a manual run is still allowed (that's how you test a draft).
	Enabled     bool   `json:"enabled"`
	Description string `json:"description,omitempty"`
	Nodes       []Node `json:"nodes"`
	Edges       []Edge `json:"edges,omitempty"`
	CreatedMS   int64  `json:"created_ms"`
	UpdatedMS   int64  `json:"updated_ms"`
}

var (
	nameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)
	idRe   = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)
)

const (
	maxNodes        = 100
	maxEdges        = 300
	maxConfigBytes  = 32 * 1024
	maxDelaySeconds = 600
)

// Validate checks a workflow's shape: name/id rules, exactly one trigger,
// known node types, edges that resolve, legal ports, per-type config, and an
// acyclic graph. The engine refuses to run anything Validate rejects.
func Validate(w Workflow) error {
	if !nameRe.MatchString(w.Name) {
		return fmt.Errorf("workflow: name must match %s", nameRe)
	}
	if len(w.Nodes) == 0 {
		return errors.New("workflow: at least one node (the trigger) is required")
	}
	if len(w.Nodes) > maxNodes {
		return fmt.Errorf("workflow: at most %d nodes", maxNodes)
	}
	if len(w.Edges) > maxEdges {
		return fmt.Errorf("workflow: at most %d edges", maxEdges)
	}

	byID := make(map[string]*Node, len(w.Nodes))
	triggers := 0
	for i := range w.Nodes {
		n := &w.Nodes[i]
		if !idRe.MatchString(n.ID) {
			return fmt.Errorf("workflow: node id %q must match %s", n.ID, idRe)
		}
		if _, dup := byID[n.ID]; dup {
			return fmt.Errorf("workflow: duplicate node id %q", n.ID)
		}
		byID[n.ID] = n
		if len(n.Config) > maxConfigBytes {
			return fmt.Errorf("workflow: node %s config exceeds %d bytes", n.ID, maxConfigBytes)
		}
		if err := validateNodeConfig(n); err != nil {
			return err
		}
		if n.Type == NodeTrigger {
			triggers++
		}
	}
	if triggers != 1 {
		return fmt.Errorf("workflow: exactly one trigger node required, found %d", triggers)
	}

	indeg := map[string]int{}
	adj := map[string][]string{}
	for _, e := range w.Edges {
		from, ok := byID[e.From]
		if !ok {
			return fmt.Errorf("workflow: edge from unknown node %q", e.From)
		}
		if _, ok := byID[e.To]; !ok {
			return fmt.Errorf("workflow: edge to unknown node %q", e.To)
		}
		if e.From == e.To {
			return fmt.Errorf("workflow: self-edge on %q", e.From)
		}
		if err := validateEdgePort(from, e.Port); err != nil {
			return err
		}
		if from.Type == NodeTrigger && byID[e.To].Type == NodeTrigger {
			return errors.New("workflow: trigger cannot feed a trigger")
		}
		adj[e.From] = append(adj[e.From], e.To)
		indeg[e.To]++
	}
	for _, n := range w.Nodes {
		if n.Type == NodeTrigger && indeg[n.ID] > 0 {
			return errors.New("workflow: the trigger cannot have incoming edges")
		}
	}

	// Acyclicity (Kahn) over the whole graph.
	queue := make([]string, 0, len(w.Nodes))
	deg := make(map[string]int, len(w.Nodes))
	for id := range byID {
		deg[id] = indeg[id]
		if indeg[id] == 0 {
			queue = append(queue, id)
		}
	}
	seen := 0
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		seen++
		for _, next := range adj[id] {
			deg[next]--
			if deg[next] == 0 {
				queue = append(queue, next)
			}
		}
	}
	if seen != len(w.Nodes) {
		return errors.New("workflow: the graph has a cycle")
	}
	return nil
}

// TriggerConfig is the trigger node's config (M799): how a workflow STARTS.
// kind "" or "manual" = run-on-demand only. "cron" fires on a clock —
// either every interval_sec seconds (≥30) or once a day at daily_at
// ("HH:MM", daemon-local time). "event" fires when a journal event's
// subject matches the glob (bus semantics: "*" = one token, ">" = rest),
// with the event riding in as {{trigger.payload}}. Triggers only arm while
// the workflow is ENABLED.
type TriggerConfig struct {
	Kind        string `json:"kind,omitempty"`
	IntervalSec int    `json:"interval_sec,omitempty"` // cron: every N seconds (≥30)
	DailyAt     string `json:"daily_at,omitempty"`     // cron: "HH:MM" once a day
	Subject     string `json:"subject,omitempty"`      // event: subject glob
	// Secret authenticates the webhook kind (M809): an external POST to
	// /hooks/<name> must present it. Per-workflow, never the console token —
	// a leaked hook secret can fire ONE workflow, nothing else.
	Secret string `json:"secret,omitempty"`
	// Reply (M812) makes the webhook SYNCHRONOUS: the caller's POST holds
	// until the run finishes and the response carries the outputs — n8n's
	// "respond to webhook". Reply workflows should be fast (the sync path
	// is capped well under the async 15m); long pipelines stay async.
	Reply bool `json:"reply,omitempty"`
}

const (
	minIntervalSec      = 30
	minWebhookSecretLen = 12
)

var dailyAtRe = regexp.MustCompile(`^([01]?\d|2[0-3]):[0-5]\d$`)

// canonicalDailyAt normalizes a daily_at value to the fixed-width "HH:MM" form
// the trigger scheduler compares against.
//
// dailyAtRe deliberately accepts a one-digit hour ("9:05"), but the runner
// decides "has today's time arrived?" by comparing that value as a STRING
// against time.Format("15:04"), which is always two-digit. A one-digit hour
// therefore compares wrong for every minute of the day: each possible clock
// value begins with '0', '1' or '2', all of which sort below '9', so "9:05"
// still reads as "not yet" at 23:59 and the trigger never fires at all.
// Padding here — at the single point where stored config becomes a TriggerSpec
// — repairs newly saved and already-persisted workflows alike.
//
// A value the regex does not match is returned trimmed but otherwise unchanged:
// Validate rejects those, and this must not silently disable a trigger the
// operator did set.
func canonicalDailyAt(s string) string {
	s = strings.TrimSpace(s)
	if m := dailyAtRe.FindStringSubmatch(s); m != nil && len(m[1]) == 1 {
		return "0" + s
	}
	return s
}

// TriggerSpec parses a workflow's trigger configuration (Validate
// guarantees it parses and is legal).
func (w Workflow) TriggerSpec() TriggerConfig {
	var c TriggerConfig
	if n := w.TriggerNode(); n != nil {
		_ = json.Unmarshal(orEmpty(n.Config), &c)
	}
	if c.Kind == "" {
		c.Kind = "manual"
	}
	c.DailyAt = canonicalDailyAt(c.DailyAt)
	return c
}
